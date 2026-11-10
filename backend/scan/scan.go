// Package scan turns a card photo into a Pokémon: read the name at the top of
// the card, then look it up on PokéAPI.
package scan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dhan0406/pokescan/backend/apierr"
	"github.com/dhan0406/pokescan/backend/imageprep"
	"github.com/dhan0406/pokescan/backend/ocr"
	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/tesseract"
)

// OCR reads one line of text from a PNG.
type OCR interface {
	Read(ctx context.Context, png []byte, opts tesseract.Options) (tesseract.Result, error)
}

// SpeciesGetter fetches Pokémon species details.
type SpeciesGetter interface {
	Species(ctx context.Context, idOrName string) (pokeapi.Species, error)
}

// Outcome statuses.
const (
	StatusMatch  = "match"
	StatusRetake = "retake"
)

// Retake reasons.
const (
	ReasonNameUnreadable = "name_unreadable"
	ReasonNotFound       = "not_found"
)

// Outcome is the result of a scan.
type Outcome struct {
	Status  string
	Pokemon *pokeapi.Species // match only
	Reason  string           // retake only
}

// ErrOCR means the OCR engine failed.
var ErrOCR = errors.New("ocr failed")

// Scanner runs the scan pipeline.
type Scanner struct {
	OCR           OCR
	Species       SpeciesGetter
	Bands         imageprep.Bands
	MinConfidence float64
}

// New returns a Scanner with the default bands and a confidence threshold of 60.
func New(o OCR, s SpeciesGetter) *Scanner {
	return &Scanner{OCR: o, Species: s, Bands: imageprep.DefaultBands, MinConfidence: 60}
}

// Scan reads a card photo and looks up the Pokémon named at the top of the card.
// OCR and upstream failures are returned as errors; everything a better photo
// could fix is a retake.
func (s *Scanner) Scan(ctx context.Context, img io.Reader) (Outcome, error) {
	regions, err := imageprep.Prepare(img, s.Bands)
	if err != nil {
		return Outcome{}, err
	}

	name, err := s.readNameBand(ctx, regions.Name)
	if err != nil {
		return Outcome{}, err
	}
	located := false
	if name == "" {
		located = true
		name = s.locate(ctx, regions.Image)
	}
	if name == "" {
		return Outcome{Status: StatusRetake, Reason: ReasonNameUnreadable}, nil
	}

	out, err := s.lookup(ctx, name)
	if err != nil || out.Status == StatusMatch || located {
		return out, err
	}
	// The band read something confident that is not a Pokémon (a copyright
	// line, say). Look for the name elsewhere in the photo before giving up.
	if alt := s.locate(ctx, regions.Image); alt != "" && alt != name {
		return s.lookup(ctx, alt)
	}
	return out, nil
}

// readNameBand reads the name band, retrying in block mode because
// single-line mode sometimes finds nothing on a small crop.
func (s *Scanner) readNameBand(ctx context.Context, png []byte) (string, error) {
	for _, opts := range []tesseract.Options{tesseract.NameOptions, tesseract.NameFallbackOptions} {
		read, err := s.OCR.Read(ctx, png, opts)
		if err != nil {
			return "", fmt.Errorf("%w: name band: %w", ErrOCR, err)
		}
		if name := s.readName(read); name != "" {
			return name, nil
		}
	}
	return "", nil
}

// lookup finds the species for a card name, trying the name as printed and then
// shorter runs of its words, so "Hop's Wooloo" finds wooloo and "Charizard ex"
// finds charizard.
func (s *Scanner) lookup(ctx context.Context, name string) (Outcome, error) {
	for _, slug := range nameCandidates(name) {
		sp, err := s.Species.Species(ctx, slug)
		if err == nil {
			return Outcome{Status: StatusMatch, Pokemon: &sp}, nil
		}
		if !errors.Is(err, apierr.ErrNotFound) {
			return Outcome{}, err
		}
	}
	return Outcome{Status: StatusRetake, Reason: ReasonNotFound}, nil
}

// nameCandidates returns PokéAPI names to try for a card name: every run of
// consecutive words, longest first.
func nameCandidates(name string) []string {
	words := strings.Fields(name)
	seen := map[string]bool{}
	var out []string
	for size := len(words); size >= 1; size-- {
		for i := 0; i+size <= len(words); i++ {
			slug := slugify(strings.Join(words[i:i+size], " "))
			if len(slug) < 3 || seen[slug] { // skips "v", "ex" and the like
				continue
			}
			seen[slug] = true
			out = append(out, slug)
		}
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// slugify turns a card name into a PokéAPI name: "Mr. Mime" is mr-mime,
// "Farfetch'd" is farfetchd, "Nidoran♀" is nidoran-f, "Flabébé" is flabebe.
func slugify(s string) string {
	s = strings.NewReplacer("♀", "-f", "♂", "-m", "é", "e", "É", "e", "'", "", "’", "", ".", "", ":", "").Replace(strings.ToLower(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// noiseConfidence is the word confidence below which a word is treated as
// stray marks (symbols, label fragments) and ignored when reading the name.
const noiseConfidence = 50

// readName returns the card name if it was extracted from confidently read
// words, or "". Only the words the name is built from count, not the band's
// overall confidence, because the band also holds the stage label, HP and the
// energy symbol.
func (s *Scanner) readName(r tesseract.Result) string {
	var kept tesseract.Result
	var texts []string
	for _, w := range r.Words {
		if w.Confidence < noiseConfidence {
			continue
		}
		kept.Words = append(kept.Words, w)
		texts = append(texts, w.Text)
		kept.Confidence += w.Confidence
	}
	if len(kept.Words) == 0 {
		return ""
	}
	kept.Confidence /= float64(len(kept.Words))
	kept.Text = strings.Join(texts, " ")

	name := ocr.ExtractName(kept.Text)
	if name == "" || sourceConfidence(kept, ocr.ExtractName, name) < s.MinConfidence {
		return ""
	}
	return name
}

// sourceConfidence finds the shortest run of consecutive words from which
// extract still produces want, and returns those words' mean confidence. If no
// run does, it falls back to the whole result's confidence.
func sourceConfidence(r tesseract.Result, extract func(string) string, want string) float64 {
	for size := 1; size <= len(r.Words); size++ {
		for i := 0; i+size <= len(r.Words); i++ {
			run := r.Words[i : i+size]
			texts := make([]string, len(run))
			var total float64
			for j, w := range run {
				texts[j] = w.Text
				total += w.Confidence
			}
			if extract(strings.Join(texts, " ")) == want {
				return total / float64(size)
			}
		}
	}
	return r.Confidence
}

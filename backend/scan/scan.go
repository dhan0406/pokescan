// Package scan turns a card photo into a matched card, a list of candidates,
// or a request to retake the photo.
package scan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dhan0406/pokescan/backend/apierr"
	"github.com/dhan0406/pokescan/backend/ocr"

	"github.com/dhan0406/pokescan/backend/imageprep"
	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/tcg"
	"github.com/dhan0406/pokescan/backend/tesseract"
)

// OCR reads one line of text from a PNG.
type OCR interface {
	Read(ctx context.Context, png []byte, opts tesseract.Options) (tesseract.Result, error)
}

// CardSearcher finds TCG cards.
type CardSearcher interface {
	FindCards(ctx context.Context, name, number string) ([]tcg.Card, error)
	Search(ctx context.Context, q tcg.Query) ([]tcg.Card, error)
}

// SpeciesGetter fetches Pokémon species details.
type SpeciesGetter interface {
	Species(ctx context.Context, idOrName string) (pokeapi.Species, error)
}

// Outcome statuses.
const (
	StatusMatch  = "match"
	StatusChoose = "choose"
	StatusRetake = "retake"
)

// Retake reasons.
const (
	ReasonNameUnreadable   = "name_unreadable"
	ReasonNumberUnreadable = "number_unreadable"
	ReasonCardUnreadable   = "card_unreadable"
	ReasonNotFound         = "not_found"
)

// Outcome is the result of a scan.
type Outcome struct {
	Status     string
	Card       *tcg.Card        // match only
	Pokemon    *pokeapi.Species // match only; nil for Trainer and Energy cards
	Candidates []tcg.Card       // choose, and optionally retake
	Reason     string           // retake only
}

// ErrOCR means the OCR engine failed.
var ErrOCR = errors.New("ocr failed")

// Scanner runs the scan pipeline.
type Scanner struct {
	OCR           OCR
	Cards         CardSearcher
	Species       SpeciesGetter
	Bands         imageprep.Bands
	MinConfidence float64
}

// New returns a Scanner with the default bands and a confidence threshold of 60.
func New(o OCR, c CardSearcher, s SpeciesGetter) *Scanner {
	return &Scanner{OCR: o, Cards: c, Species: s, Bands: imageprep.DefaultBands, MinConfidence: 60}
}

// noiseConfidence is the word confidence below which a word is treated as
// stray marks (symbols, label fragments) and ignored when reading the name.
const noiseConfidence = 50

// readName returns the card name if it was extracted from confidently read
// words, or "".
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
// extract still produces want, and returns those words' mean confidence. If
// no run does (the words don't line up with the text), it falls back to the
// whole band's confidence.
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

// maxCandidates caps every candidate list.
const maxCandidates = 5

// Scan reads a card photo and decides the outcome. Upstream and OCR failures
// are returned as errors; everything a better photo could fix is a retake.
func (s *Scanner) Scan(ctx context.Context, img io.Reader) (Outcome, error) {
	regions, err := imageprep.Prepare(img, s.Bands)
	if err != nil {
		return Outcome{}, err
	}

	nameRead, err := s.OCR.Read(ctx, regions.Name, tesseract.NameOptions)
	if err != nil {
		return Outcome{}, fmt.Errorf("%w: name band: %w", ErrOCR, err)
	}
	numberRead, err := s.OCR.Read(ctx, regions.Number, tesseract.NumberOptions)
	if err != nil {
		return Outcome{}, fmt.Errorf("%w: number band: %w", ErrOCR, err)
	}

	// The name must be read confidently: only the words it was built from count,
	// not the band's overall confidence, because the band also holds the stage
	// label, HP and energy symbol. The number band is read with a character
	// whitelist, which makes Tesseract report confidence 0 for good digits, so
	// the number is validated by its pattern here and by the card lookup after.
	name := s.readName(nameRead)
	number, total := ocr.ExtractNumber(numberRead.Text)

	// Single-line mode sometimes finds nothing on a small crop that block mode
	// reads fine, so try the name once more before giving up on it.
	if name == "" {
		retry, err := s.OCR.Read(ctx, regions.Name, tesseract.NameFallbackOptions)
		if err != nil {
			return Outcome{}, fmt.Errorf("%w: name band retry: %w", ErrOCR, err)
		}
		name = s.readName(retry)
	}

	switch {
	case name != "" && number != "":
		return s.fullRead(ctx, name, number)
	case number != "":
		setTotal, _ := strconv.Atoi(total) // 0 when absent or not numeric (TG30)
		return s.partialRead(ctx, tcg.Query{Number: number, SetTotal: setTotal}, ReasonNameUnreadable)
	case name != "":
		return s.partialRead(ctx, tcg.Query{Name: name}, ReasonNumberUnreadable)
	default:
		return Outcome{Status: StatusRetake, Reason: ReasonCardUnreadable}, nil
	}
}

// fullRead handles a confident name and number.
func (s *Scanner) fullRead(ctx context.Context, name, number string) (Outcome, error) {
	found, err := s.Cards.FindCards(ctx, name, number)
	if err != nil {
		return Outcome{}, err
	}
	switch len(found) {
	case 0:
		return Outcome{Status: StatusRetake, Reason: ReasonNotFound}, nil
	case 1:
		card := found[0]
		out := Outcome{Status: StatusMatch, Card: &card}
		if card.DexNumber == 0 {
			return out, nil // Trainer or Energy card
		}
		sp, err := s.Species.Species(ctx, strconv.Itoa(card.DexNumber))
		if err != nil {
			if errors.Is(err, apierr.ErrNotFound) {
				// The card exists, so a missing species is an upstream
				// inconsistency, not "not found" for the user.
				return Outcome{}, fmt.Errorf("%w: species %d for card %s: %v", apierr.ErrUpstream, card.DexNumber, card.ID, err)
			}
			return Outcome{}, err
		}
		out.Pokemon = &sp
		return out, nil
	default:
		return Outcome{Status: StatusChoose, Candidates: capped(found)}, nil
	}
}

// partialRead asks for a retake but offers the closest matches it can find.
func (s *Scanner) partialRead(ctx context.Context, q tcg.Query, reason string) (Outcome, error) {
	found, err := s.Cards.Search(ctx, q)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Status: StatusRetake, Reason: reason, Candidates: capped(found)}, nil
}

func capped(c []tcg.Card) []tcg.Card {
	if len(c) == 0 {
		return nil
	}
	if len(c) > maxCandidates {
		c = c[:maxCandidates]
	}
	return append([]tcg.Card(nil), c...)
}

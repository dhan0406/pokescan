package scan

import (
	"context"
	"image"
	"sort"
	"strings"

	"github.com/dhan0406/pokescan/backend/imageprep"
	"github.com/dhan0406/pokescan/backend/ocr"
	"github.com/dhan0406/pokescan/backend/tesseract"
)

// sparseOptions reads a whole photo for scattered text.
var sparseOptions = tesseract.Options{PSM: 11}

// locate is the fallback when the fixed name band misses: read the whole photo
// and find the name (the large text on the HP line) wherever it is, then re-read
// a tight crop around it. Best effort; any problem just means nothing was found.
func (s *Scanner) locate(ctx context.Context, img image.Image) string {
	if img == nil {
		return ""
	}
	full, err := imageprep.GrayPNG(img)
	if err != nil {
		return ""
	}
	res, err := s.OCR.Read(ctx, full, sparseOptions)
	if err != nil || len(res.Words) == 0 {
		return ""
	}
	line := findNameLine(res.Words)
	if len(line) == 0 {
		return ""
	}
	union := box(line[0])
	for _, w := range line[1:] {
		union = union.Union(box(w))
	}
	h := union.Dy()
	crop, err := imageprep.CropPNG(img, pad(union, h/2, h/3))
	if err != nil {
		return ""
	}
	for _, opts := range []tesseract.Options{tesseract.NameOptions, tesseract.NameFallbackOptions} {
		if re, err := s.OCR.Read(ctx, crop, opts); err == nil {
			if name := s.readName(re); name != "" {
				return name
			}
		}
	}
	return ""
}

func box(w tesseract.Word) image.Rectangle {
	return image.Rect(w.Left, w.Top, w.Left+w.Width, w.Top+w.Height)
}

func pad(r image.Rectangle, x, y int) image.Rectangle {
	return image.Rect(r.Min.X-x, r.Min.Y-y, r.Max.X+x, r.Max.Y+y)
}

// findNameLine returns the words that make up the card name, left to right.
// On a Pokémon card the name shares a line with "HP" and the hit points, to
// its left. Trainer and Energy cards have no HP, so the tallest text in the
// upper part of the card is used instead.
func findNameLine(words []tesseract.Word) []tesseract.Word {
	for _, hp := range words {
		if !strings.EqualFold(strings.Trim(hp.Text, ".:"), "HP") {
			continue
		}
		cy := hp.Top + hp.Height/2
		var line []tesseract.Word
		for _, w := range words {
			wy := w.Top + w.Height/2
			if w.Left+w.Width <= hp.Left && abs(wy-cy) <= 2*hp.Height+w.Height/2 && w.Confidence >= noiseConfidence {
				line = append(line, w)
			}
		}
		if len(line) > 0 {
			sort.Slice(line, func(i, j int) bool { return line[i].Left < line[j].Left })
			return line
		}
	}

	// No HP: the tallest confident word in the top half of the text.
	top, bottom := words[0].Top, words[0].Top+words[0].Height
	for _, w := range words {
		top, bottom = min(top, w.Top), max(bottom, w.Top+w.Height)
	}
	var tallest *tesseract.Word
	for i, w := range words {
		letters := len(ocr.ExtractName(w.Text))
		if w.Top < top+(bottom-top)/2 && w.Confidence >= noiseConfidence && letters >= 3 && (tallest == nil || w.Height > tallest.Height) {
			tallest = &words[i]
		}
	}
	if tallest == nil {
		return nil
	}
	cy := tallest.Top + tallest.Height/2
	var line []tesseract.Word
	for _, w := range words {
		if abs(w.Top+w.Height/2-cy) <= tallest.Height/2 && w.Confidence >= noiseConfidence {
			line = append(line, w)
		}
	}
	sort.Slice(line, func(i, j int) bool { return line[i].Left < line[j].Left })
	return line
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

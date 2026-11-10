// Package ocr turns messy OCR text from a Pokémon card into a card name and
package ocr

import (
	"regexp"
	"strings"
	"unicode"
)

// ExtractName returns the card name from OCR text of the top of a card, or ""
// if none could be read. It takes the first line that still has letters after
// dropping stage labels ("BASIC", "Stage 1"), HP values and stray symbols.
func ExtractName(text string) string {
	text = strings.NewReplacer("’", "'", "‘", "'").Replace(text)
	for _, line := range strings.Split(text, "\n") {
		var words []string
		for _, tok := range strings.Fields(line) {
			if w := cleanNameToken(tok); w != "" {
				words = append(words, w)
			}
		}
		if len(words) > 0 {
			return strings.Join(words, " ")
		}
	}
	return ""
}

// cleanNameToken trims noise from one word and returns "" if the word is not
// part of a name (stage label, HP value, bare number or symbol).
func cleanNameToken(tok string) string {
	// Keep a trailing dot for names like "Mr." and "Jr.".
	tok = strings.TrimLeftFunc(tok, func(r rune) bool { return !isAlnum(r) })
	tok = strings.TrimRightFunc(tok, func(r rune) bool { return !isAlnum(r) && r != '.' })
	if tok == "" || !strings.ContainsFunc(tok, unicode.IsLetter) {
		return ""
	}
	if noiseWord.MatchString(tok) || strayMark.MatchString(tok) {
		return ""
	}
	if len([]rune(tok)) <= 2 && !shortNameWords[tok] {
		return "" // a fragment of the HP or energy symbol, not part of a name
	}
	return tok
}

// Short words that really do appear in card names.
var shortNameWords = map[string]bool{
	"ex": true, "EX": true, "V": true, "GX": true, "Z": true, "Mr": true, "Mr.": true, "Jr": true, "Jr.": true, "♀": true, "♂": true,
}

// A lowercase letter followed by digits ("w70") is the HP value, not a name.
var strayMark = regexp.MustCompile(`^[a-z]\d+$`)

var noiseWord = regexp.MustCompile(`(?i)^(basic|stage|hp\d*)$`)

func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

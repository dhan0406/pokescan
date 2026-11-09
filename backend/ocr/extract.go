// Package ocr turns messy OCR text from a Pokémon card into a card name and
// collector number. The scan pipeline OCRs the name region and the number
// region separately, so each has its own function.
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
	if noiseWord.MatchString(tok) {
		return ""
	}
	return tok
}

var noiseWord = regexp.MustCompile(`(?i)^(basic|stage|hp\d*)$`)

func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

var (
	// A token on each side of a slash, e.g. "004/165" or "TG05/TG30".
	slashPair = regexp.MustCompile(`\b([A-Za-z0-9]{1,6})\s*/\s*([A-Za-z0-9]{1,6})\b`)
	// A promo number with no total, e.g. "SWSH123".
	promo = regexp.MustCompile(`\b[A-Z]{2,5}[0-9]{1,3}\b`)
	// What a normalized number looks like: optional letter prefix, then digits.
	validNumber = regexp.MustCompile(`^(?:[A-Z]{2,5})?[0-9]{1,3}$`)
)

// ExtractNumber returns the collector number and set total from OCR text of the
// bottom of a card, e.g. "004/165" gives ("004", "165"). Promo cards have no
// total, e.g. "SWSH123" gives ("SWSH123", ""). Both are "" if none was found.
func ExtractNumber(text string) (number, total string) {
	for _, m := range slashPair.FindAllStringSubmatch(text, -1) {
		n, t := normalizeNumber(m[1]), normalizeNumber(m[2])
		if validNumber.MatchString(n) && validNumber.MatchString(t) {
			return n, t
		}
	}
	if m := promo.FindString(text); m != "" {
		return m, ""
	}
	return "", ""
}

// normalizeNumber fixes letters that OCR commonly confuses with digits
// (O for 0, l or I for 1). A prefix of two or more letters followed by a digit,
// like "TG" in "TG05", is a real prefix and is left alone.
func normalizeNumber(tok string) string {
	letters := len(tok) - len(strings.TrimLeftFunc(tok, unicode.IsLetter))
	if letters >= 2 && letters < len(tok) && unicode.IsDigit(rune(tok[letters])) {
		return strings.ToUpper(tok[:letters]) + fixDigits(tok[letters:])
	}
	return fixDigits(tok)
}

var digitLookalikes = strings.NewReplacer("O", "0", "o", "0", "l", "1", "I", "1", "i", "1")

func fixDigits(s string) string { return digitLookalikes.Replace(s) }

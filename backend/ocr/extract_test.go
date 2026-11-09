package ocr

import "testing"

func TestExtractName(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"plain name", "Charizard", "Charizard"},
		{"leading blank lines", "\n\n  Pikachu \n", "Pikachu"},
		{"stage and HP noise", "BASIC Pikachu HP 60", "Pikachu"},
		{"stage 1 with joined HP", "Stage 1 Ivysaur HP90", "Ivysaur"},
		{"trailing stray pipe", "Pikachu |", "Pikachu"},
		{"suffix is part of the name", "Charizard ex", "Charizard ex"},
		{"name with dot", "Mr. Mime", "Mr. Mime"},
		{"trailing dot kept", "Mime Jr.", "Mime Jr."},
		{"curly apostrophe", "Farfetch’d", "Farfetch'd"},
		{"accented letters", "Flabébé", "Flabébé"},
		{"digit inside a name", "Porygon2", "Porygon2"},
		{"skips lines with no letters", "|||\n60\nSnorlax", "Snorlax"},
		{"empty text", "", ""},
		{"only garbage", "~~ || ..", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractName(tt.text); got != tt.want {
				t.Errorf("ExtractName(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestExtractNumber(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantNumber string
		wantTotal  string
	}{
		{"clean number", "004/165", "004", "165"},
		{"spaces around slash", "004 / 165", "004", "165"},
		{"surrounded by other text", "Illus. Mitsuhiro Arita 004/165 ★", "004", "165"},
		{"letter O and l read as digits", "O04/l65", "004", "165"},
		{"promo with no total", "SWSH123", "SWSH123", ""},
		{"trainer gallery", "TG05/TG30", "TG05", "TG30"},
		{"skips slash that is not a number", "and/or 025/198", "025", "198"},
		{"empty text", "", "", ""},
		{"no number present", "Illus. Mitsuhiro Arita", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, total := ExtractNumber(tt.text)
			if n != tt.wantNumber || total != tt.wantTotal {
				t.Errorf("ExtractNumber(%q) = (%q, %q), want (%q, %q)", tt.text, n, total, tt.wantNumber, tt.wantTotal)
			}
		})
	}
}

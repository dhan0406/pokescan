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

package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"reflect"
	"strings"
	"testing"

	"github.com/dhan0406/pokescan/backend/apierr"
	"github.com/dhan0406/pokescan/backend/imageprep"
	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/tcg"
	"github.com/dhan0406/pokescan/backend/tesseract"
)

// fakeOCR returns results in call order: name band first, number band second.
type fakeOCR struct {
	results []tesseract.Result
	err     error
	calls   int
	opts    []tesseract.Options
}

func (f *fakeOCR) Read(ctx context.Context, png []byte, opts tesseract.Options) (tesseract.Result, error) {
	f.calls++
	f.opts = append(f.opts, opts)
	if f.err != nil {
		return tesseract.Result{}, f.err
	}
	if f.calls > len(f.results) {
		return tesseract.Result{}, nil // a read nobody scripted finds nothing
	}
	return f.results[f.calls-1], nil
}

type fakeCards struct {
	find, search       []tcg.Card
	findErr, searchErr error
	findArgs           [][2]string
	searchArgs         []tcg.Query
}

func (f *fakeCards) FindCards(ctx context.Context, name, number string) ([]tcg.Card, error) {
	f.findArgs = append(f.findArgs, [2]string{name, number})
	return f.find, f.findErr
}

func (f *fakeCards) Search(ctx context.Context, q tcg.Query) ([]tcg.Card, error) {
	f.searchArgs = append(f.searchArgs, q)
	return f.search, f.searchErr
}

type fakeSpecies struct {
	err  error
	args []string
}

func (f *fakeSpecies) Species(ctx context.Context, idOrName string) (pokeapi.Species, error) {
	f.args = append(f.args, idOrName)
	if f.err != nil {
		return pokeapi.Species{}, f.err
	}
	return pokeapi.Species{ID: 25, Name: "pikachu"}, nil
}

func cardPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 500, 700))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// read builds a Result whose words all have the same confidence.
func read(text string, conf float64) tesseract.Result {
	r := tesseract.Result{Text: text, Confidence: conf}
	for _, w := range strings.Fields(text) {
		r.Words = append(r.Words, tesseract.Word{Text: w, Confidence: conf})
	}
	return r
}

// words builds a Result from words with individual confidences, with the
// band mean in Confidence as Tesseract reports it.
func words(ws ...tesseract.Word) tesseract.Result {
	r := tesseract.Result{Words: ws}
	var texts []string
	for _, w := range ws {
		texts = append(texts, w.Text)
		r.Confidence += w.Confidence / float64(len(ws))
	}
	r.Text = strings.Join(texts, " ")
	return r
}

func TestScan_thresholdUsesTheExtractedWords(t *testing.T) {
	w := func(text string, conf float64) tesseract.Word { return tesseract.Word{Text: text, Confidence: conf} }
	tests := []struct {
		name       string
		nameRead   tesseract.Result
		numberRead tesseract.Result
		wantStatus string
		wantReason string
		wantFind   [][2]string
		wantSearch []tcg.Query
	}{
		{
			// A whitelisted number read reports confidence 0 (or any value), so
			// it is validated by its pattern and the card lookup instead.
			name:       "number confidence is not used",
			nameRead:   read("Pikachu", 90),
			numberRead: words(w("025/165", 0), w("Pokemon", 5), w("Nintendo", 3)),
			wantStatus: StatusMatch,
			wantFind:   [][2]string{{"Pikachu", "025"}},
		},
		{
			// Real Tesseract output for a tight crop of a Mareep card: the
			// BASIC label and energy symbol come out as low-confidence junk.
			name:       "low-confidence junk words are dropped before reading the name",
			nameRead:   words(w("Re", 1.5), w("Mareep", 74.8), w("+e", 0), w("/", 20), w("OO", 20)),
			numberRead: read("027/086", 0),
			wantStatus: StatusMatch,
			wantFind:   [][2]string{{"Mareep", "027"}},
		},
		{
			name:       "a number that is not a number is unread whatever its confidence",
			nameRead:   read("Pikachu", 90),
			numberRead: words(w("DC", 95), w("UKUMO", 90)),
			wantStatus: StatusRetake, wantReason: ReasonNumberUnreadable,
			wantSearch: []tcg.Query{{Name: "Pikachu"}},
		},
		{
			// Band mean is about 42, dragged down by the copyright strip, but
			// the number itself was read at 95: it must count.
			name:       "shaky boilerplate does not sink a clear number",
			nameRead:   read("Pikachu", 90),
			numberRead: words(w("025/165", 95), w("©2023", 10), w("Pokemon", 20)),
			wantStatus: StatusMatch,
			wantFind:   [][2]string{{"Pikachu", "025"}},
		},
		{
			name:       "number split across words uses those words only",
			nameRead:   read("Pikachu", 90),
			numberRead: words(w("Illus.", 15), w("025", 92), w("/", 88), w("165", 90), w("©2023", 10)),
			wantStatus: StatusMatch,
			wantFind:   [][2]string{{"Pikachu", "025"}},
		},
		{
			name:       "multi-word name ignores the HP label",
			nameRead:   words(w("Charizard", 90), w("ex", 80), w("HP", 30), w("330", 20)),
			numberRead: read("125/197", 90),
			wantStatus: StatusMatch,
			wantFind:   [][2]string{{"Charizard ex", "125"}},
		},
		{
			name:       "shaky name word is unread even if the rest is clear",
			nameRead:   words(w("Basic", 95), w("Pikachv", 41), w("HP", 92), w("60", 95)),
			numberRead: read("025/165", 90),
			wantStatus: StatusRetake, wantReason: ReasonNameUnreadable,
			wantSearch: []tcg.Query{{Number: "025", SetTotal: 165}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeCards{find: cards(1), search: cards(1)}
			got, err := New(&fakeOCR{results: []tesseract.Result{tt.nameRead, tt.numberRead}}, c, &fakeSpecies{}).
				Scan(context.Background(), bytes.NewReader(cardPNG(t)))
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if got.Status != tt.wantStatus || got.Reason != tt.wantReason {
				t.Errorf("status, reason = %q, %q; want %q, %q", got.Status, got.Reason, tt.wantStatus, tt.wantReason)
			}
			if !reflect.DeepEqual(c.findArgs, tt.wantFind) {
				t.Errorf("FindCards calls = %v, want %v", c.findArgs, tt.wantFind)
			}
			if !reflect.DeepEqual(c.searchArgs, tt.wantSearch) {
				t.Errorf("Search calls = %+v, want %+v", c.searchArgs, tt.wantSearch)
			}
		})
	}
}

func cards(n int) []tcg.Card {
	out := make([]tcg.Card, n)
	for i := range out {
		out[i] = tcg.Card{ID: fmt.Sprintf("set%d-25", i), Name: "Pikachu", Number: "25", DexNumber: 25}
	}
	return out
}

func TestScan_outcomes(t *testing.T) {
	tests := []struct {
		name           string
		nameRead       tesseract.Result
		numberRead     tesseract.Result
		find, search   []tcg.Card
		wantStatus     string
		wantReason     string
		wantCandidates int
		wantFind       [][2]string
		wantSearch     []tcg.Query
		wantSpecies    []string
	}{
		{
			name:     "one match",
			nameRead: read("Pikachu", 90), numberRead: read("025/165", 90),
			find:       cards(1),
			wantStatus: StatusMatch, wantFind: [][2]string{{"Pikachu", "025"}}, wantSpecies: []string{"25"},
		},
		{
			name:     "two matches",
			nameRead: read("Charizard ex", 90), numberRead: read("125/197", 90),
			find:       cards(2),
			wantStatus: StatusChoose, wantCandidates: 2, wantFind: [][2]string{{"Charizard ex", "125"}},
		},
		{
			name:     "seven matches are capped at five",
			nameRead: read("Pikachu", 90), numberRead: read("25/102", 90),
			find:       cards(7),
			wantStatus: StatusChoose, wantCandidates: 5, wantFind: [][2]string{{"Pikachu", "25"}},
		},
		{
			name:     "no match",
			nameRead: read("Pikachu", 90), numberRead: read("999/165", 90),
			find:       []tcg.Card{},
			wantStatus: StatusRetake, wantReason: ReasonNotFound, wantFind: [][2]string{{"Pikachu", "999"}},
		},
		{
			name:     "number and total only",
			nameRead: read("~~", 20), numberRead: read("025/165", 90),
			search:     cards(7),
			wantStatus: StatusRetake, wantReason: ReasonNameUnreadable, wantCandidates: 5,
			wantSearch: []tcg.Query{{Number: "025", SetTotal: 165}},
		},
		{
			name:     "promo number without total",
			nameRead: read("", 0), numberRead: read("SWSH039", 90),
			search:     cards(1),
			wantStatus: StatusRetake, wantReason: ReasonNameUnreadable, wantCandidates: 1,
			wantSearch: []tcg.Query{{Number: "SWSH039"}},
		},
		{
			name:     "trainer gallery total is not numeric",
			nameRead: read("", 0), numberRead: read("TG05/TG30", 90),
			search:     cards(2),
			wantStatus: StatusRetake, wantReason: ReasonNameUnreadable, wantCandidates: 2,
			wantSearch: []tcg.Query{{Number: "TG05"}},
		},
		{
			name:     "name only",
			nameRead: read("Pikachu", 90), numberRead: read("", 0),
			search:     cards(3),
			wantStatus: StatusRetake, wantReason: ReasonNumberUnreadable, wantCandidates: 3,
			wantSearch: []tcg.Query{{Name: "Pikachu"}},
		},
		{
			name:     "neither",
			nameRead: read("", 0), numberRead: read("", 0),
			wantStatus: StatusRetake, wantReason: ReasonCardUnreadable,
		},
		{
			name:     "confidence 59 makes a field unread",
			nameRead: read("Pikachu", 59), numberRead: read("025/165", 90),
			search:     cards(1),
			wantStatus: StatusRetake, wantReason: ReasonNameUnreadable, wantCandidates: 1,
			wantSearch: []tcg.Query{{Number: "025", SetTotal: 165}},
		},
		{
			name:     "confidence 60 counts as read",
			nameRead: read("Pikachu", 60), numberRead: read("025/165", 60),
			find:       cards(1),
			wantStatus: StatusMatch, wantFind: [][2]string{{"Pikachu", "025"}}, wantSpecies: []string{"25"},
		},
		{
			name:     "trainer card has no pokemon",
			nameRead: read("Professor's Research", 90), numberRead: read("62/73", 90),
			find:       []tcg.Card{{ID: "swsh35-62", Name: "Professor's Research"}},
			wantStatus: StatusMatch, wantFind: [][2]string{{"Professor's Research", "62"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &fakeOCR{results: []tesseract.Result{tt.nameRead, tt.numberRead}}
			c := &fakeCards{find: tt.find, search: tt.search}
			sp := &fakeSpecies{}
			got, err := New(o, c, sp).Scan(context.Background(), bytes.NewReader(cardPNG(t)))
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if got.Status != tt.wantStatus || got.Reason != tt.wantReason {
				t.Errorf("status, reason = %q, %q; want %q, %q", got.Status, got.Reason, tt.wantStatus, tt.wantReason)
			}
			if len(got.Candidates) != tt.wantCandidates {
				t.Errorf("%d candidates, want %d", len(got.Candidates), tt.wantCandidates)
			}
			if !reflect.DeepEqual(c.findArgs, tt.wantFind) {
				t.Errorf("FindCards calls = %v, want %v", c.findArgs, tt.wantFind)
			}
			if !reflect.DeepEqual(c.searchArgs, tt.wantSearch) {
				t.Errorf("Search calls = %+v, want %+v", c.searchArgs, tt.wantSearch)
			}
			if !reflect.DeepEqual(sp.args, tt.wantSpecies) {
				t.Errorf("Species calls = %v, want %v", sp.args, tt.wantSpecies)
			}
			switch tt.wantStatus {
			case StatusMatch:
				if got.Card == nil {
					t.Error("match without Card")
				}
				if (got.Pokemon != nil) != (len(tt.wantSpecies) > 0) {
					t.Errorf("Pokemon = %+v, want present only when a species was fetched", got.Pokemon)
				}
			default:
				if got.Card != nil || got.Pokemon != nil {
					t.Errorf("non-match has Card %v / Pokemon %v", got.Card, got.Pokemon)
				}
			}
		})
	}
}

func TestScan_errors(t *testing.T) {
	good := []tesseract.Result{read("Pikachu", 90), read("025/165", 90)}
	tests := []struct {
		name    string
		ocr     *fakeOCR
		cards   *fakeCards
		species *fakeSpecies
		img     []byte
		want    []error
	}{
		{"find fails", &fakeOCR{results: good}, &fakeCards{findErr: apierr.ErrUpstream}, &fakeSpecies{}, nil, []error{apierr.ErrUpstream}},
		{"search fails", &fakeOCR{results: []tesseract.Result{read("", 0), read("025/165", 90)}}, &fakeCards{searchErr: apierr.ErrTimeout}, &fakeSpecies{}, nil, []error{apierr.ErrTimeout}},
		{"species not found is upstream", &fakeOCR{results: good}, &fakeCards{find: cards(1)}, &fakeSpecies{err: apierr.ErrNotFound}, nil, []error{apierr.ErrUpstream}},
		{"species timeout", &fakeOCR{results: good}, &fakeCards{find: cards(1)}, &fakeSpecies{err: apierr.ErrTimeout}, nil, []error{apierr.ErrTimeout}},
		{"ocr fails", &fakeOCR{err: errors.New("exit 1")}, &fakeCards{}, &fakeSpecies{}, nil, []error{ErrOCR}},
		{"ocr times out", &fakeOCR{err: fmt.Errorf("%w: slow", apierr.ErrTimeout)}, &fakeCards{}, &fakeSpecies{}, nil, []error{ErrOCR, apierr.ErrTimeout}},
		{"bad image", &fakeOCR{}, &fakeCards{}, &fakeSpecies{}, []byte("not an image"), []error{imageprep.ErrBadImage}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := tt.img
			if img == nil {
				img = cardPNG(t)
			}
			_, err := New(tt.ocr, tt.cards, tt.species).Scan(context.Background(), bytes.NewReader(img))
			for _, want := range tt.want {
				if !errors.Is(err, want) {
					t.Errorf("err = %v, want it to match %v", err, want)
				}
			}
			if tt.name == "species not found is upstream" && errors.Is(err, apierr.ErrNotFound) {
				t.Errorf("err = %v, must not match ErrNotFound", err)
			}
			if tt.name == "bad image" && tt.ocr.calls != 0 {
				t.Errorf("OCR called %d times for a bad image, want 0", tt.ocr.calls)
			}
		})
	}
}

func TestScan_readsEachBandWithItsOwnOptions(t *testing.T) {
	o := &fakeOCR{results: []tesseract.Result{read("Pikachu", 90), read("025/165", 0)}}
	_, err := New(o, &fakeCards{find: cards(1)}, &fakeSpecies{}).Scan(context.Background(), bytes.NewReader(cardPNG(t)))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want := []tesseract.Options{tesseract.NameOptions, tesseract.NumberOptions}
	if !reflect.DeepEqual(o.opts, want) {
		t.Errorf("OCR options = %+v, want name then number: %+v", o.opts, want)
	}
}

func TestScan_retriesTheNameInBlockModeWhenSingleLineFindsNothing(t *testing.T) {
	w := func(text string, conf float64) tesseract.Word { return tesseract.Word{Text: text, Confidence: conf} }
	// Real output for a Mareep card: single-line mode found nothing, block mode
	// found the name among stray marks.
	blockMode := words(w("=", 41), w("Mareep", 71), w(".", 70), w("w/0'", 0), w("@", 49), w("q", 49), w("ee", 34))
	o := &fakeOCR{results: []tesseract.Result{{}, read("027/086", 0), blockMode}}
	c := &fakeCards{find: cards(1)}
	got, err := New(o, c, &fakeSpecies{}).Scan(context.Background(), bytes.NewReader(cardPNG(t)))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got.Status != StatusMatch {
		t.Errorf("status = %q (reason %q), want match", got.Status, got.Reason)
	}
	if want := [][2]string{{"Mareep", "027"}}; !reflect.DeepEqual(c.findArgs, want) {
		t.Errorf("FindCards calls = %v, want %v", c.findArgs, want)
	}
	wantOpts := []tesseract.Options{tesseract.NameOptions, tesseract.NumberOptions, tesseract.NameFallbackOptions}
	if !reflect.DeepEqual(o.opts, wantOpts) {
		t.Errorf("OCR options = %+v, want %+v", o.opts, wantOpts)
	}
}

func TestScan_doesNotRetryTheNameWhenTheFirstReadWorked(t *testing.T) {
	o := &fakeOCR{results: []tesseract.Result{read("Pikachu", 90), read("025/165", 0)}}
	New(o, &fakeCards{find: cards(1)}, &fakeSpecies{}).Scan(context.Background(), bytes.NewReader(cardPNG(t)))
	if o.calls != 2 {
		t.Errorf("OCR called %d times, want 2 (no retry)", o.calls)
	}
}

func TestScan_stillAsksForARetakeWhenBothNameReadsFail(t *testing.T) {
	o := &fakeOCR{results: []tesseract.Result{read("~~", 20), read("025/165", 0), read("##", 10)}}
	c := &fakeCards{search: cards(1)}
	got, err := New(o, c, &fakeSpecies{}).Scan(context.Background(), bytes.NewReader(cardPNG(t)))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got.Status != StatusRetake || got.Reason != ReasonNameUnreadable {
		t.Errorf("status, reason = %q, %q; want retake, name_unreadable", got.Status, got.Reason)
	}
}

func TestScan_aFailedNameRetryIsAnOCRError(t *testing.T) {
	o := &failingOnThirdRead{fakeOCR: fakeOCR{results: []tesseract.Result{read("~~", 20), read("025/165", 0)}}}
	_, err := New(o, &fakeCards{}, &fakeSpecies{}).Scan(context.Background(), bytes.NewReader(cardPNG(t)))
	if !errors.Is(err, ErrOCR) {
		t.Errorf("err = %v, want ErrOCR", err)
	}
}

// failingOnThirdRead answers the first two reads like fakeOCR and fails the third.
type failingOnThirdRead struct{ fakeOCR }

func (f *failingOnThirdRead) Read(ctx context.Context, png []byte, opts tesseract.Options) (tesseract.Result, error) {
	if f.calls >= 2 {
		f.calls++
		return tesseract.Result{}, errors.New("tesseract crashed")
	}
	return f.fakeOCR.Read(ctx, png, opts)
}

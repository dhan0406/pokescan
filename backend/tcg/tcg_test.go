package tcg

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dhan0406/pokescan/backend/apierr"
)

// fake serves the captured fixture and records what it was asked.
type fake struct {
	*httptest.Server
	mu      sync.Mutex
	queries []string
	keys    []string
}

func newFake(t *testing.T, status int, body []byte) *fake {
	t.Helper()
	f := &fake{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.queries = append(f.queries, r.URL.Query().Get("q"))
		f.keys = append(f.keys, r.Header.Get("X-Api-Key"))
		f.mu.Unlock()
		w.WriteHeader(status)
		w.Write(body)
	}))
	t.Cleanup(f.Close)
	return f
}

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/cards_pikachu_25.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFindCards_mapsFields(t *testing.T) {
	f := newFake(t, 200, fixture(t))
	c := NewClient(f.URL, "", f.Client())

	cards, err := c.FindCards(context.Background(), "Pikachu", "25")
	if err != nil {
		t.Fatalf("FindCards: %v", err)
	}
	if len(cards) == 0 {
		t.Fatal("no cards returned")
	}
	want := Card{
		ID:       "basep-25",
		Name:     "Flying Pikachu",
		Number:   "25",
		SetName:  "Wizards Black Star Promos",
		SetTotal: 53,
		ImageURL: "https://images.pokemontcg.io/basep/25_hires.png",
	}
	if cards[0] != want {
		t.Errorf("cards[0] = %+v, want %+v", cards[0], want)
	}
}

func TestFindCards_sendsNumberQuery(t *testing.T) {
	tests := []struct {
		number string
		want   string
	}{
		{"025", "number:25"},
		{"004", "number:4"},
		{"000", "number:0"},
		{"25", "number:25"},
		{"SWSH039", "number:SWSH039"},
		{"TG05", "number:TG05"},
	}
	for _, tt := range tests {
		t.Run(tt.number, func(t *testing.T) {
			f := newFake(t, 200, []byte(`{"data":[]}`))
			c := NewClient(f.URL, "", f.Client())
			if _, err := c.FindCards(context.Background(), "Pikachu", tt.number); err != nil {
				t.Fatalf("FindCards: %v", err)
			}
			if len(f.queries) != 1 || !strings.Contains(f.queries[0], tt.want) {
				t.Errorf("q = %q, want it to contain %q", f.queries, tt.want)
			}
		})
	}
}

func TestFindCards_quotesName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Farfetch'd", `name:"Farfetch'd"`},
		{"Mr. Mime", `name:"Mr. Mime"`},
		{`Say "hi"`, `name:"Say \"hi\""`},
		{`back\slash`, `name:"back\\slash"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, 200, []byte(`{"data":[]}`))
			c := NewClient(f.URL, "", f.Client())
			if _, err := c.FindCards(context.Background(), tt.name, "1"); err != nil {
				t.Fatalf("FindCards: %v", err)
			}
			if len(f.queries) != 1 || !strings.Contains(f.queries[0], tt.want) {
				t.Errorf("q = %q, want it to contain %s", f.queries, tt.want)
			}
		})
	}
}

func TestFindCards_emptyResult(t *testing.T) {
	f := newFake(t, 200, []byte(`{"data":[]}`))
	c := NewClient(f.URL, "", f.Client())
	cards, err := c.FindCards(context.Background(), "Pikachu", "25")
	if err != nil {
		t.Fatalf("FindCards: %v", err)
	}
	if cards == nil || len(cards) != 0 {
		t.Errorf("cards = %#v, want empty non-nil slice", cards)
	}
}

func TestFindCards_emptyInputsSkipRequest(t *testing.T) {
	for _, in := range [][2]string{{"", "25"}, {"Pikachu", ""}, {"", ""}} {
		f := newFake(t, 200, fixture(t))
		c := NewClient(f.URL, "", f.Client())
		cards, err := c.FindCards(context.Background(), in[0], in[1])
		if err != nil {
			t.Errorf("FindCards(%q,%q): %v", in[0], in[1], err)
		}
		if cards == nil || len(cards) != 0 {
			t.Errorf("FindCards(%q,%q) = %#v, want empty non-nil slice", in[0], in[1], cards)
		}
		if len(f.queries) != 0 {
			t.Errorf("FindCards(%q,%q) made %d requests, want 0", in[0], in[1], len(f.queries))
		}
	}
}

func TestFindCards_apiKeyHeader(t *testing.T) {
	f := newFake(t, 200, []byte(`{"data":[]}`))
	NewClient(f.URL, "secret", f.Client()).FindCards(context.Background(), "Pikachu", "25")
	NewClient(f.URL, "", f.Client()).FindCards(context.Background(), "Pikachu", "25")
	if len(f.keys) != 2 || f.keys[0] != "secret" || f.keys[1] != "" {
		t.Errorf("X-Api-Key values = %q, want [secret \"\"]", f.keys)
	}
}

func TestFindCards_errorsMapped(t *testing.T) {
	t.Run("404", func(t *testing.T) {
		f := newFake(t, 404, nil)
		_, err := NewClient(f.URL, "", f.Client()).FindCards(context.Background(), "A", "1")
		if !errors.Is(err, apierr.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("500", func(t *testing.T) {
		f := newFake(t, 500, nil)
		_, err := NewClient(f.URL, "", f.Client()).FindCards(context.Background(), "A", "1")
		if !errors.Is(err, apierr.ErrUpstream) {
			t.Errorf("err = %v, want ErrUpstream", err)
		}
	})
	t.Run("slow server", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(300 * time.Millisecond):
			case <-r.Context().Done():
			}
		}))
		defer srv.Close()
		hc := &http.Client{Timeout: 50 * time.Millisecond}
		_, err := NewClient(srv.URL, "", hc).FindCards(context.Background(), "A", "1")
		if !errors.Is(err, apierr.ErrTimeout) {
			t.Errorf("err = %v, want ErrTimeout", err)
		}
	})
}

func TestNormalizeNumber(t *testing.T) {
	tests := []struct{ in, want string }{
		{"025", "25"},
		{"004", "4"},
		{"000", "0"},
		{"0", "0"},
		{"165", "165"},
		{"SWSH039", "SWSH039"},
		{"TG05", "TG05"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeNumber(tt.in); got != tt.want {
			t.Errorf("NormalizeNumber(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFindCards_invalidNumberSkipsRequest(t *testing.T) {
	for _, number := range []string{"025/185", `25 OR name:charizard`, `25"`, "2*", "25)", "  ", "TG 05"} {
		t.Run(number, func(t *testing.T) {
			f := newFake(t, 200, fixture(t))
			cards, err := NewClient(f.URL, "", f.Client()).FindCards(context.Background(), "Pikachu", number)
			if err != nil {
				t.Fatalf("FindCards: %v", err)
			}
			if cards == nil || len(cards) != 0 {
				t.Errorf("cards = %#v, want empty non-nil slice", cards)
			}
			if len(f.queries) != 0 {
				t.Errorf("made %d requests (q=%q), want 0", len(f.queries), f.queries)
			}
		})
	}
}

func TestFindCards_trimsInputs(t *testing.T) {
	f := newFake(t, 200, []byte(`{"data":[]}`))
	c := NewClient(f.URL, "", f.Client())
	if _, err := c.FindCards(context.Background(), " Pikachu ", " 025 "); err != nil {
		t.Fatalf("FindCards: %v", err)
	}
	want := `name:"Pikachu" number:25`
	if len(f.queries) != 1 || f.queries[0] != want {
		t.Errorf("q = %q, want [%s]", f.queries, want)
	}
}

func TestFindCards_blankNameSkipsRequest(t *testing.T) {
	f := newFake(t, 200, fixture(t))
	cards, err := NewClient(f.URL, "", f.Client()).FindCards(context.Background(), "   ", "25")
	if err != nil || cards == nil || len(cards) != 0 || len(f.queries) != 0 {
		t.Errorf("got %#v, %v, %d requests; want empty slice, nil, 0", cards, err, len(f.queries))
	}
}

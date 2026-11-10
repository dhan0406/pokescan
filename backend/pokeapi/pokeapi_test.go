package pokeapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/dhan0406/pokescan/backend/apierr"
)

// fake serves fixtures by escaped URL path, 404s anything else, and records
// every escaped path it is asked for.
type fake struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
}

// newFake maps escaped request path -> fixture file name (without .json) or
// -> a status code via statuses.
func newFake(t *testing.T, routes map[string]string, statuses map[string]int) *fake {
	t.Helper()
	f := &fake{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.EscapedPath()
		f.mu.Lock()
		f.paths = append(f.paths, p)
		f.mu.Unlock()
		if code, ok := statuses[p]; ok {
			w.WriteHeader(code)
			return
		}
		name, ok := routes[p]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile("testdata/" + name + ".json")
		if err != nil {
			t.Errorf("fixture %s: %v", name, err)
			w.WriteHeader(500)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(f.Close)
	return f
}

func TestTypePokemon_returnsNames(t *testing.T) {
	f := newFake(t, map[string]string{"/api/v2/type/electric": "type_electric"}, nil)
	c := NewClient(f.URL, f.Client())

	got, err := c.TypePokemon(context.Background(), "electric")
	if err != nil {
		t.Fatalf("TypePokemon: %v", err)
	}

	var fx struct {
		Pokemon []json.RawMessage `json:"pokemon"`
	}
	b, _ := os.ReadFile("testdata/type_electric.json")
	json.Unmarshal(b, &fx)
	if len(got) != len(fx.Pokemon) {
		t.Errorf("got %d names, fixture has %d", len(got), len(fx.Pokemon))
	}
	have := map[string]bool{}
	for _, n := range got {
		have[n] = true
	}
	for _, want := range []string{"pikachu", "raichu"} {
		if !have[want] {
			t.Errorf("missing %q in result (%d names)", want, len(got))
		}
	}
}

func TestTypePokemon_lowercasesAndEscapes(t *testing.T) {
	f := newFake(t, map[string]string{"/api/v2/type/electric": "type_electric"}, nil)
	c := NewClient(f.URL, f.Client())

	if _, err := c.TypePokemon(context.Background(), "Electric"); err != nil {
		t.Fatalf("TypePokemon(Electric): %v", err)
	}
	if len(f.paths) != 1 || f.paths[0] != "/api/v2/type/electric" {
		t.Errorf("paths = %q, want [/api/v2/type/electric]", f.paths)
	}

	f.paths = nil
	_, err := c.TypePokemon(context.Background(), "../pokemon/1")
	if err == nil {
		t.Error("TypePokemon(../pokemon/1) succeeded, want an error")
	}
	for _, p := range f.paths {
		if !strings.HasPrefix(p, "/api/v2/type/") || strings.Contains(p, "/../") || p == "/api/v2/pokemon/1" {
			t.Errorf("request escaped the type endpoint: %q", p)
		}
	}
}

func TestTypePokemon_unknownTypeIsNotFound(t *testing.T) {
	f := newFake(t, nil, nil)
	_, err := NewClient(f.URL, f.Client()).TypePokemon(context.Background(), "nope")
	if !errors.Is(err, apierr.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

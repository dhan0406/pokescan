package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/dhan0406/pokescan/backend/apierr"
	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/scan"
)

type fakeScanner struct {
	out    scan.Outcome
	err    error
	calls  int
	gotImg []byte
	gotCtx context.Context
}

func (f *fakeScanner) Scan(ctx context.Context, img io.Reader) (scan.Outcome, error) {
	f.calls++
	f.gotCtx = ctx
	f.gotImg, _ = io.ReadAll(img)
	return f.out, f.err
}

type fakeSpecies struct {
	sp  pokeapi.Species
	err error
}

func (f *fakeSpecies) Species(ctx context.Context, idOrName string) (pokeapi.Species, error) {
	return f.sp, f.err
}

var pikachu = pokeapi.Species{
	ID: 25, Name: "pikachu", Types: []string{"electric"},
	Stats:      map[string]int{"hp": 35},
	FlavorText: "It keeps its tail raised.",
	Evolution:  pokeapi.EvolutionNode{Name: "pichu", EvolvesTo: []pokeapi.EvolutionNode{{Name: "pikachu"}}},
}

func newTestServer(sc Scanner, sp scan.SpeciesGetter) http.Handler {
	return New(sc, sp, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func do(t *testing.T, h http.Handler, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not a JSON object: %v: %s", err, rec.Body.String())
	}
	return m
}

func keys(m map[string]any) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	sort.Strings(k)
	return k
}

func TestPing(t *testing.T) {
	rec := do(t, newTestServer(&fakeScanner{}, &fakeSpecies{}), "GET", "/ping", nil, "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Errorf("GET /ping = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPokemon_ok(t *testing.T) {
	rec := do(t, newTestServer(&fakeScanner{}, &fakeSpecies{sp: pikachu}), "GET", "/v1/pokemon/pikachu", nil, "")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	m := decodeMap(t, rec)
	want := []string{"evolution", "flavorText", "id", "name", "stats", "types"}
	if got := keys(m); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("keys = %v, want %v", got, want)
	}
	if m["id"].(float64) != 25 {
		t.Errorf("id = %v, want 25", m["id"])
	}
}

func TestPokemon_noNullArrays(t *testing.T) {
	sp := pokeapi.Species{ID: 132, Name: "ditto", Evolution: pokeapi.EvolutionNode{Name: "ditto"}}
	rec := do(t, newTestServer(&fakeScanner{}, &fakeSpecies{sp: sp}), "GET", "/v1/pokemon/ditto", nil, "")
	body := rec.Body.String()
	if strings.Contains(body, "null") {
		t.Errorf("body contains null: %s", body)
	}
	for _, want := range []string{`"types":[]`, `"evolvesTo":[]`, `"stats":{}`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
}

var errorCases = []struct {
	err    error
	status int
	code   string
}{
	{fmt.Errorf("%w: GET https://pokeapi.co/api/v2/pokemon-species/x", apierr.ErrNotFound), 404, "not_found"},
	{fmt.Errorf("%w: GET https://pokeapi.co/slow", apierr.ErrTimeout), 504, "upstream_timeout"},
	{fmt.Errorf("%w: GET https://pokeapi.co/x: status 500", apierr.ErrUpstream), 502, "upstream_error"},
	{errors.New("something odd at https://example.com"), 500, "internal_error"},
}

func TestPokemon_errorMapping(t *testing.T) {
	for _, tc := range errorCases {
		t.Run(tc.code, func(t *testing.T) {
			rec := do(t, newTestServer(&fakeScanner{}, &fakeSpecies{err: tc.err}), "GET", "/v1/pokemon/x", nil, "")
			if rec.Code != tc.status {
				t.Errorf("status = %d, want %d", rec.Code, tc.status)
			}
			var body ErrorBody
			json.Unmarshal(rec.Body.Bytes(), &body)
			if body.Code != tc.code || body.Message == "" {
				t.Errorf("body = %+v, want code %s and a message", body, tc.code)
			}
		})
	}
}

func TestErrors_noURLsOrRawErrors(t *testing.T) {
	for _, tc := range errorCases {
		rec := do(t, newTestServer(&fakeScanner{}, &fakeSpecies{err: tc.err}), "GET", "/v1/pokemon/x", nil, "")
		var body ErrorBody
		json.Unmarshal(rec.Body.Bytes(), &body)
		if strings.Contains(body.Message, "http") || strings.Contains(body.Message, tc.err.Error()) {
			t.Errorf("%s message leaks internals: %q", tc.code, body.Message)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newTestServer(&fakeScanner{}, &fakeSpecies{sp: pikachu})
	for _, r := range [][2]string{{"POST", "/ping"}, {"GET", "/v1/scan"}, {"DELETE", "/v1/pokemon/x"}} {
		rec := do(t, h, r[0], r[1], nil, "")
		var body ErrorBody
		json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != 405 || body.Code != "method_not_allowed" {
			t.Errorf("%s %s = %d %s, want 405 method_not_allowed", r[0], r[1], rec.Code, rec.Body.String())
		}
	}
}

func TestResponsesAreJSON(t *testing.T) {
	h := newTestServer(&fakeScanner{}, &fakeSpecies{sp: pikachu})
	for _, r := range [][2]string{{"GET", "/ping"}, {"GET", "/v1/pokemon/pikachu"}, {"POST", "/ping"}} {
		rec := do(t, h, r[0], r[1], nil, "")
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s %s Content-Type = %q, want application/json", r[0], r[1], ct)
		}
	}
	rec := do(t, newTestServer(&fakeScanner{}, &fakeSpecies{err: apierr.ErrNotFound}), "GET", "/v1/pokemon/x", nil, "")
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("error Content-Type = %q, want application/json", ct)
	}
}

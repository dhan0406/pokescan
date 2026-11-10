package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
	"testing"
	"time"

	"github.com/dhan0406/pokescan/backend/apierr"
	"github.com/dhan0406/pokescan/backend/imageprep"
	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/scan"
	"github.com/dhan0406/pokescan/backend/tcg"
)

// upload builds a multipart body with one file part.
func upload(t *testing.T, field string, data []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, "card.png")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(data)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func postScan(t *testing.T, sc *fakeScanner, field string, data []byte) map[string]any {
	t.Helper()
	body, ct := upload(t, field, data)
	rec := do(t, newTestServer(sc, &fakeSpecies{}), "POST", "/v1/scan", body, ct)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	return decodeMap(t, rec)
}

var card = tcg.Card{ID: "base1-58", Name: "Pikachu", Number: "58", SetName: "Base", SetTotal: 102, ImageURL: "https://img/58.png", DexNumber: 25}

func TestScan_match(t *testing.T) {
	sp := pikachu
	m := postScan(t, &fakeScanner{out: scan.Outcome{Status: scan.StatusMatch, Card: &card, Pokemon: &sp}}, "image", []byte("img"))
	if got := strings.Join(keys(m), ","); got != "card,pokemon,status" {
		t.Errorf("keys = %s, want card,pokemon,status", got)
	}
	c := m["card"].(map[string]any)
	if got := strings.Join(keys(c), ","); got != "dexNumber,id,imageUrl,name,number,setName,setTotal" {
		t.Errorf("card keys = %s", got)
	}
	if c["dexNumber"].(float64) != 25 || m["status"] != "match" {
		t.Errorf("body = %v", m)
	}
}

func TestScan_choose(t *testing.T) {
	m := postScan(t, &fakeScanner{out: scan.Outcome{Status: scan.StatusChoose, Candidates: []tcg.Card{card, card}}}, "image", []byte("img"))
	if got := strings.Join(keys(m), ","); got != "candidates,status" {
		t.Errorf("keys = %s, want candidates,status", got)
	}
	if n := len(m["candidates"].([]any)); n != 2 {
		t.Errorf("%d candidates, want 2", n)
	}
}

func TestScan_retakeWithAndWithoutCandidates(t *testing.T) {
	m := postScan(t, &fakeScanner{out: scan.Outcome{Status: scan.StatusRetake, Reason: scan.ReasonNameUnreadable, Candidates: []tcg.Card{card}}}, "image", []byte("img"))
	if got := strings.Join(keys(m), ","); got != "candidates,reason,status" {
		t.Errorf("with candidates: keys = %s", got)
	}
	m = postScan(t, &fakeScanner{out: scan.Outcome{Status: scan.StatusRetake, Reason: scan.ReasonCardUnreadable}}, "image", []byte("img"))
	if got := strings.Join(keys(m), ","); got != "reason,status" {
		t.Errorf("without candidates: keys = %s", got)
	}
	if m["reason"] != "card_unreadable" {
		t.Errorf("reason = %v", m["reason"])
	}
}

func TestScan_trainerMatchOmitsPokemon(t *testing.T) {
	trainer := tcg.Card{ID: "swsh35-62", Name: "Professor's Research"}
	m := postScan(t, &fakeScanner{out: scan.Outcome{Status: scan.StatusMatch, Card: &trainer}}, "image", []byte("img"))
	if _, ok := m["pokemon"]; ok {
		t.Errorf("trainer match has pokemon: %v", m)
	}
}

func TestScan_noNullArraysInPokemon(t *testing.T) {
	sp := pokeapi.Species{ID: 132, Name: "ditto", Evolution: pokeapi.EvolutionNode{Name: "ditto"}}
	body, ct := upload(t, "image", []byte("img"))
	rec := do(t, newTestServer(&fakeScanner{out: scan.Outcome{Status: scan.StatusMatch, Card: &card, Pokemon: &sp}}, &fakeSpecies{}), "POST", "/v1/scan", body, ct)
	if strings.Contains(rec.Body.String(), "null") {
		t.Errorf("body contains null: %s", rec.Body.String())
	}
}

func TestScan_passesImageBytes(t *testing.T) {
	sc := &fakeScanner{out: scan.Outcome{Status: scan.StatusRetake, Reason: scan.ReasonCardUnreadable}}
	data := []byte("\x89PNG pretend image bytes \x00\x01")
	postScan(t, sc, "image", data)
	if !bytes.Equal(sc.gotImg, data) {
		t.Errorf("scanner got %q, want %q", sc.gotImg, data)
	}
}

func TestScan_badRequests(t *testing.T) {
	wrongField, wrongCT := upload(t, "file", []byte("img"))
	empty, emptyCT := upload(t, "image", nil)
	tests := []struct {
		name string
		body io.Reader
		ct   string
	}{
		{"no body", nil, ""},
		{"wrong field name", wrongField, wrongCT},
		{"zero-byte image", empty, emptyCT},
		{"json body", strings.NewReader(`{"image":"x"}`), "application/json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &fakeScanner{}
			rec := do(t, newTestServer(sc, &fakeSpecies{}), "POST", "/v1/scan", tt.body, tt.ct)
			var body ErrorBody
			json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != 400 || body.Code != "bad_image" {
				t.Errorf("= %d %s, want 400 bad_image", rec.Code, rec.Body.String())
			}
			if sc.calls != 0 {
				t.Errorf("scanner called %d times, want 0", sc.calls)
			}
		})
	}
}

func TestScan_tooLarge(t *testing.T) {
	sc := &fakeScanner{}
	body, ct := upload(t, "image", bytes.Repeat([]byte("a"), 11<<20))
	rec := do(t, newTestServer(sc, &fakeSpecies{}), "POST", "/v1/scan", body, ct)
	var eb ErrorBody
	json.Unmarshal(rec.Body.Bytes(), &eb)
	if rec.Code != 413 || eb.Code != "image_too_large" {
		t.Errorf("= %d %s, want 413 image_too_large", rec.Code, rec.Body.String())
	}
	if sc.calls != 0 {
		t.Errorf("scanner called %d times, want 0", sc.calls)
	}
}

func TestScan_deadline(t *testing.T) {
	sc := &fakeScanner{out: scan.Outcome{Status: scan.StatusRetake, Reason: scan.ReasonCardUnreadable}}
	postScan(t, sc, "image", []byte("img"))
	dl, ok := sc.gotCtx.Deadline()
	if !ok {
		t.Fatal("scan context has no deadline")
	}
	if left := time.Until(dl); left > 20*time.Second || left < 19*time.Second {
		t.Errorf("deadline in %v, want about 20s", left)
	}
}

func TestScan_scannerErrorsMapped(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("%w: gif", imageprep.ErrBadImage), 400, "bad_image"},
		{fmt.Errorf("%w: exit 1", scan.ErrOCR), 500, "ocr_failed"},
		{fmt.Errorf("%w: %w", scan.ErrOCR, apierr.ErrTimeout), 504, "upstream_timeout"},
		{fmt.Errorf("%w: GET https://api.pokemontcg.io", apierr.ErrTimeout), 504, "upstream_timeout"},
		{fmt.Errorf("%w: status 503", apierr.ErrUpstream), 502, "upstream_error"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			body, ct := upload(t, "image", []byte("img"))
			rec := do(t, newTestServer(&fakeScanner{err: tt.err}, &fakeSpecies{}), "POST", "/v1/scan", body, ct)
			var eb ErrorBody
			json.Unmarshal(rec.Body.Bytes(), &eb)
			if rec.Code != tt.status || eb.Code != tt.code {
				t.Errorf("= %d %s, want %d %s", rec.Code, rec.Body.String(), tt.status, tt.code)
			}
		})
	}
}

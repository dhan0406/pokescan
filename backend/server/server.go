// Package server is the Pokescan HTTP API.
package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhan0406/pokescan/backend/imageprep"
	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/scan"
	"github.com/dhan0406/pokescan/backend/tcg"
)

// Scanner runs a scan on an uploaded image.
type Scanner interface {
	Scan(ctx context.Context, img io.Reader) (scan.Outcome, error)
}

type server struct {
	scanner Scanner
	species scan.SpeciesGetter
	log     *slog.Logger
	saveDir string // when set, scanned photos are saved here
}

// Option configures optional server behavior.
type Option func(*server)

// WithSaveUploads saves every successfully scanned photo into dir, named by
// time and outcome, so real photos can be used to tune OCR. An empty dir turns
// it off. Save failures are logged and never change the response.
func WithSaveUploads(dir string) Option {
	return func(s *server) { s.saveDir = dir }
}

// New returns the API handler.
func New(sc Scanner, species scan.SpeciesGetter, log *slog.Logger, opts ...Option) http.Handler {
	s := &server{scanner: sc, species: species, log: log}
	for _, opt := range opts {
		opt(s)
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /ping", s.ping)
	mux.HandleFunc("GET /v1/pokemon/{idOrName}", s.pokemon)
	mux.HandleFunc("POST /v1/scan", s.scan)

	// Method-less patterns are less specific, so they only catch the wrong
	// methods on known paths and answer with a JSON 405.
	notAllowed := func(w http.ResponseWriter, r *http.Request) { writeAPIError(w, errMethodNotAllowed) }
	mux.HandleFunc("/ping", notAllowed)
	mux.HandleFunc("/v1/scan", notAllowed)
	mux.HandleFunc("/v1/pokemon/{idOrName}", notAllowed)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeAPIError(w, errNotFound) })
	return mux
}

func (s *server) ping(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) pokemon(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sp, err := s.species.Species(ctx, r.PathValue("idOrName"))
	if err != nil {
		writeError(w, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, toPokemon(sp))
}

const (
	maxUpload = 10 << 20 // 10 MB
	// requestTimeout bounds the work a handler does after reading the request,
	// so a slow upstream gives a 504 instead of a dropped connection.
	requestTimeout = 20 * time.Second
)

func (s *server) scan(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, s.log, err)
			return
		}
		writeError(w, s.log, fmt.Errorf("%w: %v", imageprep.ErrBadImage, err))
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, s.log, fmt.Errorf("%w: %v", imageprep.ErrBadImage, err))
		return
	}
	defer file.Close()
	if header.Size == 0 {
		writeError(w, s.log, fmt.Errorf("%w: empty image", imageprep.ErrBadImage))
		return
	}

	// The scanner reads the upload as a stream. When saving is on, hold the
	// bytes (at most 10 MB) so the same photo can be written out afterwards.
	var img io.Reader = file
	var saved []byte
	if s.saveDir != "" {
		saved, err = io.ReadAll(file)
		if err != nil {
			writeError(w, s.log, fmt.Errorf("%w: %v", imageprep.ErrBadImage, err))
			return
		}
		img = bytes.NewReader(saved)
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	out, err := s.scanner.Scan(ctx, img)
	if err != nil {
		writeError(w, s.log, err)
		return
	}
	if saved != nil {
		s.saveUpload(saved, out.Status)
	}
	writeJSON(w, http.StatusOK, toScanResponse(out))
}

// saveUpload writes a scanned photo to the save directory as
// <UTC time>-<status>.<ext>. Problems are logged; they never affect the response.
func (s *server) saveUpload(data []byte, status string) {
	if err := os.MkdirAll(s.saveDir, 0o700); err != nil {
		s.log.Warn("could not create upload directory", "dir", s.saveDir, "err", err)
		return
	}
	ext := ".bin"
	switch http.DetectContentType(data) {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	}
	name := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + safeLabel(status) + ext

	f, err := os.OpenFile(filepath.Join(s.saveDir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		s.log.Warn("could not save upload", "err", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		s.log.Warn("could not save upload", "err", err)
	}
}

// safeLabel keeps lowercase letters, digits and underscores so a status can
// never add a path separator to a file name.
func safeLabel(s string) string {
	label := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, s)
	if label == "" {
		return "unknown"
	}
	return label
}

func toScanResponse(o scan.Outcome) ScanResponse {
	resp := ScanResponse{Status: o.Status, Reason: o.Reason}
	if o.Card != nil {
		c := toCard(*o.Card)
		resp.Card = &c
	}
	if o.Pokemon != nil {
		p := toPokemon(*o.Pokemon)
		resp.Pokemon = &p
	}
	for _, c := range o.Candidates {
		resp.Candidates = append(resp.Candidates, toCard(c))
	}
	return resp
}

func toCard(c tcg.Card) Card {
	return Card{
		ID:        c.ID,
		Name:      c.Name,
		Number:    c.Number,
		SetName:   c.SetName,
		SetTotal:  c.SetTotal,
		ImageURL:  c.ImageURL,
		DexNumber: c.DexNumber,
	}
}

// toPokemon maps a species to its API shape, using empty arrays and maps
// instead of null so generated client types can rely on them.
func toPokemon(sp pokeapi.Species) Pokemon {
	p := Pokemon{
		ID:         sp.ID,
		Name:       sp.Name,
		Types:      append([]string{}, sp.Types...),
		Stats:      map[string]int{},
		FlavorText: sp.FlavorText,
		Evolution:  toEvolution(sp.Evolution),
	}
	for k, v := range sp.Stats {
		p.Stats[k] = v
	}
	return p
}

func toEvolution(n pokeapi.EvolutionNode) Evolution {
	e := Evolution{Name: n.Name, EvolvesTo: []Evolution{}}
	for _, next := range n.EvolvesTo {
		e.EvolvesTo = append(e.EvolvesTo, toEvolution(next))
	}
	return e
}

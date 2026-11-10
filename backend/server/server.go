// Package server is the Pokescan HTTP API.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
}

// New returns the API handler.
func New(sc Scanner, species scan.SpeciesGetter, log *slog.Logger) http.Handler {
	s := &server{scanner: sc, species: species, log: log}
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
	sp, err := s.species.Species(r.Context(), r.PathValue("idOrName"))
	if err != nil {
		writeError(w, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, toPokemon(sp))
}

const (
	maxUpload   = 10 << 20 // 10 MB
	scanTimeout = 20 * time.Second
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

	ctx, cancel := context.WithTimeout(r.Context(), scanTimeout)
	defer cancel()
	out, err := s.scanner.Scan(ctx, file)
	if err != nil {
		writeError(w, s.log, err)
		return
	}
	writeJSON(w, http.StatusOK, toScanResponse(out))
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

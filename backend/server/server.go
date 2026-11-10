// Package server is the Pokescan HTTP API.
package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/scan"
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

func (s *server) scan(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, ErrorBody{Code: "not_implemented", Message: "Not implemented yet."})
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

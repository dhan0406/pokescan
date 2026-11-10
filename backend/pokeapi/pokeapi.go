// Package pokeapi is a client for PokéAPI (species, evolutions, types).
package pokeapi

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dhan0406/pokescan/backend/apierr"
	"github.com/dhan0406/pokescan/backend/httpjson"
)

const (
	defaultBaseURL = "https://pokeapi.co"
	defaultTimeout = 10 * time.Second
)

// Client talks to PokéAPI.
type Client struct {
	baseURL string
	hc      *http.Client
}

// NewClient builds a client for the given base URL and HTTP client.
func NewClient(baseURL string, hc *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), hc: hc}
}

// New builds a client for the real API with a 10s timeout.
func New() *Client {
	return NewClient(defaultBaseURL, &http.Client{Timeout: defaultTimeout})
}

// slug is what a PokéAPI resource name or ID looks like.
var slug = regexp.MustCompile(`^[a-z0-9-]+$`)

// endpoint builds /api/v2/<resource>/<name> from a name or ID that is trimmed
// and lowercased. A name that is not a valid slug (empty, or containing "/",
// "?", spaces and so on) returns ErrNotFound without any request, so a
// caller-supplied name can never reach a list endpoint or another resource.
func (c *Client) endpoint(resource, name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !slug.MatchString(name) {
		return "", fmt.Errorf("%w: invalid %s name %q", apierr.ErrNotFound, resource, name)
	}
	return c.baseURL + "/api/v2/" + resource + "/" + name, nil
}

// TypePokemon returns the names of the Pokémon of a type.
func (c *Client) TypePokemon(ctx context.Context, typeName string) ([]string, error) {
	var resp struct {
		Pokemon []struct {
			Pokemon struct {
				Name string `json:"name"`
			} `json:"pokemon"`
		} `json:"pokemon"`
	}
	u, err := c.endpoint("type", typeName)
	if err != nil {
		return nil, err
	}
	if err := httpjson.Get(ctx, c.hc, u, nil, &resp); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(resp.Pokemon))
	for _, p := range resp.Pokemon {
		names = append(names, p.Pokemon.Name)
	}
	return names, nil
}

// EvolutionNode is one species in an evolution chain.
type EvolutionNode struct {
	Name      string
	EvolvesTo []EvolutionNode
}

// Species is the small slice of Pokémon data the app needs.
type Species struct {
	ID         int
	Name       string
	Types      []string
	Stats      map[string]int // hp, attack, defense, special-attack, special-defense, speed
	FlavorText string         // first English entry, whitespace normalized
	Evolution  EvolutionNode  // root of the evolution chain
}

// Species returns details for a Pokémon species by ID or name. It makes three
// upstream calls (species, evolution chain, pokemon) and fails if any fails.
func (c *Client) Species(ctx context.Context, idOrName string) (Species, error) {
	var sp struct {
		ID                int    `json:"id"`
		Name              string `json:"name"`
		FlavorTextEntries []struct {
			Text     string `json:"flavor_text"`
			Language struct {
				Name string `json:"name"`
			} `json:"language"`
		} `json:"flavor_text_entries"`
		EvolutionChain struct {
			URL string `json:"url"`
		} `json:"evolution_chain"`
	}
	spURL, err := c.endpoint("pokemon-species", idOrName)
	if err != nil {
		return Species{}, err
	}
	if err := httpjson.Get(ctx, c.hc, spURL, nil, &sp); err != nil {
		return Species{}, err
	}

	chainID, err := lastPathSegment(sp.EvolutionChain.URL)
	if err != nil {
		return Species{}, err
	}
	chainURL, err := c.endpoint("evolution-chain", chainID)
	if err != nil {
		return Species{}, err
	}
	var chain struct {
		Chain chainLink `json:"chain"`
	}
	if err := httpjson.Get(ctx, c.hc, chainURL, nil, &chain); err != nil {
		return Species{}, err
	}

	var pk struct {
		Types []struct {
			Type struct {
				Name string `json:"name"`
			} `json:"type"`
		} `json:"types"`
		Stats []struct {
			BaseStat int `json:"base_stat"`
			Stat     struct {
				Name string `json:"name"`
			} `json:"stat"`
		} `json:"stats"`
	}
	pkURL, err := c.endpoint("pokemon", strconv.Itoa(sp.ID))
	if err != nil {
		return Species{}, err
	}
	if err := httpjson.Get(ctx, c.hc, pkURL, nil, &pk); err != nil {
		return Species{}, err
	}

	out := Species{
		ID:        sp.ID,
		Name:      sp.Name,
		Types:     make([]string, 0, len(pk.Types)),
		Stats:     make(map[string]int, len(pk.Stats)),
		Evolution: chain.Chain.toNode(),
	}
	for _, t := range pk.Types {
		out.Types = append(out.Types, t.Type.Name)
	}
	for _, s := range pk.Stats {
		out.Stats[s.Stat.Name] = s.BaseStat
	}
	for _, e := range sp.FlavorTextEntries {
		if e.Language.Name == "en" {
			out.FlavorText = strings.Join(strings.Fields(e.Text), " ")
			break
		}
	}
	return out, nil
}

// chainLink is one node of PokéAPI's nested evolution-chain JSON.
type chainLink struct {
	Species struct {
		Name string `json:"name"`
	} `json:"species"`
	EvolvesTo []chainLink `json:"evolves_to"`
}

func (l chainLink) toNode() EvolutionNode {
	n := EvolutionNode{Name: l.Species.Name}
	for _, next := range l.EvolvesTo {
		n.EvolvesTo = append(n.EvolvesTo, next.toNode())
	}
	return n
}

// lastPathSegment returns the final path segment of a PokéAPI resource URL
// such as ".../evolution-chain/10/", requiring it to be a number. The chain
// is then requested from our own base URL, not the absolute URL in the data.
func lastPathSegment(rawURL string) (string, error) {
	seg := path.Base(strings.TrimRight(rawURL, "/"))
	if _, err := strconv.Atoi(seg); err != nil {
		return "", fmt.Errorf("%w: bad evolution chain URL %q", apierr.ErrUpstream, rawURL)
	}
	return seg, nil
}

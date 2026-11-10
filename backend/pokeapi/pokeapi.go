// Package pokeapi is a client for PokéAPI (species, evolutions, types).
package pokeapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

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

// endpoint builds /api/v2/<resource>/<name> with the name lowercased and
// path-escaped, so a caller-supplied name can never reach another endpoint.
func (c *Client) endpoint(resource, name string) string {
	return c.baseURL + "/api/v2/" + resource + "/" + url.PathEscape(strings.ToLower(name))
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
	if err := httpjson.Get(ctx, c.hc, c.endpoint("type", typeName), nil, &resp); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(resp.Pokemon))
	for _, p := range resp.Pokemon {
		names = append(names, p.Pokemon.Name)
	}
	return names, nil
}

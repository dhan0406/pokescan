// Package tcg is a client for the Pokémon TCG API (card lookup).
package tcg

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dhan0406/pokescan/backend/httpjson"
)

const (
	defaultBaseURL = "https://api.pokemontcg.io"
	defaultTimeout = 10 * time.Second
	pageSize       = "10"
	selectFields   = "id,name,number,set,images"
)

// Card is the small slice of a TCG card the app needs.
type Card struct {
	ID, Name, Number, SetName string
	SetTotal                  int
	ImageURL                  string
}

// Client talks to the Pokémon TCG API.
type Client struct {
	baseURL string
	apiKey  string
	hc      *http.Client
}

// NewClient builds a client for the given base URL and HTTP client.
// An empty apiKey means requests are sent without one.
func NewClient(baseURL, apiKey string, hc *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, hc: hc}
}

// New builds a client for the real API with a 10s timeout.
func New(apiKey string) *Client {
	return NewClient(defaultBaseURL, apiKey, &http.Client{Timeout: defaultTimeout})
}

// apiResponse is the part of the upstream JSON we read.
type apiResponse struct {
	Data []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Number string `json:"number"`
		Set    struct {
			Name         string `json:"name"`
			PrintedTotal int    `json:"printedTotal"`
		} `json:"set"`
		Images struct {
			Large string `json:"large"`
		} `json:"images"`
	} `json:"data"`
}

// FindCards looks up cards by name and collector number. Empty name or number
// returns an empty slice without calling the API. No match is an empty slice,
// not an error.
func (c *Client) FindCards(ctx context.Context, name, number string) ([]Card, error) {
	if name == "" || number == "" {
		return []Card{}, nil
	}

	q := url.Values{}
	q.Set("q", `name:"`+escapeQuoted(name)+`" number:`+NormalizeNumber(number))
	q.Set("pageSize", pageSize)
	q.Set("select", selectFields)

	var header http.Header
	if c.apiKey != "" {
		header = http.Header{"X-Api-Key": {c.apiKey}}
	}

	var resp apiResponse
	if err := httpjson.Get(ctx, c.hc, c.baseURL+"/v2/cards?"+q.Encode(), header, &resp); err != nil {
		return nil, err
	}

	cards := make([]Card, 0, len(resp.Data))
	for _, d := range resp.Data {
		cards = append(cards, Card{
			ID:       d.ID,
			Name:     d.Name,
			Number:   d.Number,
			SetName:  d.Set.Name,
			SetTotal: d.Set.PrintedTotal,
			ImageURL: d.Images.Large,
		})
	}
	return cards, nil
}

// escapeQuoted escapes backslashes and double quotes for use inside a quoted
// TCG query value.
func escapeQuoted(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// NormalizeNumber strips leading zeros from purely numeric collector numbers,
// because the TCG API stores "25", not "025". Numbers with letters (SWSH039,
// TG05) are returned unchanged.
func NormalizeNumber(s string) string {
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return s
	}
	if t := strings.TrimLeft(s, "0"); t != "" {
		return t
	}
	return "0"
}

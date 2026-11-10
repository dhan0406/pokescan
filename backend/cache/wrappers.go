package cache

import (
	"context"
	"strings"
	"time"

	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/tcg"
)

// CardFinder is what CachedCards wraps; *tcg.Client satisfies it.
type CardFinder interface {
	FindCards(ctx context.Context, name, number string) ([]tcg.Card, error)
}

// SpeciesGetter is what CachedSpecies wraps; *pokeapi.Client satisfies it.
type SpeciesGetter interface {
	Species(ctx context.Context, idOrName string) (pokeapi.Species, error)
}

// CachedCards caches successful FindCards results. Errors are never stored;
// an empty result is a success and is stored.
type CachedCards struct {
	next  CardFinder
	cache *Cache[[]tcg.Card]
}

// NewCards wraps next with a TTL cache.
func NewCards(next CardFinder, ttl time.Duration, now func() time.Time) *CachedCards {
	return &CachedCards{next: next, cache: New[[]tcg.Card](ttl, now)}
}

// FindCards implements CardFinder.
func (c *CachedCards) FindCards(ctx context.Context, name, number string) ([]tcg.Card, error) {
	key := strings.ToLower(strings.TrimSpace(name)) + "|" + tcg.NormalizeNumber(strings.TrimSpace(number))
	if v, ok := c.cache.Get(key); ok {
		return v, nil
	}
	v, err := c.next.FindCards(ctx, name, number)
	if err != nil {
		return nil, err
	}
	c.cache.Set(key, v)
	return v, nil
}

// CachedSpecies caches successful Species results. Errors are never stored.
type CachedSpecies struct {
	next  SpeciesGetter
	cache *Cache[pokeapi.Species]
}

// NewSpecies wraps next with a TTL cache.
func NewSpecies(next SpeciesGetter, ttl time.Duration, now func() time.Time) *CachedSpecies {
	return &CachedSpecies{next: next, cache: New[pokeapi.Species](ttl, now)}
}

// Species implements SpeciesGetter.
func (c *CachedSpecies) Species(ctx context.Context, idOrName string) (pokeapi.Species, error) {
	key := strings.ToLower(strings.TrimSpace(idOrName))
	if v, ok := c.cache.Get(key); ok {
		return v, nil
	}
	v, err := c.next.Species(ctx, idOrName)
	if err != nil {
		return pokeapi.Species{}, err
	}
	c.cache.Set(key, v)
	return v, nil
}

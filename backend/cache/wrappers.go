package cache

import (
	"context"
	"strings"
	"time"

	"github.com/dhan0406/pokescan/backend/pokeapi"
)

// SpeciesGetter is what CachedSpecies wraps; *pokeapi.Client satisfies it.
type SpeciesGetter interface {
	Species(ctx context.Context, idOrName string) (pokeapi.Species, error)
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

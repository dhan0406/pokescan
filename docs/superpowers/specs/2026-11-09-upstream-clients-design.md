# Upstream clients and cache design

Date: 2026-11-09
Status: draft for review
Parent: [Pokescan design](2026-11-07-pokescan-design.md) (step "TCG API and PokéAPI clients" of the build order)

## Purpose

Two Go clients for the third-party APIs the scan pipeline depends on, plus an in-memory TTL cache in front of them. The clients are built and tested in isolation, before the scan handler exists. This is also a learning goal: HTTP clients, timeouts and interfaces in Go.

## Decisions

| Area | Decision |
|---|---|
| Return types | Thin typed structs with only the fields the app needs. Raw upstream JSON never leaves a client. |
| Dependencies | Standard library only. |
| Packages | One package per client (`tcg`, `pokeapi`), plus a small `apierr` package for shared errors and a `cache` package. |
| Cache | Decorator that implements the same interface as the client it wraps. Clients know nothing about caching. |
| Testing | Clients against `httptest` servers. Cache against a counting fake. |

## Layout

```
backend/
  apierr/     ErrNotFound, ErrTimeout, ErrUpstream
  tcg/        Client, Card
  pokeapi/    Client, Species, EvolutionNode
  cache/      generic TTL store + cached wrappers
```

## Common client rules

- Constructor takes a base URL and a `*http.Client`, so tests can point at `httptest.NewServer`. A `New` helper supplies a default `http.Client` with an explicit timeout (10s, configurable). No use of `http.DefaultClient`.
- Every method takes a `context.Context` and builds requests with it, so a cancelled scan cancels the upstream call.
- Errors are wrapped with `%w` around sentinels from `apierr`: HTTP 404 gives `ErrNotFound`, a deadline or timeout gives `ErrTimeout`, anything else (5xx, bad JSON, network failure) gives `ErrUpstream`. The scan handler maps these to its `{code, message}` response.
- Response bodies are always closed. Non-2xx responses are checked before decoding.

## `tcg` client

```go
func (c *Client) FindCards(ctx context.Context, name, number string) ([]Card, error)

type Card struct {
    ID, Name, Number, SetName string
    SetTotal                  int
    ImageURL                  string // large image
}
```

- Calls `GET /v2/cards?q=name:"<name>" number:<number>` with `select` limited to the fields above.
- **The TCG API stores numbers without leading zeros.** `number:025` returns nothing and `number:25` works (checked against the live API on 2026-11-09). The client strips leading zeros from purely numeric collector numbers before querying. Numbers with letter prefixes (`SWSH123`, `TG05`) are passed through unchanged.
- Optional API key sent as the `X-Api-Key` header when configured. It is not required.
- An empty result is not an error: it returns an empty slice. The scan handler decides what that means.
- The API was observed to be occasionally slow (one request produced no response within 15s; a retry took under 0.4s), which is why timeouts are explicit.

## `pokeapi` client

```go
func (c *Client) Species(ctx context.Context, idOrName string) (Species, error)
func (c *Client) TypePokemon(ctx context.Context, typeName string) ([]string, error)

type Species struct {
    ID        int
    Name      string
    Types     []string
    Stats     map[string]int  // hp, attack, defense, special-attack, special-defense, speed
    FlavorText string         // first English entry, whitespace normalized
    Evolution EvolutionNode   // root of the chain
}

type EvolutionNode struct {
    Name      string
    EvolvesTo []EvolutionNode
}
```

- `Species` takes three upstream calls: `/pokemon-species/{id}` (flavor text, evolution chain URL), `/evolution-chain/{id}`, and `/pokemon/{id}` (types and stats, which are not on the species endpoint). The nested evolution chain is flattened into `EvolutionNode`.
- Evolution trigger details (level, item, and so on) are out of scope for v1.
- `TypePokemon` calls `/type/{type}` and returns Pokémon names.

## `cache` package

```go
type Cache[V any]   // generic TTL store: Get(key) (V, bool), Set(key, V)
```

- Thin wrappers (`CachedCards`, `CachedSpecies`) implement the same method as the client they wrap and hold a `Cache`. On a hit they return the stored value. On a miss they call the wrapped client and store the result only if it succeeded.
- The handler depends on small interfaces (`CardFinder`, `SpeciesGetter`) defined in the handler's package, not in the clients, so it does not know whether it has the real client or the cached one.
- Cache keys are normalized (lowercased name, leading zeros stripped from the number) so that equivalent requests share an entry.
- Time comes from an injectable clock so TTL tests do not sleep.
- Safe for concurrent use (mutex).

## Known limits in v1

These are deliberate and should be called out, not fixed now:

- **No `singleflight`.** Two simultaneous misses for the same key both call upstream.
- **No negative caching.** Errors and empty results are not cached, so repeated bad scans hit the upstream each time.
- **No size cap or background sweep.** Expired entries are removed when read, so a long-running worker could slowly grow.
- **Cold starts empty the cache.** Each new Runpod worker starts empty.
- No retries. A failed call returns its error and the app offers a retry.

## Testing

- Table-driven tests per client using `httptest.NewServer` with canned JSON captured from the real APIs, covering: success, empty result, 404, 5xx, malformed JSON, and a server that responds slower than the client timeout.
- `tcg`: leading-zero stripping, letter-prefixed numbers unchanged, API key header present only when configured.
- `pokeapi`: branching evolution chains (for example Eevee), a chain with no evolutions, first English flavor text chosen.
- `cache`: second call is served from the cache, a call after the TTL goes upstream again, errors are not stored, key normalization, and a `go test -race` run for concurrent access.

## Out of scope

- The scan handler and OpenAPI file (later build step).
- Retries, rate-limit handling, persistent or shared caches.

## Open items

- Check how the TCG API stores trainer-gallery and promo numbers (`TG05`, `SWSH123`) before fixing the no-change rule above.
- Decide whether `Species` should return partial data if the chain call fails, or fail the whole request. Default for now: fail the whole request.

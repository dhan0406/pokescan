# Upstream Clients and Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build typed Go clients for the Pokémon TCG API and PokéAPI, plus an in-memory TTL cache decorator, all tested without network access.

**Architecture:** One package per client (`tcg`, `pokeapi`) sharing sentinel errors (`apierr`) and one small JSON-GET helper (`httpjson`). The `cache` package wraps any client method with the same signature and stores successful results only. Clients take a base URL and `*http.Client` so tests point them at `httptest` servers.

**Tech Stack:** Go 1.27, standard library only (`net/http`, `encoding/json`, `net/http/httptest`, `context`, `sync`).

**Spec:** [docs/superpowers/specs/2026-11-09-upstream-clients-design.md](../specs/2026-11-09-upstream-clients-design.md)

**Deviation from spec layout:** the spec lists `apierr`, `tcg`, `pokeapi`, `cache`. This plan adds `httpjson` so the status-code, timeout and body-close handling is written once instead of twice.

## Global Constraints

- Standard library only; no new `go.mod` requirements.
- Module path `github.com/dhan0406/pokescan/backend`; code lives under `backend/`.
- Default client timeout 10s, configurable; never use `http.DefaultClient`.
- Every client method takes `context.Context` first and builds requests with it.
- Errors wrap sentinels from `apierr` with `%w`: 404 gives `ErrNotFound`, deadline or timeout gives `ErrTimeout`, everything else (5xx, bad JSON, network failure) gives `ErrUpstream`.
- Response bodies are always closed; status is checked before decoding.
- TCG API stores numbers without leading zeros: strip them from purely numeric numbers; pass letter-prefixed numbers (`SWSH123`, `TG05`) unchanged.
- Optional TCG API key goes in the `X-Api-Key` header, only when set.
- `Species` makes three upstream calls (`/pokemon-species/{id}`, `/evolution-chain/{id}`, `/pokemon/{id}`); any failure fails the whole call.
- Cache stores successes only; no `singleflight`, no negative caching, no size cap, no retries.
- Tests use `httptest` servers and canned JSON captured from the real APIs. No test touches the internet.

## Review Focus

1. **Special characters in a card name** (`Farfetch'd`, `Mr. Mime`, a name containing `"`) must reach the TCG query intact and must not break or alter the query. Pinned in Task 2.
2. **`idOrName` with uppercase or path characters** (`Pikachu`, `../type/fire`) must be lowercased and path-escaped so it cannot reach another endpoint. Pinned in Task 4.
3. **A species with no English flavor text** returns an empty `FlavorText`, not an error. Pinned in Task 4.
4. **An oversized upstream response** must not be read without limit; reads are capped at 5 MiB and exceeding it is `ErrUpstream`. Pinned in Task 1.
5. **Empty name or number passed to `FindCards`** returns an empty slice without making a request. Pinned in Task 2.

---

### Task 1: Shared errors and JSON GET helper

**Files:**
- Create: `backend/apierr/apierr.go`
- Create: `backend/httpjson/httpjson.go`
- Test: `backend/httpjson/httpjson_test.go`

**Interfaces:**
- Produces:
  - `apierr.ErrNotFound`, `apierr.ErrTimeout`, `apierr.ErrUpstream` (`error` sentinels).
  - `httpjson.Get(ctx context.Context, hc *http.Client, url string, header http.Header, out any) error`

- [ ] **Step 1: Write failing tests in `httpjson_test.go`** (all use `httptest.NewServer`). Names and assertions:
  - `TestGet_decodesJSON`: 200 with `{"name":"x"}` fills `out`, returns nil.
  - `TestGet_sendsHeaders`: server records `X-Api-Key`; `Get` with that header delivers it.
  - `TestGet_404IsNotFound`: `errors.Is(err, apierr.ErrNotFound)`.
  - `TestGet_5xxIsUpstream`: 503 gives `errors.Is(err, apierr.ErrUpstream)`.
  - `TestGet_badJSONIsUpstream`: 200 with `{` gives `ErrUpstream`.
  - `TestGet_slowServerIsTimeout`: handler sleeps 200ms, `hc.Timeout = 50ms`, gives `errors.Is(err, apierr.ErrTimeout)`.
  - `TestGet_contextDeadlineIsTimeout`: context with 50ms deadline against the same slow handler gives `ErrTimeout`.
  - `TestGet_cancelledContext`: a cancelled context gives an error where `errors.Is(err, context.Canceled)` is true.
  - `TestGet_oversizedBodyIsUpstream`: handler writes 6 MiB of valid-looking JSON string; gives `ErrUpstream`.

- [ ] **Step 2: Run** `cd backend && go test ./httpjson/...`. Expected: build failure (package missing), then FAIL after a stub is added.

- [ ] **Step 3: Implement** the sentinels in `apierr.go` (`errors.New`) and `Get` in `httpjson.go`. Approach: `http.NewRequestWithContext`, check `errors.Is(err, context.DeadlineExceeded)` or `net.Error.Timeout()` for `ErrTimeout`, `context.Canceled` returned wrapped as-is, wrap other transport errors in `ErrUpstream`, read through `io.LimitReader(body, 5<<20+1)` and fail if the limit is exceeded, then `json.Unmarshal`.

- [ ] **Step 4: Run** `go test ./httpjson/... ./apierr/... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git add backend/apierr backend/httpjson && git commit -m "Add shared API errors and JSON GET helper"`

---

### Task 2: TCG client

**Files:**
- Create: `backend/tcg/tcg.go`
- Create: `backend/tcg/testdata/cards_pikachu_25.json` (capture: `curl "https://api.pokemontcg.io/v2/cards?q=name:pikachu%20number:25&pageSize=3&select=id,name,number,set,images"`)
- Test: `backend/tcg/tcg_test.go`

**Interfaces:**
- Consumes: `httpjson.Get`, `apierr` sentinels.
- Produces:
  - `type Card struct { ID, Name, Number, SetName string; SetTotal int; ImageURL string }`
  - `func NewClient(baseURL, apiKey string, hc *http.Client) *Client`
  - `func New(apiKey string) *Client` (base URL `https://api.pokemontcg.io`, 10s timeout)
  - `func (c *Client) FindCards(ctx context.Context, name, number string) ([]Card, error)`

- [ ] **Step 1: Write failing tests** with a fake server that records `r.URL.Query().Get("q")` and the `X-Api-Key` header and serves the fixture:
  - `TestFindCards_mapsFields`: first card from the fixture has the expected `ID`, `Name`, `Number`, `SetName`, `SetTotal` (set `printedTotal`), `ImageURL` (the `large` image).
  - `TestFindCards_stripsLeadingZeros`: `("Pikachu","025")` sends `q` containing `number:25`; `("Pikachu","004")` contains `number:4`; `("Pikachu","000")` contains `number:0`.
  - `TestFindCards_keepsLetterPrefixedNumbers`: `"SWSH123"` and `"TG05"` are sent unchanged.
  - `TestFindCards_quotesName` (Review Focus 1): table of `Farfetch'd`, `Mr. Mime`, `Say "hi"` (embedded quote), each yielding a `q` whose name is wrapped in double quotes with `"` and `\` escaped (`name:"Say \"hi\""`), and no request is rejected.
  - `TestFindCards_emptyResult`: server returns `{"data":[]}`; result is an empty slice and nil error.
  - `TestFindCards_emptyInputsSkipRequest` (Review Focus 5): empty name or empty number returns an empty slice, nil error, and the fake server records zero requests.
  - `TestFindCards_apiKeyHeader`: header present only when a key is configured.
  - `TestFindCards_errorsMapped`: 404, 500 and slow server give `ErrNotFound`, `ErrUpstream`, `ErrTimeout`.

- [ ] **Step 2: Run** `go test ./tcg/...`. Expected: FAIL (undefined `Client`).

- [ ] **Step 3: Implement** `Card`, constructors and `FindCards` in `tcg.go`. Query is `name:"<escaped>" number:<normalized>` URL-encoded via `url.Values`, with `select=id,name,number,set,images` and `pageSize=10`. Number normalization: if every rune is a digit, trim leading `0`s but keep one `0` if all zeros.

- [ ] **Step 4: Run** `go test ./tcg/... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git add backend/tcg && git commit -m "Add TCG API client"`

---

### Task 3: PokéAPI client and `TypePokemon`

**Files:**
- Create: `backend/pokeapi/pokeapi.go`
- Create: `backend/pokeapi/testdata/type_electric.json` (capture from `https://pokeapi.co/api/v2/type/electric`)
- Test: `backend/pokeapi/pokeapi_test.go`

**Interfaces:**
- Consumes: `httpjson.Get`, `apierr` sentinels.
- Produces:
  - `func NewClient(baseURL string, hc *http.Client) *Client`
  - `func New() *Client` (base URL `https://pokeapi.co`, 10s timeout)
  - `func (c *Client) TypePokemon(ctx context.Context, typeName string) ([]string, error)`

- [ ] **Step 1: Write failing tests** with a fake server serving the fixture at `/api/v2/type/electric`:
  - `TestTypePokemon_returnsNames`: result contains `"pikachu"` and `"raichu"` and has the same length as the fixture's `pokemon` array.
  - `TestTypePokemon_lowercasesAndEscapes`: `"Electric"` requests `/api/v2/type/electric`; `"../pokemon/1"` does not reach `/api/v2/pokemon/1` (request path stays under `/api/v2/type/` and the call returns `ErrNotFound` or `ErrUpstream`, never the other endpoint's data).
  - `TestTypePokemon_unknownTypeIsNotFound`: 404 gives `ErrNotFound`.

- [ ] **Step 2: Run** `go test ./pokeapi/...`. Expected: FAIL.

- [ ] **Step 3: Implement** constructors and `TypePokemon`. Use `strings.ToLower` and `url.PathEscape` on the path segment.

- [ ] **Step 4: Run** `go test ./pokeapi/... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git add backend/pokeapi && git commit -m "Add PokeAPI client with TypePokemon"`

---

### Task 4: `Species` (three calls, evolution chain)

**Files:**
- Modify: `backend/pokeapi/pokeapi.go`
- Create fixtures in `backend/pokeapi/testdata/`: `species_pikachu.json`, `pokemon_pikachu.json`, `evolution_chain_10.json`, `species_eevee.json`, `evolution_chain_67.json`, `pokemon_eevee.json` (captured from `/api/v2/pokemon-species/{n}`, `/api/v2/pokemon/{n}`, `/api/v2/evolution-chain/{id}`)
- Test: `backend/pokeapi/species_test.go`

**Interfaces:**
- Consumes: `Client` from Task 3.
- Produces:
  - `type EvolutionNode struct { Name string; EvolvesTo []EvolutionNode }`
  - `type Species struct { ID int; Name string; Types []string; Stats map[string]int; FlavorText string; Evolution EvolutionNode }`
  - `func (c *Client) Species(ctx context.Context, idOrName string) (Species, error)`

- [ ] **Step 1: Write failing tests** with a fake server routing by path to the fixtures and counting requests:
  - `TestSpecies_pikachu`: `ID` 25, `Name` `pikachu`, `Types` `["electric"]`, `Stats["speed"]` 90 and the other five stats present, `Evolution` is `pichu -> pikachu -> raichu`.
  - `TestSpecies_branchingChain`: Eevee's `Evolution.EvolvesTo` has the length of Eevee's evolutions in the fixture and includes `vaporeon`.
  - `TestSpecies_noEvolutions`: a chain with an empty `evolves_to` gives a root with `EvolvesTo` empty (use a hand-written minimal fixture or a species such as `tauros`; capture it).
  - `TestSpecies_flavorTextIsFirstEnglishNormalized`: result has no `\n` or `\f`, single spaces, and equals the expected sentence from the fixture.
  - `TestSpecies_noEnglishFlavorText` (Review Focus 3): species JSON whose entries are all non-English gives `FlavorText == ""` and nil error.
  - `TestSpecies_followsChainIDNotAbsoluteURL`: the fixture's `evolution_chain.url` points at `https://pokeapi.co/...`; the client extracts the chain ID and requests the fake server's `/api/v2/evolution-chain/10`. The test fails if any request leaves the fake server.
  - `TestSpecies_usesThreeCalls`: exactly three requests are recorded.
  - `TestSpecies_anyCallFailingFailsAll`: table where each of the three routes returns 500 in turn gives `ErrUpstream`; 404 on the species route gives `ErrNotFound`.
  - `TestSpecies_lowercasesAndEscapes` (Review Focus 2): `"Pikachu"` hits `/api/v2/pokemon-species/pikachu`; `"../type/fire"` never reaches another endpoint.

- [ ] **Step 2: Run** `go test ./pokeapi/...`. Expected: FAIL (undefined `Species`).

- [ ] **Step 3: Implement** `Species`. Call species first (take the English flavor text, chain ID parsed from the last path segment of `evolution_chain.url`, and the Pokémon's own ID), then fetch the chain and the `/pokemon/{id}` data. Flatten the nested `chain` and `evolves_to` JSON recursively into `EvolutionNode`. Join fields on whitespace for the flavor text (`strings.Fields` then `strings.Join`).

- [ ] **Step 4: Run** `go test ./pokeapi/... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git add backend/pokeapi && git commit -m "Add PokeAPI Species with evolution chain"`

---

### Task 5: Generic TTL cache

**Files:**
- Create: `backend/cache/cache.go`
- Test: `backend/cache/cache_test.go`

**Interfaces:**
- Produces:
  - `type Cache[V any] struct{ ... }`
  - `func New[V any](ttl time.Duration, now func() time.Time) *Cache[V]`
  - `func (c *Cache[V]) Get(key string) (V, bool)`
  - `func (c *Cache[V]) Set(key string, v V)`

- [ ] **Step 1: Write failing tests** using a fake clock variable:
  - `TestCache_hitBeforeTTL`: `Set` then `Get` returns the value and `true`.
  - `TestCache_missAfterTTL`: advance the clock past the TTL; `Get` returns zero value and `false`.
  - `TestCache_missingKey`: `Get` of an unknown key is `false`.
  - `TestCache_expiredEntryIsRemoved`: after an expired `Get`, the internal map length is 0 (white-box check via `len(c.items)`).
  - `TestCache_overwriteResetsTTL`: `Set` again just before expiry extends the life.
  - `TestCache_concurrentAccess`: 50 goroutines call `Set` and `Get`; passes under `-race`.

- [ ] **Step 2: Run** `go test ./cache/...`. Expected: FAIL.

- [ ] **Step 3: Implement** with a `map[string]entry[V]` guarded by `sync.Mutex`; entry holds the value and expiry time.

- [ ] **Step 4: Run** `go test ./cache/... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git add backend/cache && git commit -m "Add generic TTL cache"`

---

### Task 6: Cached wrappers

**Files:**
- Create: `backend/cache/wrappers.go`
- Test: `backend/cache/wrappers_test.go`

**Interfaces:**
- Consumes: `Cache[V]` (Task 5), `tcg.Card`, `pokeapi.Species`.
- Produces:
  - `type CardFinder interface { FindCards(ctx context.Context, name, number string) ([]tcg.Card, error) }`
  - `type SpeciesGetter interface { Species(ctx context.Context, idOrName string) (pokeapi.Species, error) }`
  - `func NewCards(next CardFinder, ttl time.Duration, now func() time.Time) *CachedCards` with method `FindCards` (same signature)
  - `func NewSpecies(next SpeciesGetter, ttl time.Duration, now func() time.Time) *CachedSpecies` with method `Species` (same signature)

- [ ] **Step 1: Write failing tests** with fake `next` implementations that count calls and can return an error:
  - `TestCachedCards_secondCallServedFromCache`: two identical calls cause one `next` call.
  - `TestCachedCards_callAfterTTLGoesUpstream`: after the clock passes the TTL, `next` is called again.
  - `TestCachedCards_errorsNotStored`: `next` errors first then succeeds; the second call reaches `next` and returns the success; the first call's error is returned unchanged.
  - `TestCachedCards_emptyResultStoredAsSuccess`: an empty slice with nil error is cached (spec: empty is not an error).
  - `TestCachedCards_keyNormalization`: `("Pikachu","025")` and `("pikachu","25")` share one entry; `("Pikachu","TG05")` does not collide with `("Pikachu","5")`.
  - `TestCachedSpecies_keyNormalization`: `"Pikachu"` and `"pikachu"` share one entry.
  - `TestCachedSpecies_errorsNotStored`.
  - Compile-time assertions in the test file that `*tcg.Client` satisfies `CardFinder`, `*pokeapi.Client` satisfies `SpeciesGetter`, and the cached types satisfy the same interfaces as their clients.

- [ ] **Step 2: Run** `go test ./cache/...`. Expected: FAIL.

- [ ] **Step 3: Implement** the wrappers. Card key is lowercased name plus `|` plus the number normalized the same way as the `tcg` client (digits-only strips leading zeros). Export the TCG normalization from `tcg` as `tcg.NormalizeNumber(string) string` and use it in both places rather than copying it; add that exported function and a one-line test to Task 2's file set if it is not already there.

- [ ] **Step 4: Run** `cd backend && go vet ./... && go test ./... -race`. Expected: all packages PASS, no vet output.

- [ ] **Step 5: Commit** `git add backend && git commit -m "Add cached wrappers for TCG and PokeAPI clients"`

---

## Self-review

- **Spec coverage:** common client rules (Task 1, 2, 3), `tcg` (Task 2), `pokeapi` including three calls and flattening (Tasks 3 and 4), `cache` with injectable clock and concurrency (Tasks 5 and 6), known limits (none implemented, per spec), testing list (distributed across tasks, including branching chains and `-race`). Open item on `TG05`/`SWSH123` storage is not a task: Task 2 should note in its commit message or a code comment what the live API returned when the fixture was captured.
- **Type consistency:** `Card`, `Species`, `EvolutionNode`, `NormalizeNumber` and constructor names are the same everywhere they appear.
- **Fix applied:** `tcg.NormalizeNumber` is exported from Task 2 so Task 6 reuses it. Task 2's file list therefore includes that exported function and a `TestNormalizeNumber` table (`"025"`→`"25"`, `"000"`→`"0"`, `"SWSH123"`→`"SWSH123"`, `"TG05"`→`"TG05"`).

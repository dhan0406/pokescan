# Scan endpoint design

Date: 2026-11-09
Status: draft for review
Parent: [Pokescan design](2026-11-07-pokescan-design.md). Builds on [upstream clients](2026-11-09-upstream-clients-design.md) and the existing `ocr` package.

## Purpose

Turn a card photo into a matched card and Pokémon details over HTTP, so the whole recognition path can be exercised with `curl` before the app exists. This plan also adds the HTTP server skeleton, `GET /v1/pokemon/{idOrName}` (needed by the picker) and `GET /ping`.

## Decisions

| Area | Decision |
|---|---|
| Calling Tesseract | Shell out to the `tesseract` CLI with `os/exec`. No cgo. |
| Finding regions | Fixed percentage crops. The app crops the photo to its 2.5:3.5 frame before upload; the server cuts a name band and a number band. |
| Card to species | Use the TCG API's `nationalPokedexNumbers` field, then `Species` by number. No name parsing. |
| Uncertain results | Ask for a retake when a better photo could help. Show a picker only when several real cards match. |
| Retake limit | Enforced by the app (3 in a row), not the server. The server is stateless. |
| Confidence | Tesseract word confidence used only as a threshold. The response has `status`, not a numeric score. |

Verified against the live TCG API on 2026-11-09: `Charizard ex` has `nationalPokedexNumbers: [6]`, `Alolan Vulpix` has `[37]`, and Trainer cards such as `Professor's Research` have no such field (`supertype: "Trainer"`).

## Packages

```
backend/
  tesseract/      run the CLI, parse TSV, return text + confidence
  imageprep/      decode, crop bands, grayscale, encode PNG
  scan/           the pipeline; depends only on small interfaces
  server/         handlers, validation, error mapping, API JSON types
  cmd/pokescan/   main: wiring and env config
openapi.yaml      (repo root) contract for the endpoints below
```

Existing packages are reused: `ocr` (`ExtractName`, `ExtractNumber`), `tcg`, `pokeapi`, `cache`, `apierr`.

## `tesseract`

- `Engine{Path string}` with `Read(ctx, png []byte) (Result, error)`, where `Result{Text string; Confidence float64}`.
- Runs `tesseract stdin stdout --psm 7 tsv` (single text line) with the image on stdin, under a 5s timeout derived from `ctx`.
- Confidence is the mean of the word confidences in the TSV, ignoring rows with confidence `-1`. No words gives text `""` and confidence `0`.
- A missing binary, non-zero exit or timeout returns an error. The server maps it to `ocr_failed`, or to `upstream_timeout` for a timeout.

## `imageprep`

- `Prepare(r io.Reader) (Regions, error)`, where `Regions{Name, Number []byte}` are PNGs.
- Accepts JPEG and PNG only, detected from content, not from the file name. Anything else is an error that maps to `bad_image`.
- Bands are fractions of image height, held in one exported config struct so they can be tuned with real photos. Starting values: name band 4%–12%, number band 92%–98%, full width. The number band may later narrow to the left part of the card; tuning is an open item.
- Converts to grayscale. No resizing: the app already resizes to about 1200px on the long edge.

## `scan` pipeline

```go
type OCR interface { Read(ctx context.Context, png []byte) (tesseract.Result, error) }
type CardSearcher interface {
    FindCards(ctx context.Context, name, number string) ([]tcg.Card, error)
    Search(ctx context.Context, q tcg.Query) ([]tcg.Card, error)
}
type SpeciesGetter interface { Species(ctx context.Context, idOrName string) (pokeapi.Species, error) }

func (s *Scanner) Scan(ctx context.Context, img io.Reader) (Outcome, error)
```

Steps:

1. `imageprep.Prepare`, then OCR on the name band and the number band.
2. `ocr.ExtractName` and `ocr.ExtractNumber` on the text. A field counts as read only if extraction gives a non-empty value and that band's confidence is at least 60.
3. Decide the outcome:

| Read | Search | Outcome |
|---|---|---|
| Name and number | `FindCards(name, number)` gives 1 card | `match` |
| Name and number | gives 2 or more | `choose` with up to 5 candidates |
| Name and number | gives 0 | `retake`, reason `not_found` |
| Number (with set total) only | `Search{Number, SetTotal}` | `retake`, reason `name_unreadable`, with up to 5 candidates |
| Name only | `Search{Name}` | `retake`, reason `number_unreadable`, with up to 5 candidates |
| Neither | none | `retake`, reason `card_unreadable` |

4. For `match`, if the card's `DexNumber` is non-zero, call `Species(DexNumber)` and include it. For Trainer or Energy cards (`DexNumber == 0`), the outcome has no Pokémon.

Upstream errors from the searches or `Species` are returned as errors, not as `retake`.

## Changes to `tcg`

- `Card` gains `DexNumber int`: the first entry of `nationalPokedexNumbers`, or 0 when absent. `select` adds `nationalPokedexNumbers`.
- New `Search(ctx, q Query) ([]Card, error)` with `Query{Name, Number string; SetTotal int}`. Any combination of non-empty fields is allowed: the name is quoted and escaped as in `FindCards`, the number is validated and normalized as in `FindCards`, and `SetTotal` adds `set.printedTotal:<n>`. An all-empty query returns an empty slice without a request.
- `Search` is not cached. `FindCards` stays behind `cache.CachedCards`.

## HTTP API

All JSON. Defined in `openapi.yaml`, which is the contract the app's TypeScript types will be generated from (generation itself is out of scope).

### `POST /v1/scan`

- Multipart form, field `image`. Request body limited to 10 MB with `http.MaxBytesReader`. Overall request deadline 20s.
- Response 200 for every outcome:

```json
{
  "status": "match | choose | retake",
  "card": { "id": "", "name": "", "number": "", "setName": "", "setTotal": 0, "imageUrl": "" },
  "pokemon": { "id": 0, "name": "", "types": [], "stats": {}, "flavorText": "", "evolution": { "name": "", "evolvesTo": [] } },
  "candidates": [ { "...card fields" } ],
  "reason": "name_unreadable | number_unreadable | card_unreadable | not_found"
}
```

`card` and `pokemon` are present only for `match` (`pokemon` omitted for Trainer and Energy cards). `candidates` is present for `choose` and optionally for `retake`. `reason` is present only for `retake`. The card JSON also includes `dexNumber` so the app can request Pokémon details after a pick.

### `GET /v1/pokemon/{idOrName}`

Returns the `pokemon` object above, via the cached `Species` client. Used after the user picks a candidate.

### `GET /ping`

Returns 200 `{"status":"ok"}`.

### Errors

Body `{ "code": "...", "message": "..." }`. Messages are plain language and never contain upstream URLs or raw errors.

| Condition | Status | Code |
|---|---|---|
| Missing `image` field, or not JPEG/PNG | 400 | `bad_image` |
| Body over 10 MB | 413 | `image_too_large` |
| Unknown Pokémon (`/v1/pokemon`) | 404 | `not_found` |
| Upstream or Tesseract timeout | 504 | `upstream_timeout` |
| Other upstream failure | 502 | `upstream_error` |
| Tesseract missing or failed | 500 | `ocr_failed` |
| Wrong method | 405 | `method_not_allowed` |

## Configuration (`cmd/pokescan`)

| Env var | Default |
|---|---|
| `PORT` | `8080` |
| `CACHE_TTL` | `24h` |
| `TCG_API_KEY` | empty (optional) |
| `TESSERACT_PATH` | `tesseract` (looked up on `PATH`) |
| `SAVE_UPLOADS_DIR` | empty (off). When set, every successfully scanned photo is saved there as `<UTC time>-<status>.jpg\|png` (directory `0700`, files `0600`) so real photos can be used to tune the OCR bands and confidence threshold. Photos of cards only, but they are the user's data: point it outside the repository. A save failure is logged and never changes the response. |

Startup fails fast with a clear message if `CACHE_TTL` does not parse. A missing Tesseract binary is reported at startup as a warning, and scans then return `ocr_failed`.

## Testing

- `tesseract`: tests run against a fake `tesseract` shell script in `testdata` that prints canned TSV, checking the arguments, stdin, TSV parsing, `-1` rows, empty output, non-zero exit and timeout. One extra test runs the real binary on a generated image and is skipped when the binary is missing.
- `imageprep`: generated images check the band pixel bounds, rejection of non-image and truncated input, and that JPEG and PNG are both accepted.
- `scan`: one table test per row of the outcome table, plus the confidence threshold, Trainer card with no Pokémon, candidate cap of 5, and upstream errors propagating.
- `tcg`: `DexNumber` mapping (present and absent) and `Search` query building for each field combination.
- `server`: `httptest` tests for each status and each error code, the 10 MB limit, the JSON shape (field names exactly as above), and that error messages contain no URLs.

## Known limits

- Regional forms show base-species data (Alolan Vulpix shows Vulpix's types and stats).
- Partial-read searches are not cached.
- Fixed crops fail on crooked or badly framed photos; the retake flow is the mitigation.

## Out of scope

- `GET /v1/types` and `GET /v1/types/{type}`.
- Dockerfile and `docker compose` (next build step).
- Generating TypeScript types from `openapi.yaml` (app step).
- Server-side card detection.

## Open items

- Tune band percentages and the confidence threshold of 60 with real card photos, and add those photos as the integration test fixtures the parent spec asks for.

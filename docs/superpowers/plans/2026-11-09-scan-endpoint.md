# Scan Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve `POST /v1/scan`, `GET /v1/pokemon/{idOrName}` and `GET /ping` from a Go binary that reads card photos with the Tesseract CLI and matches them through the existing TCG and PokéAPI clients.

**Architecture:** `imageprep` crops two grayscale bands; `tesseract` runs the CLI per band; `scan` turns OCR text into an outcome (`match`/`choose`/`retake`) through small interfaces; `server` owns HTTP, validation, JSON types and error mapping; `cmd/pokescan` wires real implementations from env vars.

**Tech Stack:** Go 1.27 standard library only (`net/http` 1.22+ routing patterns, `os/exec`, `image/jpeg`, `image/png`, `log/slog`), Tesseract 5 CLI.

**Spec:** [docs/superpowers/specs/2026-11-09-scan-endpoint-design.md](../specs/2026-11-09-scan-endpoint-design.md)

## Global Constraints

- Standard library only; no new `go.mod` requirements.
- Module `github.com/dhan0406/pokescan/backend`; Go code under `backend/`, `openapi.yaml` at the repo root.
- Tesseract is invoked as `<path> stdin stdout --psm 7 tsv` with the PNG on stdin; 5s timeout per call.
- Confidence threshold 60; a field is read only if extraction is non-empty and its band's mean confidence ≥ 60.
- Candidate lists are capped at 5.
- Upload limit 10 MB (`http.MaxBytesReader`); overall scan deadline 20s; JPEG and PNG only, detected from content.
- Default bands: name 0.04–0.12, number 0.92–0.98 of image height, full width.
- Error body `{"code","message"}`; messages are plain language and never contain URLs or raw error text. Codes and statuses exactly as the spec's error table.
- Env: `PORT` (8080), `CACHE_TTL` (24h), `TCG_API_KEY` (empty), `TESSERACT_PATH` (`tesseract`).
- Every test runs without network access and without the real Tesseract binary, except one test that skips when the binary is missing.

## Review Focus

1. **Decompression bomb:** a small file declaring huge dimensions (for example 50000×50000) must be rejected as `bad_image` from its header, before full decode. Cap at 40 megapixels. Pinned in Task 3.
2. **Client disconnect or deadline during OCR:** the Tesseract subprocess must be killed when the context ends, and `Read` must return promptly. Pinned in Task 2.
3. **Promo or trainer-gallery number without a numeric total** (`SWSH039`, `TG05/TG30`) with an unreadable name must search by number with `SetTotal` 0, not fail. Pinned in Task 4.
4. **Empty upload or wrong field name** (`file` instead of `image`, zero-byte `image`) must give 400 `bad_image`, not 500. Pinned in Task 6.
5. **JSON shape stability for the generated TS types:** no `null` arrays (`types`, `candidates` when present, `evolvesTo`), and absent fields omitted rather than `null`. Pinned in Tasks 5 and 6.

---

### Task 1: `tcg` gains `DexNumber` and `Search`

**Files:**
- Modify: `backend/tcg/tcg.go`
- Modify: `backend/tcg/testdata/cards_pikachu_25.json` (add `"nationalPokedexNumbers":[25]` to the first card by hand, matching the live shape verified in the spec)
- Create: `backend/tcg/testdata/cards_trainer.json` (hand-written: one card with `"supertype":"Trainer"` and no `nationalPokedexNumbers`)
- Test: `backend/tcg/tcg_test.go`

**Interfaces:**
- Produces:
  - `Card` gains field `DexNumber int` (first `nationalPokedexNumbers` entry, else 0).
  - `type Query struct { Name, Number string; SetTotal int }`
  - `func (c *Client) Search(ctx context.Context, q Query) ([]Card, error)`

- [ ] **Step 1: Write failing tests**
  - Update `TestFindCards_mapsFields`: want includes `DexNumber: 25`.
  - `TestFindCards_trainerHasNoDexNumber`: serving `cards_trainer.json` gives `DexNumber == 0`.
  - `TestFindCards_selectIncludesDex`: request's `select` query param contains `nationalPokedexNumbers`.
  - `TestSearch_queryBuilding` table on the recorded `q`:
    - `{Name:"Pikachu"}` gives exactly `name:"Pikachu"`
    - `{Number:"025"}` gives `number:25`
    - `{Number:"025", SetTotal:165}` gives `number:25 set.printedTotal:165`
    - `{Name:"Pikachu", Number:"TG05"}` gives `name:"Pikachu" number:TG05`
    - `{Name:`Say "hi"`}` gives `name:"Say \"hi\""`
  - `TestSearch_emptyOrInvalidSkipsRequest`: `Query{}`, `{Name:"  "}`, `{Number:"025/185"}` each give an empty non-nil slice, nil error, 0 requests.
  - `TestSearch_errorsMapped`: 500 gives `apierr.ErrUpstream`.

- [ ] **Step 2: Run** `cd backend && go test ./tcg/...`. Expected: FAIL (`DexNumber`, `Search`, `Query` undefined).

- [ ] **Step 3: Implement.** Add `NationalPokedexNumbers []int` to the decode struct and `nationalPokedexNumbers` to `selectFields`. Extract query-clause building into one helper used by both `FindCards` and `Search` so trimming, quoting and number validation stay identical. An invalid non-empty number in `Search` skips the request (same as `FindCards`).

- [ ] **Step 4: Run** `go test ./tcg/... ./cache/... -race`. Expected: PASS (cache tests compare cards and must still pass).

- [ ] **Step 5: Commit** `git commit -m "Add DexNumber and Search to TCG client"`

---

### Task 2: `tesseract` package

**Files:**
- Create: `backend/tesseract/tesseract.go`
- Create: `backend/tesseract/testdata/fake-tesseract.sh` (executable)
- Test: `backend/tesseract/tesseract_test.go`

**Interfaces:**
- Consumes: `apierr.ErrTimeout`.
- Produces:
  - `type Result struct { Text string; Confidence float64 }`
  - `type Engine struct { Path string; Timeout time.Duration }` (zero `Timeout` means 5s)
  - `func (e Engine) Read(ctx context.Context, png []byte) (Result, error)`

The fake script is driven by environment variables the test sets with `t.Setenv`: it writes its arguments to `$FAKE_ARGS_OUT`, copies stdin to `$FAKE_STDIN_OUT`, sleeps `$FAKE_SLEEP` seconds if set, prints the file `$FAKE_TSV`, and exits with `$FAKE_EXIT` (default 0).

TSV format (tab-separated, header row first): `level page_num block_num par_num line_num word_num left top width height conf text`. Word rows have `level` 5.

- [ ] **Step 1: Write failing tests** (all use the fake script as `Path`):
  - `TestRead_argsAndStdin`: args are exactly `stdin stdout --psm 7 tsv`; stdin content equals the bytes passed.
  - `TestRead_parsesWordsAndConfidence`: TSV with words `Pikachu` (conf 90) and `V` (conf 70) gives `Text "Pikachu V"`, `Confidence 80`.
  - `TestRead_ignoresMinusOneRows`: non-word rows with conf `-1` are excluded from the mean.
  - `TestRead_noWords`: header-only TSV gives `Result{"", 0}` and nil error.
  - `TestRead_nonZeroExit`: `FAKE_EXIT=1` gives an error that is not `ErrTimeout`.
  - `TestRead_missingBinary`: `Path` pointing at a nonexistent file gives an error.
  - `TestRead_timeout`: `FAKE_SLEEP=2`, `Timeout 100ms` gives `errors.Is(err, apierr.ErrTimeout)` and returns in under 1s.
  - `TestRead_cancelledContextKillsProcess` (Review Focus 2): `FAKE_SLEEP=2`, context cancelled after 50ms; `Read` returns in under 1s with an error.
  - `TestRead_realBinary`: `exec.LookPath("tesseract")` or `t.Skip`; reading a 400×60 all-white PNG gives nil error and `Text ""`.

- [ ] **Step 2: Run** `go test ./tesseract/...`. Expected: FAIL.

- [ ] **Step 3: Implement** with `exec.CommandContext` on a `context.WithTimeout` child context, `cmd.Stdin = bytes.NewReader(png)`, capture stdout and stderr. A context deadline gives `fmt.Errorf("%w: ...", apierr.ErrTimeout)`. Parse with `strings.Split` on lines and tabs (12 columns, text may be empty), keeping `level == "5"` rows with non-empty text.

- [ ] **Step 4: Run** `go test ./tesseract/... -race`. Expected: PASS (`TestRead_realBinary` runs, since Tesseract is installed).

- [ ] **Step 5: Commit** `git commit -m "Add tesseract CLI wrapper"`

---

### Task 3: `imageprep` package

**Files:**
- Create: `backend/imageprep/imageprep.go`
- Test: `backend/imageprep/imageprep_test.go`

**Interfaces:**
- Produces:
  - `type Bands struct { NameTop, NameBottom, NumberTop, NumberBottom float64 }`
  - `var DefaultBands = Bands{0.04, 0.12, 0.92, 0.98}`
  - `type Regions struct { Name, Number []byte }` (PNG-encoded grayscale)
  - `var ErrBadImage = errors.New("bad image")`
  - `func Prepare(r io.Reader, b Bands) (Regions, error)`

- [ ] **Step 1: Write failing tests** with images generated in the test:
  - `TestPrepare_cropsBands`: a 500×700 image whose rows are coloured by height band. Decoded `Name` is 500×56 (rows 28–84), `Number` is 500×42 (rows 644–686); pixel samples match the source rows as gray.
  - `TestPrepare_acceptsJPEGAndPNG`.
  - `TestPrepare_outputIsGrayPNG`: decoded regions are `*image.Gray`.
  - `TestPrepare_rejectsNonImage`: `"hello"`, an empty reader, a GIF, and a truncated PNG (first 100 bytes) each give `errors.Is(err, ErrBadImage)`.
  - `TestPrepare_rejectsHugeDimensions` (Review Focus 1): a hand-built PNG header declaring 50000×50000 gives `ErrBadImage` without attempting full decode (test finishes in under 1s).
  - `TestPrepare_rejectsTinyImage`: a 10×10 image gives `ErrBadImage` (a band would be under 1px high).

- [ ] **Step 2: Run** `go test ./imageprep/...`. Expected: FAIL.

- [ ] **Step 3: Implement.** Read all input into memory (the server already caps it at 10 MB), `image.DecodeConfig` to check format (`jpeg`/`png` only, registered by blank import) and pixel count ≤ 40,000,000, then `image.Decode`, crop with `SubImage` bounds computed from the fractions, draw into `image.NewGray`, `png.Encode`. A band less than 1px high is `ErrBadImage`.

- [ ] **Step 4: Run** `go test ./imageprep/... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git commit -m "Add image band preparation"`

---

### Task 4: `scan` pipeline

**Files:**
- Create: `backend/scan/scan.go`
- Test: `backend/scan/scan_test.go`

**Interfaces:**
- Consumes: `imageprep.Prepare`, `imageprep.DefaultBands`, `imageprep.ErrBadImage`; `tesseract.Result`; `ocr.ExtractName`, `ocr.ExtractNumber`; `tcg.Card`, `tcg.Query`; `pokeapi.Species`; `apierr` sentinels.
- Produces:
  - `type OCR interface { Read(ctx context.Context, png []byte) (tesseract.Result, error) }`
  - `type CardSearcher interface { FindCards(ctx context.Context, name, number string) ([]tcg.Card, error); Search(ctx context.Context, q tcg.Query) ([]tcg.Card, error) }`
  - `type SpeciesGetter interface { Species(ctx context.Context, idOrName string) (pokeapi.Species, error) }`
  - Status constants `StatusMatch = "match"`, `StatusChoose = "choose"`, `StatusRetake = "retake"`; reason constants `ReasonNameUnreadable = "name_unreadable"`, `ReasonNumberUnreadable = "number_unreadable"`, `ReasonCardUnreadable = "card_unreadable"`, `ReasonNotFound = "not_found"`.
  - `type Outcome struct { Status string; Card *tcg.Card; Pokemon *pokeapi.Species; Candidates []tcg.Card; Reason string }`
  - `var ErrOCR = errors.New("ocr failed")`
  - `type Scanner struct { OCR OCR; Cards CardSearcher; Species SpeciesGetter; Bands imageprep.Bands; MinConfidence float64 }`
  - `func New(o OCR, c CardSearcher, s SpeciesGetter) *Scanner` (defaults: `DefaultBands`, 60)
  - `func (s *Scanner) Scan(ctx context.Context, img io.Reader) (Outcome, error)`

Test fakes: an OCR fake that returns a configured `Result` per call order (first call is the name band, second the number band) and can return an error; a card fake recording `FindCards` and `Search` arguments; a species fake recording its argument. The input image is any valid 500×700 PNG generated in the test.

- [ ] **Step 1: Write failing tests**, table-driven over the spec's outcome table:
  - one match gives `match`, `Card` set, `Pokemon` from `Species("25")` (decimal of `DexNumber`).
  - two matches give `choose` with both; seven matches give `choose` with exactly 5.
  - zero matches give `retake` / `not_found`, no candidates.
  - number `025/165` read, name unreadable: `Search{Number:"025", SetTotal:165}`, outcome `retake` / `name_unreadable` with ≤5 candidates.
  - number `SWSH039` (no total) or `TG05/TG30` read, name unreadable (Review Focus 3): `Search` with `SetTotal 0`.
  - name read, number unreadable: `Search{Name:"Pikachu"}`, `retake` / `number_unreadable`.
  - neither read gives `retake` / `card_unreadable`, no search calls.
  - confidence 59 on a band makes that field unread; 60 counts as read.
  - Trainer card (`DexNumber 0`) gives `match` with `Pokemon == nil` and no species call.
  - `FindCards` returns `apierr.ErrUpstream`: `Scan` returns an error matching it.
  - `Species` returns `apierr.ErrNotFound`: `Scan` returns an error matching `apierr.ErrUpstream` (the card exists, so a missing species is an upstream inconsistency, not "not found").
  - OCR returns a plain error: `errors.Is(err, ErrOCR)`. OCR returns an `apierr.ErrTimeout` error: it still matches `apierr.ErrTimeout`.
  - Invalid image: `errors.Is(err, imageprep.ErrBadImage)`, no OCR calls.

- [ ] **Step 2: Run** `go test ./scan/...`. Expected: FAIL.

- [ ] **Step 3: Implement** `Scan` as the spec's steps. Convert the total with `strconv.Atoi`, using 0 when it isn't numeric. Wrap OCR errors as `fmt.Errorf("%w: %w", ErrOCR, err)` so both `ErrOCR` and an inner `ErrTimeout` match.

- [ ] **Step 4: Run** `go test ./scan/... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git commit -m "Add scan pipeline"`

---

### Task 5: `server` skeleton: JSON types, errors, `/ping`, `/v1/pokemon`

**Files:**
- Create: `backend/server/server.go`, `backend/server/json.go`, `backend/server/errors.go`
- Test: `backend/server/server_test.go`

**Interfaces:**
- Consumes: `scan.Outcome`, `scan.SpeciesGetter`, `scan.ErrOCR`, `imageprep.ErrBadImage`, `apierr` sentinels.
- Produces:
  - `type Scanner interface { Scan(ctx context.Context, img io.Reader) (scan.Outcome, error) }`
  - `func New(sc Scanner, species scan.SpeciesGetter, log *slog.Logger) http.Handler`
  - JSON types with exact tags: `Card{ID "id", Name "name", Number "number", SetName "setName", SetTotal "setTotal", ImageURL "imageUrl", DexNumber "dexNumber"}`, `Pokemon{ID "id", Name "name", Types "types", Stats "stats", FlavorText "flavorText", Evolution "evolution"}`, `Evolution{Name "name", EvolvesTo "evolvesTo"}`, `ScanResponse{Status "status", Card "card,omitempty" (*Card), Pokemon "pokemon,omitempty" (*Pokemon), Candidates "candidates,omitempty", Reason "reason,omitempty"}`, `ErrorBody{Code "code", Message "message"}`.
  - `func writeError(w http.ResponseWriter, log *slog.Logger, err error)`

- [ ] **Step 1: Write failing tests** with fake scanner and species:
  - `TestPing`: `GET /ping` gives 200 and `{"status":"ok"}`.
  - `TestPokemon_ok`: `GET /v1/pokemon/pikachu` gives 200, and the body decoded into `map[string]any` has exactly the keys `id, name, types, stats, flavorText, evolution`.
  - `TestPokemon_noNullArrays` (Review Focus 5): a species with nil `Types` and a leaf evolution node gives JSON with `"types":[]` and `"evolvesTo":[]`, never `null`.
  - `TestPokemon_errorMapping` table: `ErrNotFound` gives 404 `not_found`, `ErrTimeout` gives 504 `upstream_timeout`, `ErrUpstream` gives 502 `upstream_error`, a plain error gives 500 `internal_error`.
  - `TestErrors_noURLsOrRawErrors`: for each case above, `message` contains neither `http` nor the wrapped error's text.
  - `TestMethodNotAllowed`: `POST /ping`, `GET /v1/scan`, `DELETE /v1/pokemon/x` each give 405 with JSON code `method_not_allowed`.
  - `TestResponsesAreJSON`: every response has `Content-Type: application/json`.

- [ ] **Step 2: Run** `go test ./server/...`. Expected: FAIL.

- [ ] **Step 3: Implement.** Use a `http.ServeMux` with patterns `GET /ping`, `GET /v1/pokemon/{idOrName}`, `POST /v1/scan` (stub returning 501 until Task 6), plus method-less patterns `/ping`, `/v1/scan`, `/v1/pokemon/{idOrName}` that answer 405 JSON. `writeError` checks with `errors.Is` and `errors.As` in this order: `*http.MaxBytesError` (413 `image_too_large`), `imageprep.ErrBadImage` (400 `bad_image`), `apierr.ErrNotFound` (404), `apierr.ErrTimeout` or `context.DeadlineExceeded` (504), `scan.ErrOCR` (500 `ocr_failed`), `apierr.ErrUpstream` (502), else 500 `internal_error`. Fixed plain-language messages per code; the full error is logged with `slog`.

- [ ] **Step 4: Run** `go test ./server/... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git commit -m "Add HTTP server skeleton with ping and pokemon endpoints"`

---

### Task 6: `POST /v1/scan` handler

**Files:**
- Modify: `backend/server/server.go`
- Test: `backend/server/scan_test.go`

**Interfaces:**
- Consumes: everything from Task 5.
- Produces: the real `POST /v1/scan` handler.

- [ ] **Step 1: Write failing tests** (multipart bodies built with `mime/multipart`):
  - `TestScan_match`: the fake scanner returns a match with Pokémon, giving 200, `status:"match"`, and `card` and `pokemon` present; `candidates` and `reason` keys are absent.
  - `TestScan_choose`: `candidates` has 2 entries, and `card`, `pokemon` and `reason` are absent.
  - `TestScan_retakeWithAndWithoutCandidates`: `reason` is present; `candidates` is absent when there are none.
  - `TestScan_trainerMatchOmitsPokemon`.
  - `TestScan_passesImageBytes`: the scanner receives exactly the uploaded bytes.
  - `TestScan_badRequests` (Review Focus 4): no multipart body, field named `file`, zero-byte `image`, and non-multipart JSON body each give 400 `bad_image`, with the scanner not called.
  - `TestScan_tooLarge`: an 11 MB `image` part gives 413 `image_too_large`, with the scanner not called.
  - `TestScan_deadline`: the scanner's context has a deadline at most 20s away.
  - `TestScan_scannerErrorsMapped`: `imageprep.ErrBadImage` gives 400, `scan.ErrOCR` gives 500 `ocr_failed`, and `apierr.ErrTimeout` gives 504.

- [ ] **Step 2: Run** `go test ./server/...`. Expected: FAIL (the handler is the 501 stub).

- [ ] **Step 3: Implement.** Wrap the body with `http.MaxBytesReader(w, r.Body, 10<<20)`, call `r.ParseMultipartForm(10<<20)`, and get `r.FormFile("image")`. A missing file or an empty file gives `bad_image`. Use `context.WithTimeout(r.Context(), 20*time.Second)`, call `Scan`, and map the outcome to `ScanResponse`.

- [ ] **Step 4: Run** `go test ./server/... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git commit -m "Add scan endpoint handler"`

---

### Task 7: `cmd/pokescan`, `openapi.yaml`, smoke test

**Files:**
- Create: `backend/cmd/pokescan/main.go`, `backend/cmd/pokescan/config.go`
- Test: `backend/cmd/pokescan/config_test.go`
- Create: `openapi.yaml` (repo root)

**Interfaces:**
- Consumes: all packages above; `cache.NewCards`, `cache.NewSpecies`, `tcg.New`, `pokeapi.New`.
- Produces:
  - `type Config struct { Port string; CacheTTL time.Duration; TCGAPIKey string; TesseractPath string }`
  - `func loadConfig(getenv func(string) string) (Config, error)`

- [ ] **Step 1: Write failing tests:**
  - `TestLoadConfig_defaults`: an empty env gives `8080`, `24h`, `""`, `tesseract`.
  - `TestLoadConfig_overrides`: all four values are read.
  - `TestLoadConfig_badTTL`: `CACHE_TTL=soon` gives an error naming `CACHE_TTL`.

- [ ] **Step 2: Run** `go test ./cmd/...`. Expected: FAIL.

- [ ] **Step 3: Implement** `loadConfig` and `main`. `main` builds a `cardSearcher` adapter struct (named fields `cached *cache.CachedCards` and `client *tcg.Client`, with `FindCards` delegating to `cached` and `Search` to `client`; embedding both would make `FindCards` ambiguous). It logs a warning if `exec.LookPath(TesseractPath)` fails, and serves with `http.Server{Addr: ":"+Port, ReadHeaderTimeout: 10s, WriteTimeout: 30s}`. On SIGINT or SIGTERM it calls `Shutdown` with a 10s timeout.

- [ ] **Step 4: Write `openapi.yaml`** (OpenAPI 3.1) covering the three endpoints, the `Card`, `Pokemon`, `Evolution`, `ScanResponse` and `ErrorBody` schemas with the exact field names from Task 5, the `status` and `reason` enums, and every error status from the spec. Arrays are marked `type: array` and are not nullable.

- [ ] **Step 5: Verify:**
  - `cd backend && go vet ./... && go test ./... -race`. Expected: all PASS.
  - Smoke test with the real binary and live network: `go run ./cmd/pokescan &`, then:
    - `curl -s localhost:8080/ping` gives `{"status":"ok"}`.
    - `curl -s localhost:8080/v1/pokemon/pikachu` gives JSON with `"id":25`.
    - `curl -s -F image=@<blank 500x700 PNG> localhost:8080/v1/scan` gives `{"status":"retake","reason":"card_unreadable"}`.
    - `curl -s -X POST localhost:8080/v1/scan` gives 400 `bad_image`.
  - Stop the server.

- [ ] **Step 6: Commit** `git commit -m "Add pokescan server binary and OpenAPI contract"`

---

## Self-review

- **Spec coverage:** `tesseract` (Task 2), `imageprep` (3), pipeline and outcome table (4), `tcg` changes (1), all three endpoints and the error table (5, 6), configuration and the startup warning (7), `openapi.yaml` (7), and the testing list (distributed across tasks). The spec's "fail fast on bad `CACHE_TTL`" is in Task 7. Known limits and out-of-scope items have no tasks, by design.
- **Additions beyond the spec:** two rulings. A 500 `internal_error` code for unclassified errors (the spec's table has no catch-all). A species 404 during a scan maps to `upstream_error` (the card was found, so "not found" would mislead the user).
- **Type consistency:** `tcg.Query`, `scan.Outcome`, the status and reason constants, `scan.ErrOCR`, `imageprep.ErrBadImage` and the JSON tags match across Tasks 1–7.

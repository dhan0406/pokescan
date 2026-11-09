# Pokescan design

Date: 2026-11-07
Status: draft for review

## Purpose

A portfolio and learning project. Open the app, point the iPhone camera at a physical Pokémon card, and see information about that Pokémon. Scanned cards can be saved to a local binder. Users can tap through evolutions and types to learn about other Pokémon.

Not shipping to the App Store. No paid Apple Developer account.

## Decisions

| Area | Decision |
|---|---|
| Client | Expo (React Native) with TypeScript, run in Expo Go. No Xcode. |
| Backend | Go service in Docker |
| Recognition | Server-side OCR (Tesseract) on the Go backend. The phone only captures and uploads. |
| Data sources | PokéAPI (species, stats, evolutions, types) and the Pokémon TCG API (card lookup) |
| Hosting | Runpod Serverless load balancing endpoint running the Go container. CPU Pod is the fallback. |
| Binder storage | On the phone (`expo-sqlite`). No backend database, no accounts, no sync. |
| Saving cards | Explicit "Add to binder" button in the viewer. No auto-save. |
| Visual direction | Binder grid for the collection, handheld Pokédex look for the card viewer |

Why Expo Go and server-side OCR: Expo Go cannot run custom native modules, so on-device OCR (for example ML Kit) needs an EAS dev build, which in turn needs a paid Apple Developer account. Keeping recognition on the server avoids that and keeps the Go service as the interesting engineering.

## User flow

1. The app opens directly on the Scan tab with a live camera.
2. The user lines a card up in a card-shaped frame and taps the shutter.
3. The app shows "Reading card…" while the image uploads and is processed.
4. The card viewer opens with information about the Pokémon.
5. The user can tap "Add to binder", tap an evolution or type to open that Pokémon, or go back to scan again.
6. The Binder tab shows previously saved cards. Tapping one opens the same viewer.

## Navigation

Expo Router with two tabs:

- Scan (initial route)
- Binder

The card viewer is a stack screen pushed over the tabs. Back returns to the camera, ready to scan again.

## Screens

### Scan

- Full-screen camera preview (`expo-camera`) with a 2.5:3.5 card frame overlay and one shutter button.
- On capture, resize to about 1200px on the long edge and compress before upload.
- States:
  - Camera permission denied: plain explanation and a button that opens Settings.
  - Uploading: "Reading card…" with the preview frozen.
  - Low-confidence match: "Which card is this?" picker with the top candidates.
  - Could not read the card, or no connection: say what happened and offer a retry. No raw errors.

### Card viewer (handheld Pokédex look)

- Shows name, Pokédex number, types, stats, flavor text, evolution chain, and the scanned card image.
- Tapping an evolution or type opens that Pokémon in the same viewer. This is the way users learn about other Pokémon. There is no standalone browse screen in v1.
- "Add to binder" button. If the card is already saved, it reads "In binder". The binder is keyed by TCG card ID, so a card cannot be added twice.
- Monospace is used for headings and stat readouts only. Flavor text and evolution info use a normal readable font.
- The LCD area takes a faint tint from the Pokémon's primary type.

### Binder

- Pocket-grid layout of saved cards.
- Stored locally with the full scan response and card image URL so cards open instantly and work offline.
- Empty slots are optional.
- Empty state invites the user to scan a first card.

## Visual system

Both skins share one set of design tokens (colors, spacing, type scale) so the app does not feel like two apps. Use NativeWind with a small tokens file, and Reanimated for the scan and reveal animations (both work in Expo Go). Spend the visual boldness on the viewer and keep the rest quiet.

Copy follows plain-language rules: sentence case, verb-first buttons, errors say what happened and what to do.

## API

Defined in `openapi.yaml` at the repo root, which is the single source of truth. TypeScript types for the mobile app are generated from it.

| Endpoint | Purpose |
|---|---|
| `POST /v1/scan` | Multipart image in. Returns the matched card, Pokémon details, a confidence score, and a short list of alternative candidates. |
| `GET /v1/pokemon/{idOrName}` | Species details and evolution chain |
| `GET /v1/types` | List of types |
| `GET /v1/types/{type}` | Pokémon of a type |
| `GET /ping` | Health check for the Runpod load balancing endpoint |

Errors use a structured body: `{ "code": "...", "message": "..." }`. The app maps each code to a plain-language state. Every upstream call has a timeout.

## Scan pipeline

1. Normalize and resize the image.
2. Run Tesseract on two regions: the name at the top and the collector number at the bottom (for example `004/165`).
3. Query the Pokémon TCG API using name plus number.
4. If OCR confidence is low or the match is ambiguous, return the top candidates and let the app show the picker instead of guessing.
5. Enrich the match with PokéAPI species data.

An in-memory cache with a TTL sits in front of PokéAPI and TCG API calls, since that data rarely changes and both APIs are rate limited.

Future upgrade, out of scope for v1: a vision model on a Runpod GPU behind the same `POST /v1/scan` contract.

## Repo layout

```
pokescan/
  mobile/          Expo + TypeScript (expo-router)
  backend/         Go service + Dockerfile
  openapi.yaml     API contract
  docker-compose.yml
  docs/superpowers/specs/
```

## Backend details

- Standard library `net/http`, no framework.
- Multi-stage Dockerfile with Tesseract installed.
- Configuration through environment variables: port, cache TTL, optional TCG API key for higher rate limits.

## Testing

Go (tested most heavily):

- Table-driven tests for extracting the name and collector number from messy OCR text.
- Tests for the TCG and PokéAPI clients against `httptest` fakes.
- Handler tests for `/v1/scan`.
- One integration test using a few sample card photos as fixtures.

Mobile:

- Jest tests for binder storage and the API client.
- No end-to-end UI tests in v1.

## Deployment

- Local: `docker compose up`. The phone reaches the API through the laptop's LAN IP via `EXPO_PUBLIC_API_URL`.
- Production: build the image, push it to a registry (GHCR or Docker Hub), and point a Runpod Serverless load balancing endpoint at it.
- Runpod load balancing endpoints route requests straight to the worker's HTTP server and support custom paths and multipart uploads. Confirm the port and health-check contract (the docs reference a `/ping` route) against current Runpod docs when building.
- Trade-offs accepted: cold starts after idle time, and requests are dropped when workers are overloaded. Keep one worker warm for demos if needed.
- Fallback: a CPU Pod running the same image, which has no cold start but is billed while running.
- CI for build and push is a later nice-to-have.

## Out of scope for v1

- Accounts or cloud sync
- App Store release
- On-device OCR
- Vision-model recognition
- Standalone browse-by-type screen
- CI pipeline

## Open items

- Confirm the Runpod load balancing port and health-check behavior with a Go container.
- Decide how much OCR accuracy is good enough on real card photos, and whether the candidate picker needs tuning.

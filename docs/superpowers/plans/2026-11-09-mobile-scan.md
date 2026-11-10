# Mobile App: Scaffold and Scan Screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An Expo app that runs in Expo Go, reaches the Go backend over the LAN, and lets you photograph a card, crop it to the frame, upload it to `POST /v1/scan`, and see the raw response.

**Architecture:** Expo Router with two tabs (Scan, Binder placeholder). Pure TypeScript modules hold everything testable: a typed API client (types generated from `openapi.yaml`), crop math, and a scan state reducer. The Scan screen is thin glue between `expo-camera`, `expo-image-manipulator` and those modules.

**Tech Stack:** Expo (SDK matching the Expo Go app on your phone), React Native, TypeScript (strict), Expo Router, `expo-camera`, `expo-image-manipulator`, `openapi-typescript`, Jest via `jest-expo`.

**Spec:** [docs/superpowers/specs/2026-11-07-pokescan-design.md](../specs/2026-11-07-pokescan-design.md) (Navigation, Scan screen, Visual system, API) and [openapi.yaml](../../../openapi.yaml). No separate design spec: the owner chose to go straight to a plan. Steps 1 and 2 of the app build order only; the card viewer and binder are later plans.

## Global Constraints

- Everything under `mobile/`. Run in Expo Go: no custom native modules, no EAS, no Xcode.
- TypeScript `strict: true`. No `any` except the one React Native `FormData` file-part cast in the API client.
- Add Expo-managed packages with `npx expo install <pkg>` so versions match the SDK.
- API base URL comes from `EXPO_PUBLIC_API_URL` (no trailing slash handling needed: strip it). `mobile/.env` is git-ignored; `mobile/.env.example` is committed.
- API types are generated from `../openapi.yaml` into `src/api/schema.d.ts` (committed) by `npm run gen:api`. Never hand-edit it.
- Capture is cropped to the on-screen card frame (2.5:3.5, portrait), resized to **1200 px on the long edge**, JPEG quality 0.8, then uploaded.
- Copy follows the spec: sentence case, verb-first buttons, errors say what happened and what to do, no raw errors shown to the user.
- NativeWind and Reanimated are **not** installed in this plan (the spec lists them for the viewer; they are added when first needed). Styles use `StyleSheet` and a small tokens file.
- Jest tests run without a device or network. Steps marked **Manual** run on the phone and are part of the task's done-criteria.

## Review Focus

1. **Preview/photo aspect mismatch:** the phone's photo (for example 3024×4032) and the on-screen preview (for example 390×844) have different aspect ratios, so the frame drawn on screen maps to a *different* region of the photo. The crop must match what the frame shows, stay within the photo bounds, and use integer pixels. Pinned in Task 4, plus a visual check in Task 5.
2. **Double-tap on the shutter, or leaving the screen mid-upload:** must not start two scans or update state after unmount. Pinned in Task 5.
3. **Server unreachable, timeout, or a non-JSON reply (for example an HTML error page from a proxy):** the user sees a plain-language state, never a raw exception or `undefined`. Pinned in Task 2.
4. **Camera permission denied or permanently blocked:** show an explanation and a button that opens Settings, not a blank camera. Pinned in Task 5.
5. **Missing `EXPO_PUBLIC_API_URL`:** fail with a clear message at startup, not requests to `undefined/ping`. Pinned in Task 3.

---

### Task 1: Scaffold the Expo app

**Files:**
- Create: `mobile/` (via `create-expo-app`), `mobile/.env.example`
- Modify: `mobile/app/` (reduce to two tabs), `mobile/tsconfig.json`, `mobile/package.json`
- Create: `mobile/src/theme/tokens.ts`
- Test: `mobile/src/theme/tokens.test.ts`

**Interfaces:**
- Produces: `export const colors`, `export const spacing`, `export const type` in `src/theme/tokens.ts`; routes `app/(tabs)/index.tsx` (Scan) and `app/(tabs)/binder.tsx` (Binder); npm scripts `test`, `typecheck`, `gen:api` (added in Task 2).

- [ ] **Step 1: Create the app.** From the repo root: `npx create-expo-app@latest mobile` (default template, which includes Expo Router and TypeScript), then `cd mobile && npm run reset-project` if the template offers it, moving the example code to nothing (delete `app-example/`). Confirm `npx expo --version` and the SDK number in `package.json`.

- [ ] **Step 2: Write the failing test** `src/theme/tokens.test.ts` asserting: `colors` has `background`, `surface`, `text`, `textMuted`, `accent`, `danger` (non-empty strings); `spacing` has `xs, sm, md, lg, xl` as increasing numbers; `type` has `body`, `heading`, `mono` entries each with a numeric `fontSize`.

- [ ] **Step 3: Install and configure Jest.** `npx expo install jest-expo jest @types/jest --dev`, add `"test": "jest"`, `"typecheck": "tsc --noEmit"`, a `jest` block with `"preset": "jest-expo"`. Run `npm test`. Expected: FAIL (module `./tokens` missing).

- [ ] **Step 4: Implement `src/theme/tokens.ts`** exporting `colors`, `spacing`, `type` as above (quiet palette; `mono` uses `Menlo`/`monospace` per spec "monospace for headings and stat readouts only").

- [ ] **Step 5: Reduce the router to two tabs.** `app/(tabs)/_layout.tsx` with tabs `index` titled "Scan" and `binder` titled "Binder"; `app/(tabs)/binder.tsx` renders the text "Your binder will appear here." Delete unused template screens, components, hooks and assets that nothing imports.

- [ ] **Step 6: Verify.** `npm test` PASS, `npm run typecheck` clean. **Manual:** `npx expo start`, scan the QR code with the phone's camera, confirm Expo Go opens the app on the Scan tab with two tabs. Record the Expo Go SDK compatibility result; if Expo Go on the phone rejects the SDK, stop and report.

- [ ] **Step 7: Commit** `mobile/` (check `git status` shows no `node_modules`, `.expo`, or `ios`/`android` folders): `git commit -m "Scaffold Expo app with Scan and Binder tabs"`

---

### Task 2: Generated API types and typed client

**Files:**
- Modify: `mobile/package.json` (script `gen:api`, devDependency `openapi-typescript`)
- Create: `mobile/src/api/schema.d.ts` (generated), `mobile/src/api/client.ts`
- Test: `mobile/src/api/client.test.ts`

**Interfaces:**
- Consumes: `../openapi.yaml`.
- Produces:
  - `type ScanResponse`, `type Pokemon`, `type Card` (aliases of `components['schemas'][...]`)
  - `type ApiErrorCode = 'bad_image' | 'image_too_large' | 'not_found' | 'method_not_allowed' | 'ocr_failed' | 'internal_error' | 'upstream_error' | 'upstream_timeout' | 'network_error' | 'timeout' | 'bad_response'`
  - `class ApiError extends Error { code: ApiErrorCode; status?: number }`
  - `function createApiClient(opts: { baseUrl: string; fetch?: typeof fetch; scanTimeoutMs?: number }): { ping(): Promise<void>; scan(photoUri: string, signal?: AbortSignal): Promise<ScanResponse>; getPokemon(idOrName: string): Promise<Pokemon> }`

- [ ] **Step 1: Generate types.** `npm i -D openapi-typescript`, script `"gen:api": "openapi-typescript ../openapi.yaml -o src/api/schema.d.ts"`, run it, confirm `ScanResponse` appears in the output.

- [ ] **Step 2: Write failing tests** with a fake `fetch` that records its arguments:
  - `ping resolves on 200 {"status":"ok"}`; requests `<baseUrl>/ping`; a base URL with a trailing slash does not produce `//ping`.
  - `scan sends multipart`: body is a `FormData` whose `image` part is the file object `{uri, name: 'card.jpg', type: 'image/jpeg'}`; the request has **no** `Content-Type` header (fetch must set the multipart boundary); method POST to `/v1/scan`.
  - `scan returns the parsed ScanResponse` for a `match` body, and for `choose` and `retake` bodies.
  - `scan maps server errors`: a 400 body `{"code":"bad_image","message":"..."}` rejects with `ApiError` where `code === 'bad_image'`, `status === 400`, and the message equals the server's message.
  - `non-JSON error bodies` (Review Focus 3): a 502 with an HTML body, and a 200 with HTML, each reject with `ApiError` code `bad_response`, and the HTML never appears in `error.message`.
  - `JSON without a status field` on a 200 scan response rejects with `bad_response`.
  - `network failure`: fetch rejecting with `TypeError('Network request failed')` gives code `network_error`.
  - `timeout`: a fetch that never resolves and `scanTimeoutMs: 50` rejects with code `timeout` within 500 ms (use the abort signal passed to fetch).
  - `caller abort`: aborting the `signal` passed to `scan` rejects with an `ApiError` of code `network_error` and message "Cancelled.", and never hangs.
  - `getPokemon` requests `/v1/pokemon/<encodeURIComponent(idOrName)>` and returns the parsed body; a 404 gives `ApiError` code `not_found`.

- [ ] **Step 3: Run** `npm test -- client`. Expected: FAIL.

- [ ] **Step 4: Implement `client.ts`.** One private `request` helper: builds the URL, applies a timeout through `AbortController` (default 25 s for scan, 10 s otherwise), parses JSON inside try/catch, converts every failure into `ApiError` with a fixed plain-language message per code (server `message` is used only when the body is valid `ErrorBody` JSON). The React Native file-part object is appended with a single documented `as unknown as Blob` cast.

- [ ] **Step 5: Run** `npm test && npm run typecheck`. Expected: PASS.

- [ ] **Step 6: Commit** `git commit -m "Add generated API types and typed client"`

---

### Task 3: Config and server status

**Files:**
- Create: `mobile/src/config.ts`, `mobile/src/api/index.ts`, `mobile/src/components/ServerStatus.tsx`, `mobile/.env.example`
- Modify: `mobile/app/(tabs)/index.tsx`, `mobile/.gitignore` (ensure `.env` is ignored)
- Test: `mobile/src/config.test.ts`

**Interfaces:**
- Consumes: `createApiClient` (Task 2).
- Produces:
  - `function readApiUrl(env: Record<string, string | undefined>): string` (throws `Error` with a message naming `EXPO_PUBLIC_API_URL` and an example value when missing, empty, or not starting with `http://` or `https://`; strips trailing slashes)
  - `export const api` from `src/api/index.ts`: the shared client built from `readApiUrl(process.env)`
  - `<ServerStatus />`: shows "Server: checking…", "Server: connected", or "Server: can't connect. Check that the backend is running and the phone is on the same Wi-Fi."

- [ ] **Step 1: Write failing tests** for `readApiUrl` (Review Focus 5): returns `http://192.168.1.5:8080` unchanged; strips `/` and `//` suffix; throws for `undefined`, `""`, `"192.168.1.5:8080"` (no scheme), and each error message contains `EXPO_PUBLIC_API_URL`.

- [ ] **Step 2: Run** `npm test -- config`. Expected: FAIL.

- [ ] **Step 3: Implement** `readApiUrl`, `api/index.ts`, and `ServerStatus` (calls `api.ping()` on mount and when the user taps it to retry; ignores results after unmount). Show `<ServerStatus />` at the top of the Scan screen. If `readApiUrl` throws, the Scan screen renders its message in a plain error view instead of crashing.

- [ ] **Step 4: Create** `.env.example` containing `EXPO_PUBLIC_API_URL=http://192.168.0.10:8080` with a comment on finding the laptop's LAN IP (`ipconfig getifaddr en0`). Create your local `.env` with the real LAN IP.

- [ ] **Step 5: Run** `npm test && npm run typecheck`. Expected: PASS.

- [ ] **Step 6: Manual.** In one terminal `cd backend && go run ./cmd/pokescan`; in another `cd mobile && npx expo start`. Allow macOS to accept incoming connections if prompted. Open the app: the banner should read "Server: connected". If it fails with a cleartext/network error, try `curl http://<LAN-IP>:8080/ping` from the laptop first, then fall back to a tunnel (`cloudflared tunnel --url http://localhost:8080` or ngrok) and use its `https` URL in `.env`. Record which route worked.

- [ ] **Step 7: Commit** `git commit -m "Add API config and server status banner"`

---

### Task 4: Card crop math and upload preparation

**Files:**
- Create: `mobile/src/capture/crop.ts`, `mobile/src/capture/prepare.ts`
- Test: `mobile/src/capture/crop.test.ts`

**Interfaces:**
- Produces:
  - `type Size = { width: number; height: number }`, `type Rect = { originX: number; originY: number; width: number; height: number }`
  - `const CARD_ASPECT = 2.5 / 3.5`, `const UPLOAD_LONG_EDGE = 1200`
  - `function frameRect(view: Size, widthFraction?: number): Rect` (default 0.8; a portrait card frame centered in the view, in view points)
  - `function cropRectForPhoto(photo: Size, view: Size, frame: Rect): Rect` (maps the frame to photo pixels assuming the preview scales the photo to *cover* the view and centers it; result is integers, clamped inside the photo)
  - `function uploadSize(crop: Rect): Size` (scales so the long edge is `UPLOAD_LONG_EDGE`, never upscales)
  - `async function prepareUpload(photoUri: string, photo: Size, view: Size): Promise<{ uri: string; width: number; height: number }>` (thin wrapper over `expo-image-manipulator`: crop, resize, JPEG 0.8)

- [ ] **Step 1: Write failing tests** for `crop.ts` (Review Focus 1):
  - `frameRect`: for a 390×844 view, width is 312, height is `312 / CARD_ASPECT`, and the rect is centered (equal margins within 1 pt).
  - `cropRectForPhoto` with a photo of the same aspect as the view (780×1688 for 390×844): scale is 2, so the crop is exactly the frame rect doubled.
  - 3024×4032 photo in a 390×844 view (preview is cropped left/right): the crop's aspect ratio is within 0.5% of `CARD_ASPECT`; it is horizontally centered on the photo within 1 px; all values are integers; it lies fully inside `[0,photo]`.
  - A photo *wider* relative to the view than cover can show (for example 4032×3024 given portrait view dimensions) and a photo taller than the view both stay in bounds and keep the aspect.
  - A frame touching the view edge (`widthFraction: 1`) clamps to the photo instead of overshooting.
  - `uploadSize`: crop 1491×2087 gives long edge 1200 and preserves aspect within 1 px; a 600×840 crop stays 600×840.

- [ ] **Step 2: Run** `npm test -- crop`. Expected: FAIL.

- [ ] **Step 3: Implement** the pure functions in `crop.ts`. Cover mapping: `scale = max(view.width / photo.width, view.height / photo.height)`; the displayed photo is `photo * scale`, offset by `((displayed - view) / 2)` on each axis; photo coordinate = `(viewCoordinate + offset) / scale`. Round origin down and size to the nearest integer, then clamp. Implement `prepare.ts` with `ImageManipulator.manipulateAsync(uri, [{crop}, {resize}], {compress: 0.8, format: JPEG})`.

- [ ] **Step 4: Run** `npm test && npm run typecheck`. Expected: PASS.

- [ ] **Step 5: Commit** `git commit -m "Add card crop math and upload preparation"`

---

### Task 5: Scan screen

**Files:**
- Create: `mobile/src/scan/reducer.ts`, `mobile/src/scan/useScan.ts`, `mobile/src/components/CardFrame.tsx`, `mobile/src/components/PermissionPrompt.tsx`
- Modify: `mobile/app/(tabs)/index.tsx`
- Test: `mobile/src/scan/reducer.test.ts`

**Interfaces:**
- Consumes: `api`, `ScanResponse`, `ApiError` (Tasks 2-3); `frameRect`, `prepareUpload` (Task 4).
- Produces:
  - `type ScanState = { phase: 'idle' } | { phase: 'working' } | { phase: 'done'; response: ScanResponse; thumbnailUri: string } | { phase: 'failed'; error: ApiError }`
  - `type ScanAction = { type: 'start' } | { type: 'succeeded'; response: ScanResponse; thumbnailUri: string } | { type: 'failed'; error: ApiError } | { type: 'reset' }`
  - `function scanReducer(state: ScanState, action: ScanAction): ScanState`
  - `function useScan(): { state: ScanState; capture(camera: CameraView, view: Size): Promise<void>; reset(): void }`

- [ ] **Step 1: Write failing tests** for `scanReducer` (Review Focus 2):
  - `idle + start → working`; `working + succeeded → done` carrying response and thumbnail; `working + failed → failed`; `done|failed + reset → idle`.
  - `working + start → working` (a second tap is ignored: same state object returned).
  - `idle + succeeded` and `idle + failed` are ignored (a late result after reset does not appear).
  - `done + start` is ignored until `reset`.

- [ ] **Step 2: Run** `npm test -- reducer`. Expected: FAIL.

- [ ] **Step 3: Implement** `reducer.ts`, then `useScan.ts`: `capture` returns immediately if the state is not `idle`; takes the picture with `camera.takePictureAsync({ skipProcessing: false })`; calls `prepareUpload` with the returned width and height; uploads with `api.scan`; dispatches `succeeded` or `failed`; ignores results if the screen unmounted (an `isMounted` ref) and aborts the request via `AbortController` on unmount.

- [ ] **Step 4: Implement the screen.** `app/(tabs)/index.tsx`: `useCameraPermissions()`; while loading show nothing; if denied show `<PermissionPrompt />` ("Pokescan needs your camera to read cards." + button "Allow camera" that requests permission, or "Open Settings" via `Linking.openSettings()` when `canAskAgain` is false) (Review Focus 4). Otherwise a full-screen `CameraView` (back camera) with `<CardFrame />` (a dimmed overlay with a transparent card-shaped cut-out sized from `frameRect(view)`) and a shutter button, disabled while `working`. While `working`, show "Reading card…". On `done`, show the cropped thumbnail and the response JSON (`JSON.stringify(response, null, 2)`) in a scroll view with a "Scan another card" button. On `failed`, show `error.message` and a "Try again" button. Keep `<ServerStatus />` at the top.

- [ ] **Step 5: Run** `npm test && npm run typecheck`. Expected: PASS.

- [ ] **Step 6: Manual (done-criteria for the whole plan).** Backend running, phone on the same Wi-Fi:
  1. Camera permission prompt appears; "Don't allow" shows the permission screen and "Open Settings" works.
  2. Point at a real card, line it up in the frame, tap the shutter: "Reading card…" appears once even if you tap repeatedly.
  3. The returned **thumbnail matches the framed area** (card fills it, name at the top and number at the bottom both visible). If it does not match, fix the crop math before continuing (Review Focus 1).
  4. A readable card gives `status: "match"` or `"choose"`; an unreadable one gives `"retake"` with a reason. Note which, for tuning.
  5. Stop the backend and scan again: a plain-language failure message appears, not a raw error.

- [ ] **Step 7: Commit** `git commit -m "Add scan screen with camera, crop and upload"`

---

### Task 6: Save uploads on the server for OCR tuning (addition, optional)

Not part of app build-order steps 1-2. It exists so the photos you take in Task 5 can be used to tune the band positions and confidence threshold, which the backend spec lists as an open item. Drop it if you would rather save photos another way.

**Files:**
- Modify: `backend/cmd/pokescan/config.go`, `backend/cmd/pokescan/main.go`, `backend/server/server.go`
- Test: `backend/cmd/pokescan/config_test.go`, `backend/server/scan_test.go`

**Interfaces:**
- Consumes: existing `Config`, `server.New`.
- Produces: env var `SAVE_UPLOADS_DIR` (default empty = off); `Config.SaveUploadsDir string`; `server.New` gains an option or field to write each accepted upload to `<dir>/<UTC timestamp>-<status>.jpg|png`.

- [ ] **Step 1: Write failing tests.** Config: `SAVE_UPLOADS_DIR` default empty and override read. Server: with a directory set, a successful scan writes exactly one file whose bytes equal the upload and whose name contains the outcome status (`match`, `choose`, `retake`); with no directory, nothing is written; an unwritable directory does not fail the request (the scan still returns 200 and the failure is logged); file names cannot contain path separators.

- [ ] **Step 2: Run** `cd backend && go test ./cmd/... ./server/...`. Expected: FAIL.

- [ ] **Step 3: Implement.** Keep `server.New`'s signature compatible (for example a variadic `Option` with `WithSaveUploads(dir string)`). Write after the scan completes, so the file name carries the status; never let a save error change the response.

- [ ] **Step 4: Run** `go vet ./... && go test ./... -race`. Expected: PASS.

- [ ] **Step 5: Commit** `git commit -m "Add optional upload saving for OCR tuning"`

---

## Self-review

- **Spec coverage:** Navigation (two tabs, Scan first): Task 1. Scan screen camera, 2.5:3.5 frame, shutter, crop, 1200 px resize, "Reading card…", permission-denied state with Settings button, plain-language errors: Tasks 4-5. TypeScript types generated from `openapi.yaml`: Task 2. `EXPO_PUBLIC_API_URL`: Task 3. Not covered by design: the retake tips, the picker and the 3-retake rule, the viewer, the binder, NativeWind and Reanimated (all later plans).
- **Type consistency:** `ScanResponse`, `ApiError`, `Size`, `Rect`, `frameRect`, `prepareUpload`, `scanReducer` names and signatures match across Tasks 2-5.
- **Placeholder scan:** no open decisions left; every step names its behavior and assertions.
- **Risks flagged, not solved:** Expo Go SDK compatibility with the phone (Task 1 step 6), cleartext HTTP to the LAN (Task 3 step 6), and whether the camera preview really uses cover scaling (Task 5 step 6.3).

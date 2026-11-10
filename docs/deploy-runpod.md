# Deploy to Runpod (Serverless, Load Balancer endpoint)

Docs checked 2026-11-10: https://docs.runpod.io/serverless/load-balancing/overview

## 1. Build and push an x86 image
Runpod runs linux/amd64; a Mac builds arm64 by default.

```
docker login
docker buildx build --platform linux/amd64 -t <dockerhub-user>/pokescan-api:latest --push backend
```

## 2. Create the endpoint (console)
Serverless → New Endpoint → Import from Docker Registry → `<dockerhub-user>/pokescan-api:latest`.
- **Endpoint Type: Load Balancer.**
- Environment variables: `PORT=8080`, `PORT_HEALTH=8081` (the image defaults to these).
- Expose HTTP ports: `8080` (and `8081`).
- Workers: min 0 saves money but cold starts take a while; use min 1 for a demo.
- The guide only mentions GPU types. The server is CPU-only and needs none; if a CPU
  option exists in the console, use it.

## 3. Point the app at it
`mobile/.env`:
```
EXPO_PUBLIC_API_URL=https://ENDPOINT_ID.api.runpod.ai
EXPO_PUBLIC_API_KEY=<Runpod API key>
```
Requests send `Authorization: Bearer <key>`. Anything `EXPO_PUBLIC_*` can be read by anyone
with the app, so use a **restricted** key limited to this endpoint and rotate it if shared.
Restart Expo after editing `.env`.

## Behavior to know
- `/ping`: 200 = healthy. The server answers it on both `PORT` and `PORT_HEALTH`.
- Cold start: requests can wait up to 2 min for a worker, or return `{"error":"no workers
  available"}`. The app waits up to 120 s for a scan and retries that error 3 times, 5 s apart.
- Limits: 30 MB payload, 5.5 min per request.
- Quick check: `curl -H "Authorization: Bearer $KEY" https://ENDPOINT_ID.api.runpod.ai/ping`

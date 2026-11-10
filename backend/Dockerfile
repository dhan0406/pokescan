# Build a static Go binary, then copy it into a small image with Tesseract.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pokescan ./cmd/pokescan

FROM alpine:3.21
# tesseract does the OCR; ca-certificates lets the server reach PokéAPI over HTTPS.
RUN apk add --no-cache tesseract-ocr tesseract-ocr-data-eng ca-certificates \
 && adduser -D -u 10001 pokescan
COPY --from=build /out/pokescan /usr/local/bin/pokescan
USER pokescan
# Runpod load balancing endpoints poll PORT_HEALTH for GET /ping.
ENV PORT=8080 PORT_HEALTH=8081
EXPOSE 8080 8081
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- "http://127.0.0.1:${PORT}/ping" >/dev/null || exit 1
ENTRYPOINT ["pokescan"]

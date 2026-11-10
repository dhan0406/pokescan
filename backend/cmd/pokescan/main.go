// Command pokescan serves the Pokescan API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/dhan0406/pokescan/backend/cache"
	"github.com/dhan0406/pokescan/backend/pokeapi"
	"github.com/dhan0406/pokescan/backend/scan"
	"github.com/dhan0406/pokescan/backend/server"
	"github.com/dhan0406/pokescan/backend/tcg"
	"github.com/dhan0406/pokescan/backend/tesseract"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(cfg.TesseractPath); err != nil {
		log.Warn("tesseract not found; scans will fail with ocr_failed", "path", cfg.TesseractPath)
	}

	tcgClient := tcg.New(cfg.TCGAPIKey)
	cards := cardSearcher{cached: cache.NewCards(tcgClient, cfg.CacheTTL, time.Now), client: tcgClient}
	species := cache.NewSpecies(pokeapi.New(), cfg.CacheTTL, time.Now)
	scanner := scan.New(tesseract.Engine{Path: cfg.TesseractPath}, cards, species)

	srv := newHTTPServer(cfg.Port, server.New(scanner, species, log))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// cardSearcher serves FindCards from the cache and Search (partial reads,
// not cached) from the client. Embedding both would make FindCards ambiguous.
type cardSearcher struct {
	cached *cache.CachedCards
	client *tcg.Client
}

func (c cardSearcher) FindCards(ctx context.Context, name, number string) ([]tcg.Card, error) {
	return c.cached.FindCards(ctx, name, number)
}

func (c cardSearcher) Search(ctx context.Context, q tcg.Query) ([]tcg.Card, error) {
	return c.client.Search(ctx, q)
}

// newHTTPServer sets timeouts that fit the request budget. WriteTimeout runs
// from the end of the request headers, so it must cover reading the upload
// (ReadTimeout), the handler's 20s deadline and writing the reply; otherwise a
// slow upload followed by a slow scan loses its response entirely.
func newHTTPServer(port string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

package main

import (
	"fmt"
	"time"
)

// Config is the server configuration, read from environment variables.
type Config struct {
	Port          string
	CacheTTL      time.Duration
	TCGAPIKey     string
	TesseractPath string
	// SaveUploadsDir, when set, makes the server save every scanned photo there for OCR tuning.
	SaveUploadsDir string
}

// loadConfig reads PORT, CACHE_TTL, TCG_API_KEY, TESSERACT_PATH and SAVE_UPLOADS_DIR, applying
// defaults for any that are unset.
func loadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:           withDefault(getenv("PORT"), "8080"),
		CacheTTL:       24 * time.Hour,
		TCGAPIKey:      getenv("TCG_API_KEY"),
		TesseractPath:  withDefault(getenv("TESSERACT_PATH"), "tesseract"),
		SaveUploadsDir: getenv("SAVE_UPLOADS_DIR"),
	}
	if v := getenv("CACHE_TTL"); v != "" {
		ttl, err := time.ParseDuration(v)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("CACHE_TTL=%q must be a positive duration such as 24h or 90m", v)
		}
		cfg.CacheTTL = ttl
	}
	return cfg, nil
}

func withDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

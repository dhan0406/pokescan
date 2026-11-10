package main

import (
	"fmt"
	"time"
)

// Config is the server configuration, read from environment variables.
type Config struct {
	Port          string
	CacheTTL      time.Duration
	TesseractPath string
	// SaveUploadsDir, when set, makes the server save every scanned photo there for OCR tuning.
	// HealthPort, when set and different from Port, serves GET /ping alone on
	// that port (Runpod load balancing endpoints poll PORT_HEALTH).
	HealthPort     string
	SaveUploadsDir string
}

// loadConfig reads PORT, CACHE_TTL, TESSERACT_PATH, PORT_HEALTH and SAVE_UPLOADS_DIR, applying
// defaults for any that are unset.
func loadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:           withDefault(getenv("PORT"), "8080"),
		CacheTTL:       24 * time.Hour,
		TesseractPath:  withDefault(getenv("TESSERACT_PATH"), "tesseract"),
		HealthPort:     getenv("PORT_HEALTH"),
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

package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfig_defaults(t *testing.T) {
	got, err := loadConfig(env(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	want := Config{Port: "8080", CacheTTL: 24 * time.Hour, TesseractPath: "tesseract", SaveUploadsDir: ""}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoadConfig_overrides(t *testing.T) {
	got, err := loadConfig(env(map[string]string{
		"PORT": "9000", "CACHE_TTL": "90m", "TESSERACT_PATH": "/opt/tesseract", "SAVE_UPLOADS_DIR": "/tmp/cards",
	}))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	want := Config{Port: "9000", CacheTTL: 90 * time.Minute, TesseractPath: "/opt/tesseract", SaveUploadsDir: "/tmp/cards"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoadConfig_badTTL(t *testing.T) {
	for _, v := range []string{"soon", "-1h", "0s"} {
		_, err := loadConfig(env(map[string]string{"CACHE_TTL": v}))
		if err == nil || !strings.Contains(err.Error(), "CACHE_TTL") {
			t.Errorf("CACHE_TTL=%q: err = %v, want an error naming CACHE_TTL", v, err)
		}
	}
}

func TestNewHTTPServer_timeoutsFitTheRequestBudget(t *testing.T) {
	srv := newHTTPServer("8080", http.NotFoundHandler())
	if srv.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", srv.Addr)
	}
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Errorf("ReadHeader %v, Read %v, Idle %v: all must be set", srv.ReadHeaderTimeout, srv.ReadTimeout, srv.IdleTimeout)
	}
	// WriteTimeout runs from the end of the headers, so it must cover the
	// whole upload plus the 20s handler deadline plus time to write the reply.
	if min := srv.ReadTimeout + 20*time.Second + 5*time.Second; srv.WriteTimeout < min {
		t.Errorf("WriteTimeout = %v, want at least %v", srv.WriteTimeout, min)
	}
}

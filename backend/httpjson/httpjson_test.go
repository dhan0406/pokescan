package httpjson

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dhan0406/pokescan/backend/apierr"
)

func TestGet_decodesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name":"x"}`))
	}))
	defer srv.Close()

	var out struct{ Name string }
	if err := Get(context.Background(), srv.Client(), srv.URL, nil, &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.Name != "x" {
		t.Errorf("Name = %q, want x", out.Name)
	}
}

func TestGet_sendsHeaders(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Api-Key")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	h := http.Header{"X-Api-Key": {"secret"}}
	var out struct{}
	if err := Get(context.Background(), srv.Client(), srv.URL, h, &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "secret" {
		t.Errorf("X-Api-Key = %q, want secret", got)
	}
}

func TestGet_statusMapping(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"404 is not found", 404, apierr.ErrNotFound},
		{"500 is upstream", 500, apierr.ErrUpstream},
		{"503 is upstream", 503, apierr.ErrUpstream},
		{"429 is upstream", 429, apierr.ErrUpstream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()
			var out struct{}
			err := Get(context.Background(), srv.Client(), srv.URL, nil, &out)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want errors.Is %v", err, tt.want)
			}
		})
	}
}

func TestGet_badJSONIsUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{`))
	}))
	defer srv.Close()
	var out struct{}
	err := Get(context.Background(), srv.Client(), srv.URL, nil, &out)
	if !errors.Is(err, apierr.ErrUpstream) {
		t.Errorf("err = %v, want ErrUpstream", err)
	}
}

func slowServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-r.Context().Done():
		}
		w.Write([]byte(`{}`))
	}))
}

func TestGet_slowServerIsTimeout(t *testing.T) {
	srv := slowServer()
	defer srv.Close()
	hc := &http.Client{Timeout: 50 * time.Millisecond}
	var out struct{}
	err := Get(context.Background(), hc, srv.URL, nil, &out)
	if !errors.Is(err, apierr.ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
}

func TestGet_contextDeadlineIsTimeout(t *testing.T) {
	srv := slowServer()
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var out struct{}
	err := Get(ctx, srv.Client(), srv.URL, nil, &out)
	if !errors.Is(err, apierr.ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
}

func TestGet_cancelledContext(t *testing.T) {
	srv := slowServer()
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out struct{}
	err := Get(ctx, srv.Client(), srv.URL, nil, &out)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want errors.Is context.Canceled", err)
	}
}

func TestGet_oversizedBodyIsUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name":"` + strings.Repeat("a", 6<<20) + `"}`))
	}))
	defer srv.Close()
	var out struct{ Name string }
	err := Get(context.Background(), srv.Client(), srv.URL, nil, &out)
	if !errors.Is(err, apierr.ErrUpstream) {
		t.Errorf("err = %v, want ErrUpstream", err)
	}
}

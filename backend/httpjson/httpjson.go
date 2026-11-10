// Package httpjson makes JSON GET requests with shared error handling.
package httpjson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/dhan0406/pokescan/backend/apierr"
)

// maxBody caps how much of a response we will read.
const maxBody = 5 << 20 // 5 MiB

// Get performs a GET request and decodes the JSON response body into out.
//
// Errors wrap the sentinels in apierr: 404 is ErrNotFound, a deadline or
// timeout is ErrTimeout, and any other failure is ErrUpstream. A cancelled
// context is returned as an error that matches context.Canceled.
func Get(ctx context.Context, hc *http.Client, url string, header http.Header, out any) error {
	// The card services fail intermittently with 5xx; these are quick failures,
	// so retry a couple of times before giving up.
	for attempt := 1; ; attempt++ {
		err := get(ctx, hc, url, header, out)
		if !errors.Is(err, errRetryable) || attempt == maxAttempts {
			return err
		}
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return classify(ctx.Err())
		}
	}
}

const (
	maxAttempts = 3
	retryDelay  = 400 * time.Millisecond
)

var errRetryable = errors.New("retryable")

func get(ctx context.Context, hc *http.Client, url string, header http.Header, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %v", apierr.ErrUpstream, err)
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := hc.Do(req)
	if err != nil {
		return classify(err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: GET %s", apierr.ErrNotFound, url)
	case resp.StatusCode >= 500 && resp.StatusCode <= 504:
		return fmt.Errorf("%w: GET %s: status %d: %w", apierr.ErrUpstream, url, resp.StatusCode, errRetryable)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("%w: GET %s: status %d", apierr.ErrUpstream, url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return classify(err)
	}
	if len(body) > maxBody {
		return fmt.Errorf("%w: GET %s: response larger than %d bytes", apierr.ErrUpstream, url, maxBody)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: decode %s: %v", apierr.ErrUpstream, url, err)
	}
	return nil
}

// classify maps a transport error onto the apierr sentinels.
func classify(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return fmt.Errorf("%w: %v", apierr.ErrTimeout, err)
	}
	return fmt.Errorf("%w: %v", apierr.ErrUpstream, err)
}

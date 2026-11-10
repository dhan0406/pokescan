// Package apierr holds the sentinel errors shared by the upstream API clients.
package apierr

import "errors"

var (
	// ErrNotFound means the upstream has no such resource (HTTP 404).
	ErrNotFound = errors.New("not found")
	// ErrTimeout means the upstream did not answer in time.
	ErrTimeout = errors.New("upstream timeout")
	// ErrUpstream means the upstream failed or returned something unusable.
	ErrUpstream = errors.New("upstream error")
)

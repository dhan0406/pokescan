package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/dhan0406/pokescan/backend/apierr"
	"github.com/dhan0406/pokescan/backend/imageprep"
	"github.com/dhan0406/pokescan/backend/scan"
)

// apiError is a response status, a stable code the app maps to a screen
// state, and a plain-language message. Messages never include the underlying
// error, which may contain upstream URLs.
type apiError struct {
	status  int
	code    string
	message string
}

var (
	errBadImage         = apiError{http.StatusBadRequest, "bad_image", "That photo couldn't be read. Send a JPEG or PNG photo of the card."}
	errImageTooLarge    = apiError{http.StatusRequestEntityTooLarge, "image_too_large", "That photo is too large. Send one under 10 MB."}
	errNotFound         = apiError{http.StatusNotFound, "not_found", "We couldn't find that."}
	errUpstreamTimeout  = apiError{http.StatusGatewayTimeout, "upstream_timeout", "The card service took too long to answer. Try again."}
	errUpstream         = apiError{http.StatusBadGateway, "upstream_error", "The card service isn't responding right now. Try again in a moment."}
	errOCR              = apiError{http.StatusInternalServerError, "ocr_failed", "We couldn't read the photo right now. Try again in a moment."}
	errMethodNotAllowed = apiError{http.StatusMethodNotAllowed, "method_not_allowed", "That request method isn't supported here."}
	errInternal         = apiError{http.StatusInternalServerError, "internal_error", "Something went wrong on our side. Try again."}
)

// classify maps an error from the pipeline or the clients to an apiError.
// Order matters: an OCR timeout matches both scan.ErrOCR and ErrTimeout and
// should report as a timeout.
func classify(err error) apiError {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		return errImageTooLarge
	case errors.Is(err, imageprep.ErrBadImage):
		return errBadImage
	case errors.Is(err, apierr.ErrNotFound):
		return errNotFound
	case errors.Is(err, apierr.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return errUpstreamTimeout
	case errors.Is(err, scan.ErrOCR):
		return errOCR
	case errors.Is(err, apierr.ErrUpstream):
		return errUpstream
	default:
		return errInternal
	}
}

// writeError logs err and writes the matching error response.
func writeError(w http.ResponseWriter, log *slog.Logger, err error) {
	e := classify(err)
	log.Error("request failed", "code", e.code, "err", err)
	writeAPIError(w, e)
}

func writeAPIError(w http.ResponseWriter, e apiError) {
	writeJSON(w, e.status, ErrorBody{Code: e.code, Message: e.message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

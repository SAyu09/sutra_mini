package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"sutra-mini/internal/apperr"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	// Limit request body to 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: invalid JSON payload: %v", apperr.ErrInvalidInput, err)
	}

	// Check for trailing garbage
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: request body must contain only a single JSON object", apperr.ErrInvalidInput)
	}

	return nil
}

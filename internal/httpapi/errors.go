package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"sutra-mini/internal/apperr"
)

type ErrorPayload struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Outstanding *int   `json:"outstanding,omitempty"`
}

type ErrorResponse struct {
	Error ErrorPayload `json:"error"`
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}

	var overReceiptErr apperr.ErrOverReceipt
	if errors.As(err, &overReceiptErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error: ErrorPayload{
				Code:        "OVER_RECEIPT",
				Message:     overReceiptErr.Error(),
				Outstanding: &overReceiptErr.Outstanding,
			},
		})
		return
	}

	if errors.Is(err, apperr.ErrInvalidInput) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error: ErrorPayload{
				Code:    "INVALID_INPUT",
				Message: err.Error(),
			},
		})
		return
	}

	if errors.Is(err, apperr.ErrNotFound) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error: ErrorPayload{
				Code:    "NOT_FOUND",
				Message: err.Error(),
			},
		})
		return
	}

	if errors.Is(err, apperr.ErrInvalidState) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error: ErrorPayload{
				Code:    "INVALID_STATE",
				Message: err.Error(),
			},
		})
		return
	}

	if errors.Is(err, apperr.ErrKeyReused) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error: ErrorPayload{
				Code:    "IDEMPOTENCY_KEY_REUSED",
				Message: err.Error(),
			},
		})
		return
	}

	// Internal error
	slog.Error("internal server error",
		"method", r.Method,
		"path", r.URL.Path,
		"error", err,
	)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorPayload{
			Code:    "INTERNAL",
			Message: "internal server error",
		},
	})
}

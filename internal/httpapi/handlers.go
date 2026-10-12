package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sutra-mini/internal/apperr"
	"sutra-mini/internal/issues"
	"sutra-mini/internal/receipts"
	"sutra-mini/internal/vouchers"
)

type Handlers struct {
	pool            *pgxpool.Pool
	receiptsService *receipts.Service
	issuesService   *issues.Service
	vouchersService *vouchers.Service
}

func NewHandlers(
	pool *pgxpool.Pool,
	receiptsService *receipts.Service,
	issuesService *issues.Service,
	vouchersService *vouchers.Service,
) *Handlers {
	return &Handlers{
		pool:            pool,
		receiptsService: receiptsService,
		issuesService:   issuesService,
		vouchersService: vouchersService,
	}
}

// POST /jobwork/receipts
func (h *Handlers) handleCreateReceipt(w http.ResponseWriter, r *http.Request) {
	var keyPtr *string
	if rawKey := r.Header.Get("Idempotency-Key"); rawKey != "" {
		trimmed := strings.TrimSpace(rawKey)
		if len(trimmed) > 255 {
			writeError(w, r, fmt.Errorf("%w: Idempotency-Key header exceeds 255 characters", apperr.ErrInvalidInput))
			return
		}
		if trimmed != "" {
			keyPtr = &trimmed
		}
	}

	var in receipts.Input
	if err := readJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}

	res, replayed, err := h.receiptsService.CreateReceipt(r.Context(), in, keyPtr)
	if err != nil {
		writeError(w, r, err)
		return
	}

	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}

	writeJSON(w, http.StatusCreated, res)
}

// GET /issues/{id}
func (h *Handlers) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, fmt.Errorf("%w: invalid issue id %q", apperr.ErrInvalidInput, idStr))
		return
	}

	issue, err := h.issuesService.GetIssue(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

// GET /vouchers?status=
func (h *Handlers) handleListVouchers(w http.ResponseWriter, r *http.Request) {
	var statusPtr *string
	if rawStatus := r.URL.Query().Get("status"); rawStatus != "" {
		trimmed := strings.TrimSpace(rawStatus)
		statusPtr = &trimmed
	}

	list, err := h.vouchersService.List(r.Context(), statusPtr)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"vouchers": list})
}

// POST /vouchers/{id}/retry
func (h *Handlers) handleRetryVoucher(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, fmt.Errorf("%w: invalid voucher id %q", apperr.ErrInvalidInput, idStr))
		return
	}

	updated, err := h.vouchersService.Retry(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// GET /healthz
func (h *Handlers) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

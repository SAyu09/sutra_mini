package httpapi

import (
	"net/http"
)

// NewRouter registers all API routes and middleware using Go standard library routing.
func NewRouter(h *Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /jobwork/receipts", h.handleCreateReceipt)
	mux.HandleFunc("GET /issues/{id}", h.handleGetIssue)
	mux.HandleFunc("GET /vouchers", h.handleListVouchers)
	mux.HandleFunc("POST /vouchers/{id}/retry", h.handleRetryVoucher)
	mux.HandleFunc("GET /healthz", h.handleHealthz)

	var handler http.Handler = mux
	handler = RequestLoggingMiddleware(handler)
	handler = RecoverMiddleware(handler)

	return handler
}

package httpapi

import (
	"net/http"
)

// NewRouter registers all API routes and middleware using Go standard library routing.
func NewRouter(h *Handlers) http.Handler {
	mux := http.NewServeMux()

	// /v1 routes
	mux.HandleFunc("POST /v1/jobwork/receipts", h.handleCreateReceipt)
	mux.HandleFunc("GET /v1/issues", h.handleListIssues)
	mux.HandleFunc("GET /v1/issues/{id}", h.handleGetIssue)
	mux.HandleFunc("GET /v1/vouchers", h.handleListVouchers)
	mux.HandleFunc("POST /v1/vouchers/{id}/retry", h.handleRetryVoucher)
	mux.HandleFunc("GET /v1/healthz", h.handleHealthz)

	// Unprefixed routes
	mux.HandleFunc("POST /jobwork/receipts", h.handleCreateReceipt)
	mux.HandleFunc("GET /issues", h.handleListIssues)
	mux.HandleFunc("GET /issues/{id}", h.handleGetIssue)
	mux.HandleFunc("GET /vouchers", h.handleListVouchers)
	mux.HandleFunc("POST /vouchers/{id}/retry", h.handleRetryVoucher)
	mux.HandleFunc("GET /healthz", h.handleHealthz)

	var handler http.Handler = mux
	handler = RequestLoggingMiddleware(handler)
	handler = RecoverMiddleware(handler)

	return handler
}

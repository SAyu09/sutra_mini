package mock

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RandomSource abstracts random number generation for deterministic testability.
type RandomSource interface {
	Float64() float64
}

type threadSafeRand struct {
	mu sync.Mutex
	r  *rand.Rand
}

func newThreadSafeRand() *threadSafeRand {
	return &threadSafeRand{
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (t *threadSafeRand) Float64() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.r.Float64()
}

// Server encapsulates the chaos accounting mock server.
// Limitation (D7): Registry is stored in-memory; server restart resets state.
type Server struct {
	mu           sync.Mutex
	registry     map[string]string // client_ref -> external_ref
	errorRate    float64
	hangRate     float64
	hangDuration time.Duration
	rand         RandomSource
}

// Options configure the mock server behaviour.
type Options struct {
	ErrorRate    float64
	HangRate     float64
	HangDuration time.Duration
	Rand         RandomSource
}

func NewServer(opts Options) *Server {
	rnd := opts.Rand
	if rnd == nil {
		rnd = newThreadSafeRand()
	}

	return &Server{
		registry:     make(map[string]string),
		errorRate:    opts.ErrorRate,
		hangRate:     opts.HangRate,
		hangDuration: opts.HangDuration,
		rand:         rnd,
	}
}

type VoucherPushRequest struct {
	ClientRef   string `json:"client_ref"`
	Type        string `json:"type,omitempty"`
	SourceID    int64  `json:"source_id,omitempty"`
	AmountPaise int64  `json:"amount_paise,omitempty"`
}

// Handler returns the HTTP handler for the mock server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/accounting/vouchers", s.handlePostVoucher)
	mux.HandleFunc("POST /api/vouchers", s.handlePostVoucher)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handlePostVoucher implements Section 10 decision logic in exact order:
// 1. Validate JSON; require non-empty client_ref (else 400).
// 2. If client_ref is in registry: 409 with {"external_ref":"<stored>"}.
// 3. Else roll the dice: with probability ERROR_RATE return 500 and record NOTHING.
// 4. Else RECORD client_ref -> new external_ref in registry BEFORE any sleep.
// 5. With probability HANG_RATE: sleep HANG_DURATION, honouring context, then succeed.
// 6. Success: 200 {"external_ref":"EXT-<uuid>"}.
//
// Invariants enforced:
// - Mutex is NEVER held during sleep.
// - client_ref is recorded BEFORE any hang sleep.
func (s *Server) handlePostVoucher(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 1. Validate JSON
	var req VoucherPushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON body"})
		return
	}

	clientRef := strings.TrimSpace(req.ClientRef)
	if clientRef == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "client_ref is required"})
		return
	}

	// 2. Check registry
	s.mu.Lock()
	if existingExtRef, found := s.registry[clientRef]; found {
		s.mu.Unlock()
		slog.Info("mock decision: 409 conflict (reconciliation)", "client_ref", clientRef, "external_ref", existingExtRef)
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"external_ref": existingExtRef})
		return
	}

	// 3. Roll dice for simulated 500 error
	if s.rand.Float64() < s.errorRate {
		s.mu.Unlock() // recorded nothing
		slog.Warn("mock decision: 500 simulated failure", "client_ref", clientRef)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "simulated failure"})
		return
	}

	// 4. Record client_ref -> new external_ref BEFORE any sleep
	extRef := fmt.Sprintf("EXT-%s", uuid.NewString())
	s.registry[clientRef] = extRef
	s.mu.Unlock() // Mutex is released here! NEVER hold while sleeping.

	slog.Info("mock recorded external_ref", "client_ref", clientRef, "external_ref", extRef)

	// 5. Roll dice for hang
	if s.rand.Float64() < s.hangRate {
		slog.Info("mock decision: simulated hang", "client_ref", clientRef, "duration", s.hangDuration)
		select {
		case <-time.After(s.hangDuration):
		case <-r.Context().Done():
			slog.Warn("mock request context canceled during hang", "client_ref", clientRef)
			return
		}
	}

	// 6. Success
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"external_ref": extRef})
}

// RegistryCount returns current number of registered client_refs (test helper).
func (s *Server) RegistryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.registry)
}

// GetExternalRef returns the stored external_ref for a client_ref (test helper).
func (s *Server) GetExternalRef(clientRef string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	val, ok := s.registry[clientRef]
	return val, ok
}

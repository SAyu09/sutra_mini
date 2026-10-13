package mock_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sutra-mini/internal/mock"
)

// sequenceRand returns numbers sequentially from a slice.
type sequenceRand struct {
	mu     sync.Mutex
	values []float64
	idx    int
}

func (s *sequenceRand) Float64() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.values) == 0 {
		return 0.5
	}
	val := s.values[s.idx%len(s.values)]
	s.idx++
	return val
}

// TestM1 forces each mock branch:
// 1. 500 simulated failure (recording nothing);
// 2. Success recording client_ref -> EXT-<uuid>;
// 3. 409 Conflict returning original external_ref on duplicate client_ref;
// 4. Hang branch (verifying client_ref recorded BEFORE sleep);
// 5. Hanging request does not block concurrent requests.
func TestM1(t *testing.T) {
	t.Run("Branch1_500SimulatedFailureRecordsNothing", func(t *testing.T) {
		// Rand returns 0.05 < 0.20 (triggers error branch)
		rnd := &sequenceRand{values: []float64{0.05}}
		server := mock.NewServer(mock.Options{
			ErrorRate:    0.20,
			HangRate:     0.10,
			HangDuration: 10 * time.Millisecond,
			Rand:         rnd,
		})

		body := `{"client_ref":"ref-err-1","type":"jobwork_receipt","amount_paise":1000}`
		req := httptest.NewRequest("POST", "/api/vouchers", strings.NewReader(body))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
		if server.RegistryCount() != 0 {
			t.Fatalf("expected registry to record nothing on 500, got %d entries", server.RegistryCount())
		}
	})

	t.Run("Branch2_SuccessRecordsExternalRef", func(t *testing.T) {
		// Rand returns 0.50 (no error, no hang)
		rnd := &sequenceRand{values: []float64{0.50, 0.50}}
		server := mock.NewServer(mock.Options{
			ErrorRate:    0.20,
			HangRate:     0.10,
			HangDuration: 10 * time.Millisecond,
			Rand:         rnd,
		})

		body := `{"client_ref":"ref-success-1","type":"jobwork_receipt","amount_paise":2000}`
		req := httptest.NewRequest("POST", "/api/vouchers", strings.NewReader(body))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		extRef, ok := resp["external_ref"]
		if !ok || !strings.HasPrefix(extRef, "EXT-") {
			t.Fatalf("expected EXT- external_ref, got %v", extRef)
		}

		if server.RegistryCount() != 1 {
			t.Fatalf("expected 1 registry entry, got %d", server.RegistryCount())
		}
		stored, found := server.GetExternalRef("ref-success-1")
		if !found || stored != extRef {
			t.Fatalf("expected stored external_ref %s, got %s", extRef, stored)
		}
	})

	t.Run("Branch3_DuplicateReturns409Reconciliation", func(t *testing.T) {
		rnd := &sequenceRand{values: []float64{0.50, 0.50}}
		server := mock.NewServer(mock.Options{
			ErrorRate:    0.20,
			HangRate:     0.10,
			HangDuration: 10 * time.Millisecond,
			Rand:         rnd,
		})

		body := `{"client_ref":"ref-dup-1"}`

		// Call 1: Success (200)
		req1 := httptest.NewRequest("POST", "/api/vouchers", strings.NewReader(body))
		rec1 := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusOK {
			t.Fatalf("call 1 failed with %d: %s", rec1.Code, rec1.Body.String())
		}
		var resp1 map[string]string
		_ = json.Unmarshal(rec1.Body.Bytes(), &resp1)
		origExtRef := resp1["external_ref"]

		// Call 2: Duplicate client_ref (must return 409 with original external_ref)
		req2 := httptest.NewRequest("POST", "/api/vouchers", strings.NewReader(body))
		rec2 := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec2, req2)

		if rec2.Code != http.StatusConflict {
			t.Fatalf("expected 409 for duplicate client_ref, got %d: %s", rec2.Code, rec2.Body.String())
		}
		var resp2 map[string]string
		_ = json.Unmarshal(rec2.Body.Bytes(), &resp2)
		if resp2["external_ref"] != origExtRef {
			t.Fatalf("expected original external_ref %s on 409, got %s", origExtRef, resp2["external_ref"])
		}
	})

	t.Run("Branch4_HangRecordsBeforeSleep", func(t *testing.T) {
		// Rand returns: 0.5 (no 500 error), 0.05 (triggers hang)
		rnd := &sequenceRand{values: []float64{0.50, 0.05}}
		server := mock.NewServer(mock.Options{
			ErrorRate:    0.20,
			HangRate:     0.10,
			HangDuration: 80 * time.Millisecond,
			Rand:         rnd,
		})

		clientRef := "ref-hang-check"
		body := `{"client_ref":"` + clientRef + `"}`
		req := httptest.NewRequest("POST", "/api/vouchers", strings.NewReader(body))
		rec := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			server.Handler().ServeHTTP(rec, req)
			close(done)
		}()

		// Wait 20ms into the 80ms sleep: check that client_ref is ALREADY recorded in registry
		time.Sleep(20 * time.Millisecond)
		extRef, recorded := server.GetExternalRef(clientRef)
		if !recorded || extRef == "" {
			t.Fatalf("CRITICAL: client_ref must be recorded in registry BEFORE sleep!")
		}

		// Wait for sleep to finish and verify 200 OK
		<-done
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 after hang, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Branch5_HangingCallDoesNotBlockConcurrentCall", func(t *testing.T) {
		// First call hangs for 300ms. Second call is immediate.
		rnd := &sequenceRand{values: []float64{
			0.50, 0.05, // Call 1: no error, hang
			0.50, 0.90, // Call 2: no error, no hang
		}}

		server := mock.NewServer(mock.Options{
			ErrorRate:    0.20,
			HangRate:     0.10,
			HangDuration: 300 * time.Millisecond,
			Rand:         rnd,
		})

		hangDone := make(chan struct{})
		go func() {
			body1 := `{"client_ref":"call-1-hangs"}`
			req1 := httptest.NewRequest("POST", "/api/vouchers", strings.NewReader(body1))
			rec1 := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec1, req1)
			close(hangDone)
		}()

		// Give call 1 20ms to enter the sleep
		time.Sleep(20 * time.Millisecond)

		// Fire call 2 while call 1 is still sleeping
		call2Start := time.Now()
		body2 := `{"client_ref":"call-2-fast"}`
		req2 := httptest.NewRequest("POST", "/api/vouchers", strings.NewReader(body2))
		rec2 := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec2, req2)
		call2Duration := time.Since(call2Start)

		if rec2.Code != http.StatusOK {
			t.Fatalf("call 2 failed with %d: %s", rec2.Code, rec2.Body.String())
		}

		// Call 2 must finish almost instantly (< 50ms), proving the mutex was not held during sleep
		if call2Duration > 100*time.Millisecond {
			t.Fatalf("call 2 was blocked by call 1 sleep! Took %v", call2Duration)
		}

		<-hangDone
	})

	t.Run("ContextCancellationDuringHang", func(t *testing.T) {
		rnd := &sequenceRand{values: []float64{0.50, 0.05}}
		server := mock.NewServer(mock.Options{
			ErrorRate:    0.20,
			HangRate:     0.10,
			HangDuration: 500 * time.Millisecond,
			Rand:         rnd,
		})

		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("POST", "/api/vouchers", bytes.NewBufferString(`{"client_ref":"cancel-me"}`)).WithContext(ctx)
		rec := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			server.Handler().ServeHTTP(rec, req)
			close(done)
		}()

		// Cancel after 20ms
		time.Sleep(20 * time.Millisecond)
		cancel()

		select {
		case <-done:
			// Canceled cleanly
		case <-time.After(200 * time.Millisecond):
			t.Fatal("handler did not return promptly upon context cancellation")
		}
	})
}

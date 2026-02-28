package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestID_Generated(t *testing.T) {
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rr, req)

	id := rr.Header().Get("X-Request-ID")
	assert.NotEmpty(t, id)
	assert.Len(t, id, 16) // 8 bytes → 16 hex chars
}

func TestRequestID_Passthrough(t *testing.T) {
	const existingID = "my-custom-id-1234"
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existingID)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, existingID, rr.Header().Get("X-Request-ID"))
}

func TestRecoverer_CatchesPanic(t *testing.T) {
	panicHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := recovererMiddleware(logger)(panicHandler)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Should not panic at the test level.
	assert.NotPanics(t, func() {
		handler.ServeHTTP(rr, req)
	})
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestAuthStub_SetsAnonymous(t *testing.T) {
	var capturedUser string
	handler := authStubMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, "anonymous", capturedUser)
}

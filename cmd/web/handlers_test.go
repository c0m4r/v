package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleShutdown(t *testing.T) {
	called := make(chan struct{})
	handler := handleShutdown(func() { close(called) })
	req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status: got %d, want %d", res.Code, http.StatusAccepted)
	}
	if got, want := res.Body.String(), "{\"status\":\"shutting_down\"}\n"; got != want {
		t.Errorf("response: got %q, want %q", got, want)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown was not called")
	}
}

package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostman/internal/collection"
)

func TestDoFollowsRedirectAndReturnsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("X-Test", "value")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("details"))
	}))
	defer server.Close()

	resp, err := New().Do(context.Background(), &collection.Request{Method: http.MethodGet, URL: server.URL + "/start"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.URL != server.URL+"/final" || resp.StatusCode != http.StatusBadRequest || resp.Status != "400 Bad Request" {
		t.Errorf("response = %+v", resp)
	}
	if resp.Proto != "HTTP/1.1" || resp.Headers.Get("X-Test") != "value" || string(resp.Body) != "details" {
		t.Errorf("response metadata = %+v", resp)
	}
	if resp.Duration <= 0 {
		t.Errorf("duration = %s", resp.Duration)
	}
}

func TestDoTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	_, err := NewWithTimeout(50*time.Millisecond).Do(context.Background(), &collection.Request{Method: http.MethodGet, URL: server.URL})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if !strings.Contains(err.Error(), "do request") {
		t.Errorf("missing request context in error: %v", err)
	}
}

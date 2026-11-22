package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSendChecksParameterizedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")
		if id == "2" {
			w.WriteHeader(http.StatusNotFound)
		}
		fmt.Fprintf(w, `{"id":%s}`, id)
	}))
	defer server.Close()

	out, err := executeSend(nil,
		"send", server.URL+"/{value}",
		"--match", "value=(1, 2)",
		"--expect", "status=(200, 404)",
		"--expect", "header:Content-Type=application/json",
		"--expect", "json:/id={value}",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "PASS status:") != 2 || !strings.Contains(out, "Checks: 6 passed, 0 failed") {
		t.Errorf("unexpected check output:\n%s", out)
	}
}

func TestSendChecksWithSeparateFlagsAndExpectationOnlyMatch(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 123, "name": body.Name})
	}))
	defer server.Close()

	out, err := executeSend(nil,
		"send", server.URL, "-X", "POST", "-H", "Content-Type: application/json", "-d", `{"name":"{text}"}`,
		"--match", "{text}=(dfdf, other)", "--match", "{code}=(201)",
		"--expect-status", "{code}", "--expect-json", "name={text}", "--expect-json", "id=123",
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || !strings.Contains(out, "Case 2/2: text=other, code=201") || !strings.Contains(out, "Checks: 6 passed, 0 failed") {
		t.Errorf("calls = %d, output:\n%s", calls.Load(), out)
	}
}

func TestSendFailsOnExpectationAndValidatesBeforeRequests(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	out, err := executeSend(nil, "send", server.URL, "--expect", "status=200")
	if !errors.Is(err, ErrRunFailed) || !strings.Contains(out, "FAIL status: expected 200, got 201") || !strings.Contains(out, "Checks: 0 passed, 1 failed") {
		t.Errorf("expectation failure = %v, output:\n%s", err, out)
	}
	if calls.Load() != 1 {
		t.Errorf("requests after failed check = %d", calls.Load())
	}
	if _, err := executeSend(nil, "send", server.URL, "--expect", "status=invalid"); err == nil {
		t.Fatal("expected invalid status error")
	}
	if calls.Load() != 1 {
		t.Errorf("invalid expectation sent a request: %d", calls.Load())
	}
	if _, err := executeSend(nil, "send", server.URL, "--expect-status", "invalid"); err == nil {
		t.Fatal("expected invalid --expect-status error")
	}
	if _, err := executeSend(nil, "send", server.URL, "--expect-json", "invalid"); err == nil {
		t.Fatal("expected invalid --expect-json error")
	}
	if calls.Load() != 1 {
		t.Errorf("invalid separate flags sent a request: %d", calls.Load())
	}
}

package cli

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadUsesSharedRateAndClientLimit(t *testing.T) {
	var active, peak, calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		current := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if current <= old || peak.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(60 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":123}`)
	}))
	defer server.Close()

	out, err := executeSend(nil, NormalizeArgs([]string{
		"send", server.URL, "-rps", "40", "-c", "2", "--duration", "250ms",
		"--expect-status", "201", "--expect-json", "id=123",
	})...)
	if err != nil {
		t.Fatal(err)
	}
	got := calls.Load()
	if got < 4 || got > 11 {
		t.Errorf("request count = %d, want between 4 and 11", got)
	}
	if peak.Load() != 2 {
		t.Errorf("peak concurrent requests = %d, want 2", peak.Load())
	}
	if !strings.Contains(out, "Load: target=40 rps, clients=2") || !strings.Contains(out, "Status 201:") || !strings.Contains(out, fmt.Sprintf("Checks: %d passed, 0 failed", got*2)) || strings.Contains(out, "Body:") {
		t.Errorf("unexpected load summary:\n%s", out)
	}
}

func TestLoadStartsImmediately(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	out, err := executeSend(nil, "send", server.URL, "--rps", "1", "--duration", "100ms")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || !strings.Contains(out, "Requests: 1 completed") {
		t.Errorf("calls = %d, summary:\n%s", calls.Load(), out)
	}
}

func TestLoadCyclesMatchedCases(t *testing.T) {
	var one, two atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := strings.TrimPrefix(r.URL.Path, "/")
		if value == "one" {
			one.Add(1)
		} else if value == "two" {
			two.Add(1)
		} else {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"value":%q}`, value)
	}))
	defer server.Close()

	out, err := executeSend(nil, "send", server.URL+"/{value}", "--match", "value=(one, two)", "--rps", "20", "--duration", "220ms", "--expect-json", "value={value}")
	if err != nil {
		t.Fatal(err)
	}
	if one.Load() == 0 || two.Load() == 0 || !strings.Contains(out, "0 failed") {
		t.Errorf("requests to one=%d, two=%d; summary:\n%s", one.Load(), two.Load(), out)
	}
}

func TestLoadFailsOnCheckAndDoesNotFollowRedirect(t *testing.T) {
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
	}))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer server.Close()

	out, err := executeSend(nil, "send", server.URL, "--rps", "20", "--duration", "160ms", "--expect-status", "200")
	if !errors.Is(err, ErrRunFailed) || !strings.Contains(out, "Status 302:") || !strings.Contains(out, "0 passed") || !strings.Contains(out, "failed") {
		t.Errorf("error = %v, summary:\n%s", err, out)
	}
	if redirected.Load() != 0 {
		t.Errorf("redirect destination received %d requests", redirected.Load())
	}
}

func TestLoadValidatesOptionsAndTarget(t *testing.T) {
	for _, args := range [][]string{
		{"send", "https://example.com", "--rps", strconv.Itoa(maxRemoteRPS + 1), "--duration", "100ms"},
		{"send", "https://example.com", "--rps", "10", "-c", strconv.Itoa(maxRemoteClients + 1)},
		{"send", "https://example.com", "--rps", "10", "--duration", (maxRemoteDuration + time.Second).String()},
		{"send", "http://127.0.0.1:1", "--rps", "0"},
		{"send", "http://127.0.0.1:1", "--rps", strconv.Itoa(maxLoadRPS + 1)},
		{"send", "http://127.0.0.1:1", "--rps", "10", "-c", strconv.Itoa(maxLoadClients + 1)},
		{"send", "http://127.0.0.1:1", "--rps", "10", "--duration", (maxLoadDuration + time.Second).String()},
		{"send", "http://127.0.0.1:1", "-c", "2"},
	} {
		if _, err := executeSend(nil, args...); err == nil {
			t.Errorf("expected validation error for %q", args)
		}
	}
}

func TestLoadTargetClassification(t *testing.T) {
	for _, test := range []struct {
		url   string
		local bool
	}{
		{"http://localhost:8080/api", true},
		{"http://127.0.0.1:8080/api", true},
		{"http://[::1]:8080/api", true},
		{"https://api.test.example.com/health", false},
	} {
		local, err := isLocalTarget(test.url)
		if err != nil || local != test.local {
			t.Errorf("isLocalTarget(%q) = %t, %v; want %t", test.url, local, err, test.local)
		}
	}
	if _, err := isLocalTarget("file:///tmp/request"); err == nil {
		t.Fatal("expected invalid scheme error")
	}
}

func TestNormalizeArgs(t *testing.T) {
	got := NormalizeArgs([]string{"send", "-rps", "10", "-rps=20", "--", "-rps"})
	want := []string{"send", "--rps", "10", "--rps=20", "--", "-rps"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

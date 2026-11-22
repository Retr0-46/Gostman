package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSendShowsCompleteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/items" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Values("X-Trace"); len(got) != 2 || got[0] != "first" || got[1] != "second" {
			t.Errorf("X-Trace = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != `{"name":"test"}` {
			t.Errorf("body = %q, error = %v", body, err)
		}
		w.Header().Set("X-Result", "created")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("saved"))
	}))
	defer server.Close()

	out, err := executeSend(nil, "send", server.URL+"/items", "-X", "POST", "-H", "X-Trace: first", "-H", "X-Trace: second", "-d", `{"name":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Request: POST " + server.URL + "/items",
		"Final URL: " + server.URL + "/items",
		"Status: 201 Created",
		"Protocol: HTTP/1.1",
		"Duration:",
		"Body size: 5 bytes",
		"X-Result: created",
		"Body:\nsaved\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSendBodySources(t *testing.T) {
	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	file := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(file, []byte("from file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSend(nil, "send", server.URL, "--data-file", file); err != nil {
		t.Fatal(err)
	}
	if _, err := executeSend(strings.NewReader("from stdin"), "send", server.URL, "--data-file", "-"); err != nil {
		t.Fatal(err)
	}
	if first, second := <-received, <-received; first != "from file" || second != "from stdin" {
		t.Fatalf("received bodies = %q, %q", first, second)
	}
}

func TestSendRejectsInvalidFlags(t *testing.T) {
	for _, args := range [][]string{
		{"send", "https://example.com", "-H", "invalid"},
		{"send", "https://example.com", "-X", ""},
		{"send", "https://example.com", "--timeout", "0s"},
		{"send", "https://example.com", "-d", "text", "--data-file", "body.txt"},
	} {
		if _, err := executeSend(nil, args...); err == nil {
			t.Errorf("expected error for %q", args)
		}
	}
}

func TestSendMatchedRequests(t *testing.T) {
	type receivedRequest struct {
		path string
		id   json.Number
	}
	received := make(chan receivedRequest, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID json.Number `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		received <- receivedRequest{path: r.URL.Path, id: body.ID}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	out, err := executeSend(nil, "send", server.URL+"/{value}", "-X", "POST", "-H", "Content-Type: application/json", "-d", `{"id":"{value}"}`, "--match", "{value}=(1, 2, 3)")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		got := <-received
		want := strconv.Itoa(i)
		if got.path != "/"+want || string(got.id) != want {
			t.Errorf("request %d = %+v", i, got)
		}
		if !strings.Contains(out, "Case "+want+"/3: value="+want) {
			t.Errorf("missing case %d in output:\n%s", i, out)
		}
	}
	if !strings.Contains(out, "Completed: requests=3, errors=0") {
		t.Errorf("missing summary:\n%s", out)
	}
}

func TestSendContinuesAfterMatchedRequestFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	out, err := executeSend(nil, "send", server.URL+"/{value}", "--match", "value=(fail, ok)")
	if err == nil {
		t.Fatal("expected a request failure")
	}
	if !strings.Contains(out, "Case 2/2: value=ok") || !strings.Contains(out, "Status: 204 No Content") || !strings.Contains(out, "Completed: requests=2, errors=1") {
		t.Errorf("second request did not complete:\n%s", out)
	}
}

func executeSend(in io.Reader, args ...string) (string, error) {
	cmd := NewRootCmd()
	cmd.SetArgs(args)
	if in != nil {
		cmd.SetIn(in)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := cmd.Execute()
	return out.String(), err
}

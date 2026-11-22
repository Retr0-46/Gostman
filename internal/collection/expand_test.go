package collection

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestExpandURLAndJSON(t *testing.T) {
	req := &Request{
		Method: http.MethodPost,
		URL:    "https://example.com/items/{id}?name={name}",
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"X-Case":       {"test-{name}"},
		},
		Body: []byte(`{"id":"{id}","name":"{name}","nested":["item-{name}"]}`),
	}
	cases, err := Expand(req, []string{"{id}=(1, 2)", `name=("red,blue", green)`})
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 4 {
		t.Fatalf("got %d cases", len(cases))
	}
	for i, testCase := range cases {
		id := i/2 + 1
		name := []string{"red,blue", "green"}[i%2]
		wantURL := "https://example.com/items/" + strconv.Itoa(id) + "?name=" + url.QueryEscape(name)
		if testCase.Request.URL != wantURL {
			t.Errorf("case %d URL = %q, want %q", i, testCase.Request.URL, wantURL)
		}
		if got := testCase.Request.Headers.Get("X-Case"); got != "test-"+name {
			t.Errorf("case %d X-Case = %q", i, got)
		}
		var body struct {
			ID     int      `json:"id"`
			Name   string   `json:"name"`
			Nested []string `json:"nested"`
		}
		if err := json.Unmarshal(testCase.Request.Body, &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != id || body.Name != name || len(body.Nested) != 1 || body.Nested[0] != "item-"+name {
			t.Errorf("case %d body = %+v", i, body)
		}
	}
	if string(req.Body) != `{"id":"{id}","name":"{name}","nested":["item-{name}"]}` {
		t.Fatal("original request body was modified")
	}
}

func TestExpandEscapesPathAndJSONStrings(t *testing.T) {
	req := &Request{
		URL:     "https://example.com/{value}",
		Headers: http.Header{"Content-Type": {"application/json"}},
		Body:    []byte(`{"value":"{value}"}`),
	}
	cases, err := Expand(req, []string{"value=(a/b, hello world)"})
	if err != nil {
		t.Fatal(err)
	}
	if cases[0].Request.URL != "https://example.com/a%2Fb" {
		t.Errorf("escaped path = %q", cases[0].Request.URL)
	}
	if cases[1].Request.URL != "https://example.com/hello%20world" {
		t.Errorf("escaped path = %q", cases[1].Request.URL)
	}
	if string(cases[1].Request.Body) != `{"value":"hello world"}` {
		t.Errorf("JSON body = %s", cases[1].Request.Body)
	}
}

func TestExpandRejectsInvalidMatches(t *testing.T) {
	req := &Request{URL: "https://example.com/{value}"}
	for _, specs := range [][]string{
		{"value"},
		{"value=()"},
		{"value=1", "value=2"},
		{"unused=1"},
		{"bad-name=1"},
	} {
		if _, err := Expand(req, specs); err == nil {
			t.Errorf("expected error for %q", specs)
		}
	}
	values := make([]string, 101)
	for i := range values {
		values[i] = strconv.Itoa(i)
	}
	if _, err := Expand(req, []string{"value=" + strings.Join(values, ",")}); err == nil {
		t.Fatal("expected request limit error")
	}

	badJSON := &Request{
		URL:     req.URL,
		Headers: http.Header{"Content-Type": {"application/json"}},
		Body:    []byte(`{"value":{value}}`),
	}
	if _, err := Expand(badJSON, []string{"value=1"}); err == nil {
		t.Fatal("expected invalid JSON template error")
	}
}

func TestExpandAllowsMatchUsedOnlyInExpectation(t *testing.T) {
	req := &Request{URL: "https://example.com/{name}"}
	cases, err := Expand(req, []string{"name=(one, two)", "code=(200, 201)"}, "status={code}")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 4 {
		t.Fatalf("got %d cases, want 4", len(cases))
	}
	if cases[1].Request.URL != "https://example.com/one" || cases[1].Values["code"] != "201" {
		t.Errorf("second case = %+v", cases[1])
	}
}

package expect

import (
	"net/http"
	"strings"
	"testing"

	"gostman/internal/collection"
	"gostman/internal/httpclient"
)

func TestCompileAndCheckPerCaseExpectations(t *testing.T) {
	cases := []collection.Case{
		{Values: map[string]string{"name": "Alice", "code": "200"}},
		{Values: map[string]string{"name": "Bob", "code": "404"}},
	}
	suite, err := Compile([]string{
		"status=(200, 404)",
		"status={code}",
		"header:Content-Type=application/json",
		"json:/data/id=(1, 2)",
		"json:/name={name}",
		"json:/items/0/active=true",
	}, cases)
	if err != nil {
		t.Fatal(err)
	}
	for i, code := range []int{200, 404} {
		name := []string{"Alice", "Bob"}[i]
		resp := &httpclient.Response{
			StatusCode: code,
			Headers:    http.Header{"Content-Type": {"application/json"}},
			Body:       []byte(`{"data":{"id":` + []string{"1", "2"}[i] + `},"name":"` + name + `","items":[{"active":true}]}`),
		}
		results := suite.Check(i, resp)
		if len(results) != 6 {
			t.Fatalf("case %d: got %d results", i, len(results))
		}
		for _, result := range results {
			if !result.Passed {
				t.Errorf("case %d: failed %+v", i, result)
			}
		}
	}
}

func TestCheckReportsMismatches(t *testing.T) {
	suite, err := Compile([]string{
		"status=200",
		"header:X-Test=ready",
		"json:/id=1",
	}, []collection.Case{{}})
	if err != nil {
		t.Fatal(err)
	}
	results := suite.Check(0, &httpclient.Response{StatusCode: 404, Headers: make(http.Header), Body: []byte("not JSON")})
	if len(results) != 3 || results[0].Passed || results[1].Passed || results[2].Passed {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[2].Actual != "<invalid JSON>" {
		t.Errorf("JSON mismatch = %+v", results[2])
	}
}

func TestCompileRejectsInvalidExpectations(t *testing.T) {
	cases := []collection.Case{{}, {}}
	for _, spec := range []string{
		"status=(200, 404, 500)",
		"status=abc",
		"status=99",
		"json:id=1",
		"unknown=value",
		"status",
	} {
		if _, err := Compile([]string{spec}, cases); err == nil {
			t.Errorf("expected error for %q", spec)
		}
	}
	if _, err := Compile([]string{"status=(200,404"}, cases); err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Errorf("unexpected list error: %v", err)
	}
}

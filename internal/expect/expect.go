package expect

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gostman/internal/collection"
	"gostman/internal/httpclient"
)

type kind int

const (
	statusKind kind = iota
	headerKind
	jsonKind
)

type rule struct {
	kind   kind
	target string
	key    string
	status int
	text   string
	value  any
}

type Suite struct {
	cases [][]rule
}

type Result struct {
	Target   string
	Expected string
	Actual   string
	Passed   bool
}

func Compile(specs []string, cases []collection.Case) (*Suite, error) {
	suite := &Suite{cases: make([][]rule, len(cases))}
	for _, spec := range specs {
		target, raw, ok := strings.Cut(spec, "=")
		if !ok || target == "" {
			return nil, fmt.Errorf("invalid expectation %q: expected 'status=200', 'header:Name=value', or 'json:/path=value'", spec)
		}
		values, err := parseValues(raw, len(cases))
		if err != nil {
			return nil, fmt.Errorf("expectation %q: %w", spec, err)
		}
		for i, testCase := range cases {
			value := values[0]
			if len(values) > 1 {
				value = values[i]
			}
			value = substitute(value, testCase.Values)
			item := rule{target: target, text: value}
			switch {
			case target == "status":
				item.kind = statusKind
				item.status, err = strconv.Atoi(value)
				if err != nil || item.status < 100 || item.status > 599 {
					return nil, fmt.Errorf("case %d: invalid expected HTTP status %q", i+1, value)
				}
			case strings.HasPrefix(target, "header:") && len(target) > len("header:"):
				item.kind = headerKind
				item.key = strings.TrimPrefix(target, "header:")
			case strings.HasPrefix(target, "json:"):
				item.kind = jsonKind
				item.key = strings.TrimPrefix(target, "json:")
				if item.key != "" && !strings.HasPrefix(item.key, "/") {
					return nil, fmt.Errorf("invalid JSON pointer %q: expected /path", item.key)
				}
				item.value = parseJSONValue(value)
			default:
				return nil, fmt.Errorf("unknown expectation target %q", target)
			}
			suite.cases[i] = append(suite.cases[i], item)
		}
	}
	return suite, nil
}

func parseValues(raw string, count int) ([]string, error) {
	if !strings.HasPrefix(raw, "(") {
		return []string{raw}, nil
	}
	if !strings.HasSuffix(raw, ")") {
		return nil, fmt.Errorf("unterminated value list")
	}
	reader := csv.NewReader(strings.NewReader(raw[1 : len(raw)-1]))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil || len(records) != 1 {
		return nil, fmt.Errorf("invalid comma-separated value list")
	}
	values := records[0]
	if len(values) != count {
		return nil, fmt.Errorf("got %d expected values for %d cases", len(values), count)
	}
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	return values, nil
}

func substitute(input string, values map[string]string) string {
	replacements := make([]string, 0, len(values)*2)
	for name, value := range values {
		replacements = append(replacements, "{"+name+"}", value)
	}
	return strings.NewReplacer(replacements...).Replace(input)
}

func parseJSONValue(raw string) any {
	if !json.Valid([]byte(raw)) {
		return raw
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	_ = decoder.Decode(&value)
	return value
}

func (s *Suite) Check(index int, resp *httpclient.Response) []Result {
	results := make([]Result, 0, len(s.cases[index]))
	var document any
	var jsonErr error
	jsonLoaded := false
	for _, item := range s.cases[index] {
		result := Result{Target: item.target}
		switch item.kind {
		case statusKind:
			result.Expected = strconv.Itoa(item.status)
			result.Actual = strconv.Itoa(resp.StatusCode)
			result.Passed = resp.StatusCode == item.status
		case headerKind:
			result.Expected = strconv.Quote(item.text)
			values := resp.Headers.Values(item.key)
			result.Actual = "<missing>"
			if len(values) > 0 {
				result.Actual = strconv.Quote(strings.Join(values, ", "))
				for _, value := range values {
					if value == item.text {
						result.Passed = true
					}
				}
			}
		case jsonKind:
			if !jsonLoaded {
				jsonLoaded = true
				if !json.Valid(resp.Body) {
					jsonErr = fmt.Errorf("response body is not JSON")
				} else {
					decoder := json.NewDecoder(bytes.NewReader(resp.Body))
					decoder.UseNumber()
					jsonErr = decoder.Decode(&document)
				}
			}
			result.Expected = formatJSON(item.value)
			result.Actual = "<missing>"
			if jsonErr != nil {
				result.Actual = "<invalid JSON>"
			} else if actual, found := atPointer(document, item.key); found {
				result.Actual = formatJSON(actual)
				result.Passed = reflect.DeepEqual(actual, item.value)
			}
		}
		results = append(results, result)
	}
	return results
}

func atPointer(value any, pointer string) (any, bool) {
	if pointer == "" {
		return value, true
	}
	for _, segment := range strings.Split(pointer[1:], "/") {
		segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		switch current := value.(type) {
		case map[string]any:
			var found bool
			value, found = current[segment]
			if !found {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(current) {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}

func formatJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

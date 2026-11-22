package collection

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const maxCases = 100

var (
	matchNamePattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	placeholderPattern = regexp.MustCompile(`\{[A-Za-z_][A-Za-z0-9_]*\}`)
)

type Match struct {
	Name   string
	Values []string
}

type Case struct {
	Request *Request
	Label   string
	Values  map[string]string
}

func ParseMatch(spec string) (Match, error) {
	name, rawValues, ok := strings.Cut(spec, "=")
	if !ok {
		return Match{}, fmt.Errorf("invalid match %q: expected '{name}=(value1, value2)'", spec)
	}
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}") {
		name = name[1 : len(name)-1]
	}
	if !matchNamePattern.MatchString(name) {
		return Match{}, fmt.Errorf("invalid match name %q", name)
	}

	rawValues = strings.TrimSpace(rawValues)
	if strings.HasPrefix(rawValues, "(") && strings.HasSuffix(rawValues, ")") {
		rawValues = strings.TrimSpace(rawValues[1 : len(rawValues)-1])
	}
	if rawValues == "" {
		return Match{}, fmt.Errorf("match %q has no values", name)
	}
	reader := csv.NewReader(strings.NewReader(rawValues))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil || len(records) != 1 {
		return Match{}, fmt.Errorf("invalid values for match %q: expected a comma-separated list", name)
	}
	for i := range records[0] {
		records[0][i] = strings.TrimSpace(records[0][i])
	}
	return Match{Name: name, Values: records[0]}, nil
}

func Expand(req *Request, specs []string, expectations ...string) ([]Case, error) {
	if len(specs) == 0 {
		if len(req.Form) > 0 {
			rendered, err := renderRequest(req, nil)
			if err != nil {
				return nil, err
			}
			return []Case{{Request: rendered}}, nil
		}
		return []Case{{Request: req}}, nil
	}

	matches := make([]Match, 0, len(specs))
	seen := make(map[string]bool)
	count := 1
	for _, spec := range specs {
		match, err := ParseMatch(spec)
		if err != nil {
			return nil, err
		}
		if seen[match.Name] {
			return nil, fmt.Errorf("duplicate match %q", match.Name)
		}
		seen[match.Name] = true
		token := "{" + match.Name + "}"
		used := requestContains(req, token)
		for _, expectation := range expectations {
			used = used || strings.Contains(expectation, token)
		}
		if !used {
			return nil, fmt.Errorf("placeholder {%s} is not used in the request or expectations", match.Name)
		}
		if count > maxCases/len(match.Values) {
			return nil, fmt.Errorf("too many request variants: maximum is %d", maxCases)
		}
		count *= len(match.Values)
		matches = append(matches, match)
	}

	cases := make([]Case, 0, count)
	bindings := make(map[string]string, len(matches))
	var visit func(int, []string) error
	visit = func(index int, labels []string) error {
		if index == len(matches) {
			rendered, err := renderRequest(req, bindings)
			if err != nil {
				return err
			}
			values := make(map[string]string, len(bindings))
			for name, value := range bindings {
				values[name] = value
			}
			cases = append(cases, Case{Request: rendered, Label: strings.Join(labels, ", "), Values: values})
			return nil
		}
		match := matches[index]
		for _, value := range match.Values {
			bindings[match.Name] = value
			if err := visit(index+1, append(labels, match.Name+"="+value)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(0, nil); err != nil {
		return nil, err
	}
	return cases, nil
}

func requestContains(req *Request, token string) bool {
	if strings.Contains(req.URL, token) || bytes.Contains(req.Body, []byte(token)) {
		return true
	}
	for _, part := range req.Form {
		if strings.Contains(part.Name, token) || (!part.IsFile && strings.Contains(part.Value, token)) {
			return true
		}
	}
	for name, values := range req.Headers {
		if strings.Contains(name, token) {
			return true
		}
		for _, value := range values {
			if strings.Contains(value, token) {
				return true
			}
		}
	}
	return false
}

func renderRequest(req *Request, bindings map[string]string) (*Request, error) {
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("parse URL template: %w", err)
	}
	escapedPath := parsedURL.EscapedPath()
	pathReplacements := make([]string, 0, len(bindings)*2)
	for name, value := range bindings {
		pathReplacements = append(pathReplacements, url.PathEscape("{"+name+"}"), url.PathEscape(value))
	}
	parsedURL.Path = replacePlaceholders(parsedURL.Path, bindings)
	parsedURL.RawPath = strings.NewReplacer(pathReplacements...).Replace(escapedPath)
	queryValues := make(map[string]string, len(bindings))
	for name, value := range bindings {
		queryValues[name] = url.QueryEscape(value)
	}
	parsedURL.RawQuery = replacePlaceholders(parsedURL.RawQuery, queryValues)
	parsedURL.Fragment = replacePlaceholders(parsedURL.Fragment, bindings)

	headers := make(http.Header, len(req.Headers))
	for name, values := range req.Headers {
		for _, value := range values {
			headers.Add(replacePlaceholders(name, bindings), replacePlaceholders(value, bindings))
		}
	}

	body := bytes.Clone(req.Body)
	if len(req.Form) > 0 {
		var contentType string
		body, contentType, err = renderMultipart(req.Form, bindings)
		if err != nil {
			return nil, err
		}
		headers.Set("Content-Type", contentType)
	} else if containsAnyPlaceholder(body, bindings) {
		if isJSON(headers.Get("Content-Type")) || json.Valid(body) {
			if !json.Valid(body) {
				return nil, fmt.Errorf("JSON body template must be valid JSON: wrap placeholders in quotes")
			}
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.UseNumber()
			var document any
			if err := decoder.Decode(&document); err != nil {
				return nil, fmt.Errorf("parse JSON body template: %w", err)
			}
			body, err = json.Marshal(renderJSON(document, bindings))
			if err != nil {
				return nil, fmt.Errorf("render JSON body: %w", err)
			}
		} else {
			body = []byte(replacePlaceholders(string(body), bindings))
		}
	}

	return &Request{
		Name:    req.Name,
		Method:  req.Method,
		URL:     parsedURL.String(),
		Headers: headers,
		Body:    body,
	}, nil
}

func containsAnyPlaceholder(body []byte, bindings map[string]string) bool {
	for name := range bindings {
		if bytes.Contains(body, []byte("{"+name+"}")) {
			return true
		}
	}
	return false
}

func replacePlaceholders(input string, bindings map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(input, func(token string) string {
		if value, ok := bindings[token[1:len(token)-1]]; ok {
			return value
		}
		return token
	})
}

func renderJSON(value any, bindings map[string]string) any {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			value[key] = renderJSON(child, bindings)
		}
	case []any:
		for i, child := range value {
			value[i] = renderJSON(child, bindings)
		}
	case string:
		if placeholderPattern.FindString(value) == value && value != "" {
			if raw, ok := bindings[value[1:len(value)-1]]; ok && json.Valid([]byte(raw)) {
				decoder := json.NewDecoder(strings.NewReader(raw))
				decoder.UseNumber()
				var literal any
				if err := decoder.Decode(&literal); err == nil {
					return literal
				}
			}
		}
		return replacePlaceholders(value, bindings)
	}
	return value
}

func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && (mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"))
}

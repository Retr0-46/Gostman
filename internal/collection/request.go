package collection

import "net/http"

type Request struct {
	Name    string
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
	Form    []FormPart
}

type FormPart struct {
	Name        string
	Value       string
	Filename    string
	ContentType string
	Data        []byte
	IsFile      bool
}

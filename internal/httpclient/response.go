package httpclient

import (
	"net/http"
	"time"
)

type Response struct {
	URL        string
	Proto      string
	StatusCode int
	Status     string
	Headers    http.Header
	Body       []byte
	Duration   time.Duration
}

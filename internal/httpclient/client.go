package httpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"gostman/internal/collection"
)

type Client struct {
	http *http.Client
}

func New() *Client {
	return NewWithTimeout(30 * time.Second)
}

func NewWithTimeout(timeout time.Duration) *Client {
	return &Client{http: &http.Client{Timeout: timeout}}
}

func NewWithoutRedirects(timeout time.Duration) *Client {
	return &Client{http: &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (c *Client) Do(ctx context.Context, req *collection.Request) (*Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, values := range req.Headers {
		for _, value := range values {
			httpReq.Header.Add(k, value)
		}
	}

	start := time.Now()
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return &Response{
		URL:        httpResp.Request.URL.String(),
		Proto:      httpResp.Proto,
		StatusCode: httpResp.StatusCode,
		Status:     httpResp.Status,
		Headers:    httpResp.Header,
		Body:       body,
		Duration:   time.Since(start),
	}, nil
}

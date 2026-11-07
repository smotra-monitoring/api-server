// Package ratelimit talks to the external rate-limiter service and applies
// local caching and circuit-breaking on top of its allow/deny answers.
package ratelimit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Checker asks the limiter service whether a client IP may proceed.
type Checker interface {
	// Allow returns true for HTTP 200 and false for HTTP 429; any other outcome is an error.
	Allow(ctx context.Context, ip string) (bool, error)
}

// Client is a Checker backed by the limiter's HTTP API.
type Client struct {
	url        string
	ctxTimeout time.Duration
	http       *http.Client
}

type checkRequest struct {
	IP string `json:"ip"`
}

// NewClient creates a Client that reuses keep-alive connections to the limiter.
func NewClient(url string, timeout time.Duration) *Client {
	transport := &http.Transport{
		// this timeout only measured DNS and TCP handshake
		DialContext:         (&net.Dialer{Timeout: timeout}).DialContext,
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Client{
		url: url,
		// the context timeout is used for the request context
		ctxTimeout: timeout,
		// the http.Client.Timeout covers the entire request, including reading the response body
		// we rely on it to enforce the overall request timeout.
		// other timeouts are sort of redundant
		http: &http.Client{Transport: transport, Timeout: timeout},
	}
}

// Allow implements Checker.
func (c *Client) Allow(ctx context.Context, ip string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.ctxTimeout)
	defer cancel()

	body, err := json.Marshal(checkRequest{IP: ip})
	if err != nil {
		return false, fmt.Errorf("marshal limiter request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("build limiter request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("limiter request: %w", err)
	}
	defer resp.Body.Close()
	// Drain so the connection returns to the keep-alive pool.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusTooManyRequests:
		return false, nil
	default:
		return false, fmt.Errorf("limiter returned unexpected status %d", resp.StatusCode)
	}
}

package processor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockRoundTripper is a mock http.RoundTripper that can be configured to return
// specific status codes and counts for testing retry behavior.
type mockRoundTripper struct {
	mu        sync.Mutex
	callCount int
	responses []mockResponse
}

type mockResponse struct {
	statusCode int
	body       string
	panicErr   error // optional panic to simulate network errors
}

func newMockRoundTripper(responses []mockResponse) *mockRoundTripper {
	return &mockRoundTripper{
		responses: responses,
	}
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.callCount >= len(m.responses) {
		// If out of configured responses, return the last one repeatedly
		resp := m.responses[len(m.responses)-1]
		return mockResponseToHTTP(req.Context(), resp)
	}

	resp := m.responses[m.callCount]
	m.callCount++
	return mockResponseToHTTP(req.Context(), resp)
}

func mockResponseToHTTP(ctx context.Context, mr mockResponse) (*http.Response, error) {
	if mr.panicErr != nil {
		return nil, mr.panicErr
	}
	body := io.NopCloser(strings.NewReader(mr.body))
	// Create a minimal request with context
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://test", nil)
	return &http.Response{
		StatusCode: mr.statusCode,
		Body:       body,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// TestHAClient_RetryOn429 tests that the client retries on 429 status.
func TestHAClient_RetryOn429(t *testing.T) {
	// Simulate: 429, then 429, then 200
	responses := []mockResponse{
		{statusCode: http.StatusTooManyRequests, body: `{"error":"rate limit"}`},
		{statusCode: http.StatusTooManyRequests, body: `{"error":"rate limit"}`},
		{statusCode: http.StatusOK, body: `{"result":"ok"}`},
	}

	transport := newMockRoundTripper(responses)
	client := &haClient{
		baseURL:        "http://test",
		token:          "token",
		client:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
		maxRetries:     3,
		initialBackoff: 10 * time.Millisecond, // Short for tests
		maxBackoff:     100 * time.Millisecond,
		backoffFactor:  2.0,
	}

	ctx := context.Background()
	body, err := client.Get(ctx, "/api/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if !strings.Contains(string(data), "ok") {
		t.Errorf("expected 'ok' in response, got: %s", string(data))
	}

	// Verify that we made 3 attempts (2 failures + 1 success)
	transport.mu.Lock()
	callCount := transport.callCount
	transport.mu.Unlock()
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// TestHAClient_RetryOn5xx tests that the client retries on 5xx status.
func TestHAClient_RetryOn5xx(t *testing.T) {
	// Simulate: 500, then 503, then 200
	responses := []mockResponse{
		{statusCode: http.StatusInternalServerError, body: `{"error":"server error"}`},
		{statusCode: http.StatusServiceUnavailable, body: `{"error":"unavailable"}`},
		{statusCode: http.StatusOK, body: `{"result":"success"}`},
	}

	transport := newMockRoundTripper(responses)
	client := &haClient{
		baseURL:        "http://test",
		token:          "token",
		client:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
		maxRetries:     3,
		initialBackoff: 10 * time.Millisecond,
		maxBackoff:     100 * time.Millisecond,
		backoffFactor:  2.0,
	}

	ctx := context.Background()
	body, err := client.Get(ctx, "/api/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if !strings.Contains(string(data), "success") {
		t.Errorf("expected 'success' in response, got: %s", string(data))
	}

	transport.mu.Lock()
	callCount := transport.callCount
	transport.mu.Unlock()
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// TestHAClient_NoRetryOn4xx tests that the client does NOT retry on non-429 4xx errors.
func TestHAClient_NoRetryOn4xx(t *testing.T) {
	// Simulate a single 400 Bad Request
	responses := []mockResponse{
		{statusCode: http.StatusBadRequest, body: `{"error":"bad request"}`},
	}

	transport := newMockRoundTripper(responses)
	client := &haClient{
		baseURL:        "http://test",
		token:          "token",
		client:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
		maxRetries:     3,
		initialBackoff: 10 * time.Millisecond,
		maxBackoff:     100 * time.Millisecond,
		backoffFactor:  2.0,
	}

	ctx := context.Background()
	body, err := client.Get(ctx, "/api/test")
	if err == nil {
		body.Close()
		t.Fatal("expected error for 400 status, got none")
	}

	// Should not retry - only 1 call
	transport.mu.Lock()
	callCount := transport.callCount
	transport.mu.Unlock()
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry), got %d", callCount)
	}
}

// TestHAClient_RetryExhausted tests that after max retries, an error is returned.
func TestHAClient_RetryExhausted(t *testing.T) {
	// Simulate: always 429
	responses := []mockResponse{
		{statusCode: http.StatusTooManyRequests, body: `{"error":"rate limit"}`},
		{statusCode: http.StatusTooManyRequests, body: `{"error":"rate limit"}`},
		{statusCode: http.StatusTooManyRequests, body: `{"error":"rate limit"}`},
		{statusCode: http.StatusTooManyRequests, body: `{"error":"rate limit"}`}, // 4th should not be called
	}

	transport := newMockRoundTripper(responses)
	client := &haClient{
		baseURL:        "http://test",
		token:          "token",
		client:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
		maxRetries:     3,
		initialBackoff: 10 * time.Millisecond,
		maxBackoff:     100 * time.Millisecond,
		backoffFactor:  2.0,
	}

	ctx := context.Background()
	_, err := client.Get(ctx, "/api/test")
	if err == nil {
		t.Fatal("expected error after retries exhausted, got none")
	}

	// Should have made 4 attempts (initial + 3 retries = maxRetries+1?)
	// Actually implementation does: for attempt 0 to maxRetries inclusive, that's maxRetries+1 attempts.
	// But after maxRetries failures, it returns an error without making another attempt.
	// In our code: attempt 0 (first), if fails, retry up to maxRetries times total.
	// Let's check: if maxRetries=3, we can have up to 4 attempts (0,1,2,3). On 3rd failure (attempt=3) we return error.
	// So total calls = maxRetries+1 when all fail.
	transport.mu.Lock()
	callCount := transport.callCount
	transport.mu.Unlock()
	if callCount != 4 {
		t.Errorf("expected 4 calls (max retries exhausted), got %d", callCount)
	}
}

// TestHAClient_RetryOnNetworkError tests that network errors trigger retry.
func TestHAClient_RetryOnNetworkError(t *testing.T) {
	responses := []mockResponse{
		{panicErr: fmt.Errorf("connection reset")},
		{panicErr: fmt.Errorf("connection reset")},
		{statusCode: http.StatusOK, body: `{"result":"ok"}`},
	}

	transport := newMockRoundTripper(responses)
	client := &haClient{
		baseURL:        "http://test",
		token:          "token",
		client:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
		maxRetries:     3,
		initialBackoff: 10 * time.Millisecond,
		maxBackoff:     100 * time.Millisecond,
		backoffFactor:  2.0,
	}

	ctx := context.Background()
	body, err := client.Get(ctx, "/api/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if !strings.Contains(string(data), "ok") {
		t.Errorf("expected 'ok' in response, got: %s", string(data))
	}

	transport.mu.Lock()
	callCount := transport.callCount
	transport.mu.Unlock()
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// TestHAClient_NoRetryOnSuccess tests that no unnecessary retries on success.
func TestHAClient_NoRetryOnSuccess(t *testing.T) {
	responses := []mockResponse{
		{statusCode: http.StatusOK, body: `{"result":"ok"}`},
	}

	transport := newMockRoundTripper(responses)
	client := &haClient{
		baseURL:        "http://test",
		token:          "token",
		client:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
		maxRetries:     3,
		initialBackoff: 10 * time.Millisecond,
		maxBackoff:     100 * time.Millisecond,
		backoffFactor:  2.0,
	}

	ctx := context.Background()
	body, err := client.Get(ctx, "/api/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body.Close()

	transport.mu.Lock()
	callCount := transport.callCount
	transport.mu.Unlock()
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

// TestHAClient_ContextCancellation tests that context cancellation stops retries.
func TestHAClient_ContextCancellation(t *testing.T) {
	responses := []mockResponse{
		{statusCode: http.StatusTooManyRequests, body: `{"error":"rate limit"}`},
	}

	transport := newMockRoundTripper(responses)
	client := &haClient{
		baseURL:        "http://test",
		token:          "token",
		client:         &http.Client{Transport: transport, Timeout: 30 * time.Second},
		maxRetries:     3,
		initialBackoff: 1 * time.Second, // Long backoff to catch cancellation
		maxBackoff:     10 * time.Second,
		backoffFactor:  2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay to allow first call to happen
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := client.Get(ctx, "/api/test")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	// Should have made at least 1 call (the initial request)
	transport.mu.Lock()
	callCount := transport.callCount
	transport.mu.Unlock()
	if callCount < 1 {
		t.Errorf("expected at least 1 call before cancellation, got %d", callCount)
	}
}

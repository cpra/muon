package llm

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type tempNetErr struct{}

func (tempNetErr) Error() string   { return "temp" }
func (tempNetErr) Timeout() bool   { return true }
func (tempNetErr) Temporary() bool { return true }

func TestIsRetryableError(t *testing.T) {
	if !isRetryableError(tempNetErr{}) {
		t.Fatal("expected retryable")
	}
	if !isRetryableError(io.ErrUnexpectedEOF) {
		t.Fatal("expected retryable EOF")
	}
	if isRetryableError(errors.New("x")) {
		t.Fatal("expected non-retryable")
	}
	if !isRetryableError(&url.Error{Err: tempNetErr{}}) {
		t.Fatal("expected wrapped retryable")
	}
	var _ net.Error = tempNetErr{}
}

func TestParsePrice(t *testing.T) {
	if _, err := parsePrice("abc"); err == nil {
		t.Fatal("expected error")
	}
	if p, err := parsePrice(""); err != nil || p != 0 {
		t.Fatal("expected zero")
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter("1"); d != time.Second {
		t.Fatalf("got %v", d)
	}
	_ = context.Background()
	_ = http.StatusTooManyRequests
}

func TestEnsureModelInfoConcurrentCallsShareSingleFetch(t *testing.T) {
	var requests int32
	firstArrived := make(chan struct{})
	allowResponse := make(chan struct{})
	var startedOnce sync.Once
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		atomic.AddInt32(&requests, 1)
		startedOnce.Do(func() { close(firstArrived) })
		<-allowResponse
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-test","context_length":123,"pricing":{"prompt":"0.1","completion":"0.2"}}]}`))
	})

	ts := http.NewServeMux()
	ts.Handle("/models", server)
	srv := httptest.NewServer(ts)
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Model: "gpt-test"})

	firstErr := make(chan error, 1)
	go func() { firstErr <- c.EnsureModelInfo(context.Background()) }()
	<-firstArrived

	done := make(chan error, 1)
	go func() { done <- c.EnsureModelInfo(context.Background()) }()

	select {
	case err := <-done:
		t.Fatalf("second call returned early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(allowResponse)
	if err := <-firstErr; err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected 1 request, got %d", got)
	}
	if got := c.ContextLength(); got != 123 {
		t.Fatalf("expected context length to be cached, got %d", got)
	}
}

func TestEnsureModelInfoCachesOnce(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-test"}]}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Model: "gpt-test"})
	if err := c.EnsureModelInfo(context.Background()); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := c.EnsureModelInfo(context.Background()); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected 1 request, got %d", got)
	}
}

//go:build !integration

package db

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name           string
		requestsPerSec float64
		burst          int
		wantTokens     float64
		wantMaxTokens  float64
		wantRefillRate float64
	}{
		{
			name:           "standard config",
			requestsPerSec: 10,
			burst:          5,
			wantTokens:     5,
			wantMaxTokens:  5,
			wantRefillRate: 10,
		},
		{
			name:           "high rate low burst",
			requestsPerSec: 100,
			burst:          1,
			wantTokens:     1,
			wantMaxTokens:  1,
			wantRefillRate: 100,
		},
		{
			name:           "zero burst",
			requestsPerSec: 10,
			burst:          0,
			wantTokens:     0,
			wantMaxTokens:  0,
			wantRefillRate: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.requestsPerSec, tt.burst)
			if rl == nil {
				t.Fatal("expected non-nil RateLimiter")
			}
			if rl.tokens != tt.wantTokens {
				t.Errorf("expected tokens %v, got %v", tt.wantTokens, rl.tokens)
			}
			if rl.maxTokens != tt.wantMaxTokens {
				t.Errorf("expected maxTokens %v, got %v", tt.wantMaxTokens, rl.maxTokens)
			}
			if rl.refillRate != tt.wantRefillRate {
				t.Errorf("expected refillRate %v, got %v", tt.wantRefillRate, rl.refillRate)
			}
			if rl.lastRefill.IsZero() {
				t.Error("expected lastRefill to be set")
			}
		})
	}
}

func TestRateLimiter_Allow_BurstBehavior(t *testing.T) {
	tests := []struct {
		name           string
		requestsPerSec float64
		burst          int
		callCount      int
		wantAllowed    int
	}{
		{
			name:           "allow up to burst then deny",
			requestsPerSec: 1,
			burst:          3,
			callCount:      5,
			wantAllowed:    3,
		},
		{
			name:           "single burst allows exactly one",
			requestsPerSec: 0,
			burst:          1,
			callCount:      3,
			wantAllowed:    1,
		},
		{
			name:           "no burst denies all",
			requestsPerSec: 0,
			burst:          0,
			callCount:      5,
			wantAllowed:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.requestsPerSec, tt.burst)

			allowed := 0
			for i := 0; i < tt.callCount; i++ {
				if rl.Allow() {
					allowed++
				}
			}

			if allowed != tt.wantAllowed {
				t.Errorf("expected %d allowed calls, got %d", tt.wantAllowed, allowed)
			}
		})
	}
}

func TestRateLimiter_Allow_TokenRefill(t *testing.T) {
	tests := []struct {
		name           string
		requestsPerSec float64
		burst          int
		consumeAll     int
		waitDuration   time.Duration
		extraCalls     int
		wantExtraAllow int
	}{
		{
			name:           "tokens refill after burst exhausted",
			requestsPerSec: 10,
			burst:          2,
			consumeAll:     2,
			waitDuration:   150 * time.Millisecond,
			extraCalls:     2,
			wantExtraAllow: 1,
		},
		{
			name:           "slow refill allows additional tokens",
			requestsPerSec: 5,
			burst:          1,
			consumeAll:     1,
			waitDuration:   250 * time.Millisecond,
			extraCalls:     3,
			wantExtraAllow: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.requestsPerSec, tt.burst)

			// Consume all burst tokens
			for i := 0; i < tt.consumeAll; i++ {
				rl.Allow()
			}

			// Wait for tokens to refill
			time.Sleep(tt.waitDuration)

			// Check how many extra calls are allowed
			allowed := 0
			for i := 0; i < tt.extraCalls; i++ {
				if rl.Allow() {
					allowed++
				}
			}

			if allowed != tt.wantExtraAllow {
				t.Errorf("expected %d extra allowed calls, got %d", tt.wantExtraAllow, allowed)
			}
		})
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	t.Run("returns immediately when tokens available", func(t *testing.T) {
		rl := NewRateLimiter(10, 1)

		start := time.Now()
		rl.Wait()
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			t.Errorf("Wait should return immediately when tokens available, took %v", elapsed)
		}
	})

	t.Run("blocks then allows after refill", func(t *testing.T) {
		// High rate so that enough tokens refill quickly
		rl := NewRateLimiter(100, 1)

		// Consume the single burst token
		if !rl.Allow() {
			t.Fatal("first Allow should succeed")
		}

		// Wait should block briefly then succeed as tokens refill
		start := time.Now()
		rl.Wait()
		elapsed := time.Since(start)

		if elapsed < 5*time.Millisecond {
			t.Errorf("Wait should have blocked, but returned instantly (elapsed: %v)", elapsed)
		}
	})
}

func TestRateLimiter_ConcurrentAllow(t *testing.T) {
	tests := []struct {
		name           string
		requestsPerSec float64
		burst          int
		goroutines     int
	}{
		{
			name:           "concurrent calls with sufficient burst",
			requestsPerSec: 0,
			burst:          100,
			goroutines:     100,
		},
		{
			name:           "concurrent calls with limited burst",
			requestsPerSec: 0,
			burst:          5,
			goroutines:     50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.requestsPerSec, tt.burst)
			var wg sync.WaitGroup
			allowed := int32(0)
			var mu sync.Mutex

			for i := 0; i < tt.goroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if rl.Allow() {
						mu.Lock()
						allowed++
						mu.Unlock()
					}
				}()
			}

			wg.Wait()

			// Allowed count should never exceed burst
			if allowed > int32(tt.burst) {
				t.Errorf("allowed %d exceeds burst limit %d", allowed, tt.burst)
			}

			// When burst >= goroutines, all should be allowed
			if tt.burst >= tt.goroutines && allowed != int32(tt.goroutines) {
				t.Errorf("expected all %d goroutines to be allowed, got %d", tt.goroutines, allowed)
			}
		})
	}
}

func TestRateLimiter_EdgeCases(t *testing.T) {
	t.Run("zero rate no bursts", func(t *testing.T) {
		rl := NewRateLimiter(0, 0)
		if rl.Allow() {
			t.Error("expected Allow to return false with zero rate and zero burst")
		}
		if rl.Allow() {
			t.Error("expected Allow to return false consistently with zero rate")
		}
	})

	t.Run("zero rate with burst", func(t *testing.T) {
		rl := NewRateLimiter(0, 3)
		allowed := 0
		for i := 0; i < 5; i++ {
			if rl.Allow() {
				allowed++
			}
		}
		if allowed != 3 {
			t.Errorf("expected exactly 3 allowed (burst), got %d", allowed)
		}
		// After burst consumed, no refills since rate is 0
		if rl.Allow() {
			t.Error("expected Allow to return false after burst exhausted with zero rate")
		}
	})

	t.Run("negative rate treated as zero refill", func(t *testing.T) {
		rl := NewRateLimiter(-5, 2)
		allowed := 0
		for i := 0; i < 5; i++ {
			if rl.Allow() {
				allowed++
			}
		}
		// Should get burst tokens but no refill
		if allowed != 2 {
			t.Errorf("expected exactly 2 allowed (burst only), got %d", allowed)
		}
	})
}

func TestRateLimitMiddleware_HealthEndpoints(t *testing.T) {
	limiter := NewRateLimiter(0, 0) // Zero capacity to ensure no rate limiting

	tests := []struct {
		name       string
		path       string
		bypassPath bool
	}{
		{name: "health endpoint", path: "/health", bypassPath: true},
		{name: "ready endpoint", path: "/ready", bypassPath: true},
		{name: "status endpoint", path: "/status", bypassPath: true},
		{name: "other endpoint", path: "/api/data", bypassPath: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.bypassPath {
				if !called {
					t.Error("handler should have been called for bypass path")
				}
				if rr.Code != http.StatusOK {
					t.Errorf("expected 200 for bypass path, got %d", rr.Code)
				}
			} else {
				// With zero burst and zero rate, non-bypass endpoints should get 429
				if rr.Code != http.StatusTooManyRequests {
					t.Errorf("expected 429 for rate-limited path, got %d", rr.Code)
				}
				if called {
					t.Error("handler should not have been called when rate limited")
				}
			}
		})
	}
}

func TestRateLimitMiddleware_429Response(t *testing.T) {
	limiter := NewRateLimiter(0, 0) // Exhaust all tokens immediately

	handler := RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called when rate limited")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", contentType)
	}

	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter != "1" {
		t.Errorf("expected Retry-After 1, got %q", retryAfter)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if body["error"] != "rate limit exceeded" {
		t.Errorf("expected error message 'rate limit exceeded', got %q", body["error"])
	}
}

func TestRateLimitMiddleware_AllowsWhenTokensAvailable(t *testing.T) {
	limiter := NewRateLimiter(10, 5) // Plenty of tokens

	called := false
	handler := RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("handler should have been called when tokens are available")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRateLimitMiddleware_MultipleRequests(t *testing.T) {
	limiter := NewRateLimiter(0, 2) // Only 2 burst tokens, no refill

	handler := RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name     string
		path     string
		wantCode int
		wantBody bool
	}{
		{name: "first request allowed", path: "/data", wantCode: 200, wantBody: true},
		{name: "second request allowed", path: "/data", wantCode: 200, wantBody: true},
		{name: "third request rate limited", path: "/data", wantCode: 429, wantBody: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, rr.Code)
			}
		})
	}
}

func TestTokensMin(t *testing.T) {
	tests := []struct {
		name string
		a    float64
		b    float64
		want float64
	}{
		{name: "a less than b", a: 1.5, b: 2.5, want: 1.5},
		{name: "a greater than b", a: 3.0, b: 1.0, want: 1.0},
		{name: "equal values", a: 2.0, b: 2.0, want: 2.0},
		{name: "both zero", a: 0.0, b: 0.0, want: 0.0},
		{name: "negative values", a: -1.0, b: -2.0, want: -2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokensMin(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("tokensMin(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

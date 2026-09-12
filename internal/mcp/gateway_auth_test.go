// Package mcp — unit test cho auth fail-closed path của 3 HTTP handler
// (HandleStreamableHTTP, HandleSSE, HandleMessage).
//
// Bối cảnh: WithHTTPContextFunc của mcp-go v0.56.0 không enforce được auth
// (chỉ ghi error vào context bên TRONG ServeHTTP), nên 3 handler phải gọi
// m.auth.Authenticate(r) trực tiếp và reject trước khi delegate. Bộ test này
// khóa chặt hành vi đó (gap-fill sau khi fix đã verify bằng curl E2E):
//
//   - Thiếu/sai token  → 401 JSON-RPC error -32001 "missing/invalid bearer token"
//   - Origin lạ         → 403 "origin not allowed"
//   - Vượt rate limit   → 429 "rate limit exceeded"
//   - auth == nil       → KHÔNG 401, delegate tiếp (auth tùy chọn khi nil)
//
// Test hermetic: token giả "test-token-123", không env ngoài, không server 3005.
package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/ti/router/tibrain/internal/security"
)

// Token giả cố định — KHÔNG phải token thật từ router.env.
const (
	authTestToken  = "test-token-123"
	authTestOrigin = "http://localhost:3005"
	// authJSONRPCCode là code JSON-RPC mà 3 handler ghi khi auth fail.
	authJSONRPCCode = `"code":-32001`
)

// authHandlerSpec mô tả 1 trong 3 HTTP handler cần test: tên, path mount thật
// (từ main.go) và cách gọi trực tiếp.
type authHandlerSpec struct {
	name string
	path string
	call func(m *Manager, w http.ResponseWriter, r *http.Request)
}

var authHandlerSpecs = []authHandlerSpec{
	{"HandleStreamableHTTP", "/mcp", (*Manager).HandleStreamableHTTP},
	{"HandleSSE", "/mcp/sse", (*Manager).HandleSSE},
	{"HandleMessage", "/mcp/message", (*Manager).HandleMessage},
}

// newAuthTestGateway dựng Manager tối giản chỉ với auth + srv + stream + sse —
// 4 field mà 3 handler đọc. KHÔNG gọi NewManager (cần memory, retriever,
// config đầy đủ). Delegate target là SDK server thật với tool rỗng: request
// pass auth sẽ rơi vào lỗi protocol của SDK (400/405), không phải lỗi auth.
func newAuthTestGateway(t *testing.T, token string, ratePerMin int) *Manager {
	t.Helper()
	srv := server.NewMCPServer("tibrain-test", "0.0-test", server.WithToolCapabilities(true))
	return &Manager{
		auth: security.NewAuthenticator(token, []string{authTestOrigin}, ratePerMin),
		srv:  srv,
		stream: server.NewStreamableHTTPServer(srv,
			server.WithEndpointPath("/mcp"),
		),
		sse: server.NewSSEServer(srv,
			server.WithSSEEndpoint("/mcp/sse"),
			server.WithMessageEndpoint("/mcp/message"),
		),
	}
}

// newAuthRequest tạo POST request (body rỗng — auth fail trả trước khi parse
// body) rồi áp mutate để set header.
func newAuthRequest(t *testing.T, path string, mutate func(*http.Request)) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, nil)
	if mutate != nil {
		mutate(r)
	}
	return r
}

// assertAuthDenied kiểm tra response fail-closed: đúng status, body chứa msg,
// envelope JSON-RPC -32001 và Content-Type application/json.
func assertAuthDenied(t *testing.T, resp *http.Response, body string, wantStatus int, wantMsg string) {
	t.Helper()
	if resp.StatusCode != wantStatus {
		t.Errorf("status = %d, want %d (body: %s)", resp.StatusCode, wantStatus, body)
	}
	if !strings.Contains(body, wantMsg) {
		t.Errorf("body %q không chứa %q", body, wantMsg)
	}
	if !strings.Contains(body, authJSONRPCCode) {
		t.Errorf("body %q thiếu JSON-RPC error envelope %s", body, authJSONRPCCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// assertNotAuthRejected kiểm tra request KHÔNG bị chặn ở tầng auth (không 401
// unauthorized, 403 origin, 429 rate limit) — tức đã được delegate sang SDK.
func assertNotAuthRejected(t *testing.T, resp *http.Response, body string) {
	t.Helper()
	for _, bad := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusTooManyRequests,
	} {
		if resp.StatusCode == bad {
			t.Errorf("status = %d (auth chặn nhầm request hợp lệ), body: %s", bad, body)
		}
	}
}

// readAuthBody đọc toàn body recorder.
func readAuthBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	return w.Body.String()
}

// TestGatewayAuthFailClosed: mọi auth fail path của CẢ 3 handler phải reject
// fail-closed với JSON-RPC error trước khi request chạm SDK server.
func TestGatewayAuthFailClosed(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*http.Request)
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "thiếu Authorization header",
			mutate:     nil,
			wantStatus: http.StatusUnauthorized,
			wantMsg:    "missing bearer token",
		},
		{
			name: "Authorization sai scheme (Basic, không phải Bearer)",
			mutate: func(r *http.Request) {
				r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
			},
			wantStatus: http.StatusUnauthorized,
			wantMsg:    "missing bearer token",
		},
		{
			name: "Bearer token rỗng",
			mutate: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer ")
			},
			wantStatus: http.StatusUnauthorized,
			wantMsg:    "missing bearer token",
		},
		{
			name: "Bearer token sai",
			mutate: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+authTestToken+"-sai")
			},
			wantStatus: http.StatusUnauthorized,
			wantMsg:    "invalid bearer token",
		},
		{
			name: "Origin không nằm trong allowed origins (kể cả token đúng)",
			mutate: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+authTestToken)
				r.Header.Set("Origin", "https://evil.example.com")
			},
			wantStatus: http.StatusForbidden,
			wantMsg:    "origin not allowed",
		},
	}

	for _, h := range authHandlerSpecs {
		t.Run(h.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					m := newAuthTestGateway(t, authTestToken, 60)
					r := newAuthRequest(t, h.path, tc.mutate)
					w := httptest.NewRecorder()
					h.call(m, w, r)

					resp := w.Result()
					body := readAuthBody(t, w)
					assertAuthDenied(t, resp, body, tc.wantStatus, tc.wantMsg)
				})
			}
		})
	}
}

// TestGatewayAuthNotConfigured: Authenticator có sẵn nhưng token rỗng
// (gateway "đóng") — mọi request vẫn bị reject 401, không lọt qua.
func TestGatewayAuthNotConfigured(t *testing.T) {
	for _, h := range authHandlerSpecs {
		t.Run(h.name, func(t *testing.T) {
			m := newAuthTestGateway(t, "", 60) // token rỗng nhưng auth != nil
			r := newAuthRequest(t, h.path, func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer bat-ky")
			})
			w := httptest.NewRecorder()
			h.call(m, w, r)

			resp := w.Result()
			body := readAuthBody(t, w)
			assertAuthDenied(t, resp, body, http.StatusUnauthorized, "gateway auth not configured")
		})
	}
}

// TestGatewayAuthNilManagerDelegates: Manager không có authenticator (auth ==
// nil) → handler KHÔNG trả 401 mà delegate sang SDK server. Đây là hành vi cố
// ý: auth tùy chọn khi cấu hình nil (test/dev mode không token).
func TestGatewayAuthNilManagerDelegates(t *testing.T) {
	for _, h := range authHandlerSpecs {
		t.Run(h.name, func(t *testing.T) {
			m := newAuthTestGateway(t, authTestToken, 60)
			m.auth = nil // Manager không có authenticator

			r := newAuthRequest(t, h.path, nil) // không token nào cả
			w := httptest.NewRecorder()
			h.call(m, w, r)

			resp := w.Result()
			body := readAuthBody(t, w)
			assertNotAuthRejected(t, resp, body)
			// Delegate đến SDK phải có response cụ thể (không phải zero-value).
			if resp.StatusCode == 0 {
				t.Errorf("handler không delegate — status 0, body: %s", body)
			}
		})
	}
}

// TestGatewayAuthValidTokenDelegates: token đúng + origin đúng → pass auth,
// request được delegate sang SDK (response là lỗi protocol 4xx, KHÔNG phải
// lỗi auth 401/403/429). Ủy quyền request hợp lệ không được nuốt.
func TestGatewayAuthValidTokenDelegates(t *testing.T) {
	for _, h := range authHandlerSpecs {
		t.Run(h.name, func(t *testing.T) {
			m := newAuthTestGateway(t, authTestToken, 60)
			r := newAuthRequest(t, h.path, func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+authTestToken)
				r.Header.Set("Origin", authTestOrigin)
			})
			w := httptest.NewRecorder()
			h.call(m, w, r)

			resp := w.Result()
			body := readAuthBody(t, w)
			assertNotAuthRejected(t, resp, body)
			if resp.StatusCode == 0 {
				t.Errorf("handler không delegate — status 0, body: %s", body)
			}
		})
	}
}

// TestGatewayAuthRateLimit: gọi vượt RateLimitPerMinute với token đúng → 429
// fail-closed "rate limit exceeded". Bucket rate-limit dùng chung giữa các
// handler (cùng Authenticator, cùng key token).
func TestGatewayAuthRateLimit(t *testing.T) {
	t.Run("HandleStreamableHTTP vượt ngưỡng riêng lẻ", func(t *testing.T) {
		const rate = 3
		m := newAuthTestGateway(t, authTestToken, rate)
		// `rate` request đầu pass auth (delegate trả lỗi protocol — không quan tâm).
		for i := 0; i < rate; i++ {
			r := newAuthRequest(t, "/mcp", func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+authTestToken)
			})
			w := httptest.NewRecorder()
			m.HandleStreamableHTTP(w, r)
			if got := w.Code; got == http.StatusUnauthorized || got == http.StatusTooManyRequests {
				t.Fatalf("request %d/%d không mong đợi bị chặn, status = %d", i+1, rate, got)
			}
		}
		// Request thứ rate+1 → 429.
		r := newAuthRequest(t, "/mcp", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+authTestToken)
		})
		w := httptest.NewRecorder()
		m.HandleStreamableHTTP(w, r)
		assertAuthDenied(t, w.Result(), readAuthBody(t, w), http.StatusTooManyRequests, "rate limit exceeded")
	})

	t.Run("bucket dùng chung giữa HandleSSE và HandleMessage", func(t *testing.T) {
		const rate = 2
		m := newAuthTestGateway(t, authTestToken, rate)
		// 2 request pass auth, mỗi request qua MỘT handler khác nhau.
		first := []struct {
			path string
			call func(m *Manager, w http.ResponseWriter, r *http.Request)
		}{
			{"/mcp/sse", (*Manager).HandleSSE},
			{"/mcp/message", (*Manager).HandleMessage},
		}
		for i, f := range first {
			r := newAuthRequest(t, f.path, func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+authTestToken)
			})
			w := httptest.NewRecorder()
			f.call(m, w, r)
			if got := w.Code; got == http.StatusUnauthorized || got == http.StatusTooManyRequests {
				t.Fatalf("request %d/%d qua %s không mong đợi bị chặn, status = %d", i+1, rate, f.path, got)
			}
		}
		// Request thứ 3 (qua HandleSSE) → 429 vì bucket đã đầy từ 2 request trước.
		r := newAuthRequest(t, "/mcp/sse", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+authTestToken)
		})
		w := httptest.NewRecorder()
		m.HandleSSE(w, r)
		assertAuthDenied(t, w.Result(), readAuthBody(t, w), http.StatusTooManyRequests, "rate limit exceeded")
	})
}

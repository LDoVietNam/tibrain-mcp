package mcp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// lookupEnv reads an env var trimmed.
func lookupEnv(k string) string {
	return os.Getenv(k)
}

// ssrfGuard rejects requests to link-local, loopback, private and cloud
// metadata destinations unless explicitly allowlisted.
func ssrfGuard(rawURL string, allowlist []string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := u.Hostname()
	for _, a := range allowlist {
		if host == a {
			return nil
		}
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("cannot resolve host %q (blocked by SSRF guard)", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("destination %q resolves to blocked address %v (SSRF guard)", host, ip)
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if ip.Equal(net.IPv4(169, 254, 169, 254)) {
		return true
	}
	return false
}

// defaultHTTPClient returns a client with timeouts and redirect limits.
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
			DisableKeepAlives: false,
			MaxIdleConns:      10,
			IdleConnTimeout:   30 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

func newHTTPRequest(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, rawURL, body)
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, max))
}

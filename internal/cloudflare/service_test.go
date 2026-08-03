package cloudflare

import "testing"

func TestNewCloudflareService(t *testing.T) {
	t.Parallel()
	svc := NewCloudflareService()
	if svc == nil {
		t.Fatal("NewCloudflareService() returned nil")
	}
}

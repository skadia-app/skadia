package llmwire

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsBlockedLLMIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"::1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.169.254",
		"100.64.0.1",
		"100.127.255.254",
		"0.0.0.0",
		"::ffff:127.0.0.1",
		"224.0.0.1",
	}
	for _, ip := range blocked {
		if !isBlockedLLMIP(ip) {
			t.Errorf("isBlockedLLMIP(%q) = false, want true", ip)
		}
	}

	allowed := []string{
		"8.8.8.8",
		"1.1.1.1",
		"2606:4700:4700::1111",
	}
	for _, ip := range allowed {
		if isBlockedLLMIP(ip) {
			t.Errorf("isBlockedLLMIP(%q) = true, want false", ip)
		}
	}
}

func TestSSRFProtectedHTTPClientDoesNotFollowRedirects(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	allowHost := strings.TrimPrefix(redirector.URL, "http://")
	targetHost := strings.TrimPrefix(target.URL, "http://")
	client := NewSSRFProtectedHTTPClient(5*time.Second, []string{allowHost, targetHost})
	defer client.CloseIdleConnections()

	resp, err := client.Get(redirector.URL)
	if err != nil {
		t.Fatalf("client.Get returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d (redirect must not be followed)", resp.StatusCode, http.StatusFound)
	}
	if targetCalled {
		t.Fatal("redirect target was called despite CheckRedirect guard")
	}
}

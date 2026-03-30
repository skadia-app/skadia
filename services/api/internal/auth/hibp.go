package auth

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var hibpClient = &http.Client{Timeout: 3 * time.Second}

// CheckHIBP checks if a password has been compromised using the
// HaveIBeenPwned k-Anonymity API (only first 5 SHA-1 chars sent).
// Returns (compromised, error). On network error: returns false, err
// (caller should warn but not block).
func CheckHIBP(password string) (bool, error) {
	hash := fmt.Sprintf("%X", sha1.Sum([]byte(password)))
	prefix := hash[:5]
	suffix := hash[5:]

	resp, err := hibpClient.Get("https://api.pwnedpasswords.com/range/" + prefix)
	if err != nil {
		return false, fmt.Errorf("HIBP API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("HIBP API returned status %d", resp.StatusCode)
	}

	// Limit response size (response is typically ~30KB)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max
	if err != nil {
		return false, fmt.Errorf("HIBP API read body: %w", err)
	}

	for line := range strings.SplitSeq(string(body), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], suffix) {
			return true, nil
		}
	}
	return false, nil
}

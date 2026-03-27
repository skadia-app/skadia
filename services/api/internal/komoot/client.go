package komoot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// komootCDNHosts is the allowlist for photo CDN hostnames.
// Only URLs with these hosts are downloaded to prevent SSRF.
var komootCDNHosts = map[string]bool{
	"images.komoot.de": true,
	"cdn.komoot.de":    true,
	"photos.komoot.de": true,
}

const (
	baseURL      = "https://api.komoot.de"
	loginPath    = "/v006/account/email/%s/"
	toursPath    = "/v007/users/%s/tours/"
	tourDetail   = "/v007/tours/%d"
	tourFIT      = "/v007/tours/%d.fit"
	tourGPX      = "/v007/tours/%d.gpx"
	requestDelay = 1 * time.Second // 1 req/s rate limit
)

// defaultHTTPClient is used for requests without an authenticated Client (Login, DownloadPhoto).
var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Authenticator abstracts auth method (Basic Auth now, OAuth2 later).
type Authenticator interface {
	ApplyAuth(req *http.Request)
}

type basicAuth struct {
	username string
	token    string
}

func (b *basicAuth) ApplyAuth(req *http.Request) {
	req.SetBasicAuth(b.username, b.token)
}

// Client is the Komoot API client.
type Client struct {
	http     *http.Client
	auth     Authenticator
	userID   string
	mu       sync.Mutex
	lastCall time.Time
}

// Login authenticates with email+password, returns a Client.
func Login(ctx context.Context, email, password string) (*Client, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf(baseURL+loginPath, email), nil)
	if err != nil {
		return nil, fmt.Errorf("komoot login request: %w", err)
	}
	req.SetBasicAuth(email, password)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("komoot login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("komoot login: invalid credentials")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return nil, fmt.Errorf("komoot login: HTTP %d: %s", resp.StatusCode, body)
	}

	var lr LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("komoot login decode: %w", err)
	}

	return &Client{
		http:   &http.Client{Timeout: 30 * time.Second},
		auth:   &basicAuth{username: lr.Username, token: lr.Password},
		userID: lr.Username,
	}, nil
}

// UserID returns the Komoot user ID (username) of the authenticated client.
func (c *Client) UserID() string { return c.userID }

// rateLimit ensures minimum delay between requests. Respects context cancellation.
func (c *Client) rateLimit(ctx context.Context) error {
	c.mu.Lock()
	wait := requestDelay - time.Since(c.lastCall)
	if wait > 0 {
		c.lastCall = time.Now().Add(wait)
	} else {
		c.lastCall = time.Now()
	}
	c.mu.Unlock()
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// doJSON performs an authenticated GET and decodes JSON.
func (c *Client) doJSON(ctx context.Context, url string, target any) error {
	if err := c.rateLimit(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	c.auth.ApplyAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("komoot request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		// Rate limited — caller should retry with backoff
		return fmt.Errorf("komoot rate limited for %s", url)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return fmt.Errorf("komoot HTTP %d for %s: %s", resp.StatusCode, url, body)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

// ListTours fetches all tours (paginated, 30 per page), stops early if all tours on a page
// are already known (knownIDs set for incremental sync). Returns newest-first.
func (c *Client) ListTours(ctx context.Context, knownIDs map[int64]bool) ([]Tour, error) {
	var all []Tour
	for page := 0; ; page++ {
		url := fmt.Sprintf(baseURL+toursPath+"?page=%d&limit=30&sort_field=date&sort_direction=desc",
			c.userID, page)
		var resp ToursResponse
		if err := c.doJSON(ctx, url, &resp); err != nil {
			return all, fmt.Errorf("list tours page %d: %w", page, err)
		}

		allKnown := true
		for _, t := range resp.Embedded.Tours {
			if !knownIDs[t.ID] {
				allKnown = false
			}
			all = append(all, t)
		}

		// Stop if page had only known tours (incremental sync optimization)
		if allKnown && len(resp.Embedded.Tours) > 0 {
			break
		}
		if page+1 >= resp.Page.TotalPages {
			break
		}
	}
	return all, nil
}

// GetTourDetail fetches tour with coordinates and timeline (for photos).
func (c *Client) GetTourDetail(ctx context.Context, tourID int64) (*TourDetailResponse, error) {
	url := fmt.Sprintf(baseURL+tourDetail+
		"?_embedded=coordinates,timeline,cover_images&directions=v2&fields=timeline&format=coordinate_array"+
		"&timeline_highlights_fields=tips,recommenders",
		tourID)
	var detail TourDetailResponse
	if err := c.doJSON(ctx, url, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// DownloadFIT downloads the FIT file for a tour. Falls back to GPX on error.
func (c *Client) DownloadFIT(ctx context.Context, tourID int64) ([]byte, string, error) {
	data, err := c.downloadFile(ctx, fmt.Sprintf(baseURL+tourFIT, tourID))
	if err == nil {
		return data, "fit", nil
	}
	// FIT generation can fail server-side (HTTP 500) — fallback to GPX
	data, err = c.downloadFile(ctx, fmt.Sprintf(baseURL+tourGPX, tourID))
	if err != nil {
		return nil, "", fmt.Errorf("download tour %d: FIT and GPX both failed: %w", tourID, err)
	}
	return data, "gpx", nil
}

// DownloadGPX downloads the GPX file for a tour (for routes).
func (c *Client) DownloadGPX(ctx context.Context, tourID int64) ([]byte, error) {
	return c.downloadFile(ctx, fmt.Sprintf(baseURL+tourGPX, tourID))
}

// downloadFile performs an authenticated GET and returns raw bytes.
func (c *Client) downloadFile(ctx context.Context, url string) ([]byte, error) {
	if err := c.rateLimit(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.auth.ApplyAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
}

// DownloadPhoto downloads a photo from a Komoot CDN URL (no auth needed).
// Validates the host against an allowlist to prevent SSRF attacks.
// Strips query params to get max quality.
func DownloadPhoto(ctx context.Context, imageURL string) ([]byte, error) {
	// Strip query params for original quality
	if idx := strings.Index(imageURL, "?"); idx > 0 {
		imageURL = imageURL[:idx]
	}

	// Validate host against CDN allowlist (SSRF prevention)
	parsed, err := url.Parse(imageURL)
	if err != nil {
		return nil, fmt.Errorf("photo URL parse error: %w", err)
	}
	if !komootCDNHosts[parsed.Hostname()] {
		return nil, fmt.Errorf("photo URL host %q not in Komoot CDN allowlist", parsed.Hostname())
	}

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("photo download HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
}

package komoot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	baseURL      = "https://api.komoot.de"
	loginPath    = "/v006/account/email/%s/"
	toursPath    = "/v007/users/%s/tours/"
	tourDetail   = "/v007/tours/%d"
	tourFIT      = "/v007/tours/%d.fit"
	tourGPX      = "/v007/tours/%d.gpx"
	requestDelay = 1 * time.Second // 1 req/s rate limit
)

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
	lastCall time.Time
}

// Login authenticates with email+password, returns a Client.
func Login(ctx context.Context, email, password string) (*Client, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf(baseURL+loginPath, email), nil)
	req.SetBasicAuth(email, password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("komoot login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("komoot login: invalid credentials")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
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

// rateLimit ensures minimum delay between requests.
func (c *Client) rateLimit() {
	elapsed := time.Since(c.lastCall)
	if elapsed < requestDelay {
		time.Sleep(requestDelay - elapsed)
	}
	c.lastCall = time.Now()
}

// doJSON performs an authenticated GET and decodes JSON.
func (c *Client) doJSON(ctx context.Context, url string, target any) error {
	c.rateLimit()
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
		// Rate limited — exponential backoff (2s, 4s, 8s)
		return fmt.Errorf("komoot rate limited for %s", url)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
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
	c.rateLimit()
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
	return io.ReadAll(resp.Body)
}

// DownloadPhoto downloads a photo from CDN (no auth needed).
// Strips query params from URL to get max quality.
func DownloadPhoto(ctx context.Context, imageURL string) ([]byte, error) {
	// Strip query params for original quality
	if idx := strings.Index(imageURL, "?"); idx > 0 {
		imageURL = imageURL[:idx]
	}

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("photo download HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

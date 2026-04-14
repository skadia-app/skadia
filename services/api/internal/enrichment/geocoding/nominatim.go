package geocoding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultNominatimURL = "https://nominatim.openstreetmap.org/reverse"

// Nominatim is a reverse-geocode Provider backed by the public Nominatim API
// (default nominatim.openstreetmap.org) or any compatible endpoint. Every
// outbound request is gated through a global 1-req/s rate limiter (see
// ratelimit.go) because the public endpoint's usage policy applies instance-wide.
type Nominatim struct {
	BaseURL string // empty → default public endpoint
	Timeout time.Duration
	Client  *http.Client // optional; if nil a timeout-scoped default is built per call
}

// Name implements Provider.
func (n *Nominatim) Name() string { return ProviderNominatim }

// nominatimResponse is the subset of the jsonv2 reverse-geocode payload we
// consume. `address` bundles the different place-level names — the exact field
// populated depends on settlement size (city for big cities, town/village for
// smaller ones, hamlet for the smallest), so we fall back through them.
//
// `Error` captures the quirk that Nominatim signals certain failure modes
// ("Unable to geocode", "No geometry") as HTTP 200 responses with an
// `{"error":"…"}` body. Without inspecting this we'd silently parse an
// empty address and treat it as a successful "no match", persisting that
// into the negative cache for CacheTTLDays. See Reverse() for the handling.
type nominatimResponse struct {
	Error   string `json:"error"`
	Address struct {
		City        string `json:"city"`
		Town        string `json:"town"`
		Village     string `json:"village"`
		Hamlet      string `json:"hamlet"`
		State       string `json:"state"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
	} `json:"address"`
}

// Reverse implements Provider. Returns (nil, nil) when the API responds with
// "no result" (empty address block, or jsonv2 error-key such as "Unable to
// geocode").
func (n *Nominatim) Reverse(ctx context.Context, lat, lng float64, lang string) (*Result, error) {
	// Validate and coord-round BEFORE waiting on the global 1 req/s limiter.
	// Otherwise a misconfigured base URL (or invalid coords → rejected by
	// the caller) would still consume a rate-limiter token, delaying the
	// next legitimate request by up to a full second.
	base := n.BaseURL
	if base == "" {
		base = defaultNominatimURL
	}
	// SSRF guard: reject loopback / RFC1918 / link-local / *.local hosts.
	// Empty BaseURL is fine — it's replaced by the public default above.
	if err := ValidateBaseURL(n.BaseURL); err != nil {
		return nil, fmt.Errorf("nominatim base url: %w", err)
	}

	lat = RoundCoord(lat)
	lng = RoundCoord(lng)
	if lang == "" {
		lang = "de,en"
	}

	// Token acquisition last, so cancelled contexts are caught by
	// waitNominatim's fast-path without burning a bucket slot. See
	// ratelimit.go for the rationale.
	if err := waitNominatim(ctx); err != nil {
		return nil, fmt.Errorf("nominatim rate-limit wait: %w", err)
	}

	extra := url.Values{}
	extra.Set("lat", fmt.Sprintf("%.3f", lat))
	extra.Set("lon", fmt.Sprintf("%.3f", lng))
	extra.Set("format", "jsonv2")
	extra.Set("addressdetails", "1")
	extra.Set("zoom", "14")
	extra.Set("accept-language", lang)

	fullURL, err := buildProviderURL(base, extra)
	if err != nil {
		return nil, fmt.Errorf("nominatim build url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("nominatim build request: %w", err)
	}
	req.Header.Set("User-Agent", CurrentUserAgent())
	req.Header.Set("Accept", "application/json")

	client := n.Client
	if client == nil {
		timeout := n.Timeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nominatim request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("nominatim status %d", resp.StatusCode)
	}

	var body nominatimResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("nominatim decode: %w", err)
	}

	// Nominatim signals some failures (invalid coords, "Unable to geocode",
	// "No geometry") as HTTP 200 with an {"error":"…"} body. Treat these
	// as soft misses: log for diagnostics but return (nil, nil) so the
	// caller caches a negative entry and moves on, rather than raising a
	// hard error that would abort the whole enrichment batch.
	if body.Error != "" {
		slog.Warn("geocoding: nominatim returned error body",
			"error", body.Error,
			"lat", lat,
			"lng", lng)
		return nil, nil
	}

	r := &Result{
		City:        firstNonEmpty(body.Address.City, body.Address.Town, body.Address.Village, body.Address.Hamlet),
		State:       body.Address.State,
		Country:     body.Address.Country,
		CountryCode: strings.ToUpper(body.Address.CountryCode),
	}
	if r.Empty() {
		return nil, nil
	}
	return r, nil
}

func firstNonEmpty(candidates ...string) string {
	for _, c := range candidates {
		if c != "" {
			return c
		}
	}
	return ""
}

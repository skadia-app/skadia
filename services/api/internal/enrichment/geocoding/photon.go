package geocoding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultPhotonURL = "https://photon.komoot.io/reverse"

// Photon is a reverse-geocode Provider backed by the public Photon instance at
// photon.komoot.io (or any compatible endpoint). Photon publishes no absolute
// rate limit, only a fair-use expectation; we therefore gate every outbound
// call through photonLimiter (5 req/s) — generous enough to feel instant on
// interactive single-activity enrichment, restrained enough that a wide
// backfill on cache misses stays inside fair-use and doesn't trigger manual
// operator intervention.
type Photon struct {
	BaseURL string // empty → default public endpoint
	Timeout time.Duration
	Client  *http.Client
}

// Name implements Provider.
func (p *Photon) Name() string { return ProviderPhoton }

// photonResponse is the subset of the Photon GeoJSON payload we consume.
// Photon returns a FeatureCollection — we take the first feature only. City
// may be missing for rural points; we fall back to county. country_code is
// lowercase in the API, we uppercase it to match the Nominatim convention.
type photonResponse struct {
	Features []struct {
		Properties struct {
			City        string `json:"city"`
			County      string `json:"county"`
			State       string `json:"state"`
			Country     string `json:"country"`
			CountryCode string `json:"countrycode"` // note: single-word, lowercase
		} `json:"properties"`
	} `json:"features"`
}

// Reverse implements Provider.
func (p *Photon) Reverse(ctx context.Context, lat, lng float64, lang string) (*Result, error) {
	// Validate BEFORE waiting on the rate limiter so a misconfigured base
	// URL doesn't burn a bucket token. Same rationale as nominatim.go.
	base := p.BaseURL
	if base == "" {
		base = defaultPhotonURL
	}
	// SSRF guard: see nominatim.go — same rule applies.
	if err := ValidateBaseURL(p.BaseURL); err != nil {
		return nil, fmt.Errorf("photon base url: %w", err)
	}

	lat = RoundCoord(lat)
	lng = RoundCoord(lng)
	if lang == "" {
		// Use the same Accept-Language-style fallback chain as Nominatim.
		// Photon only honours the FIRST language token in the list but
		// accepts the multi-language syntax without error, so we can keep
		// the two providers aligned on a single default.
		lang = "de,en"
	}

	if err := waitPhoton(ctx); err != nil {
		return nil, fmt.Errorf("photon rate-limit wait: %w", err)
	}

	extra := url.Values{}
	extra.Set("lat", fmt.Sprintf("%.3f", lat))
	extra.Set("lon", fmt.Sprintf("%.3f", lng))
	extra.Set("lang", lang)
	// radius=1 km: sport-activity start/end points should match tightly —
	// we only want the actual city/settlement the activity is in, not a
	// metro area within 5 km (Photon's default 5 would pull the nearest
	// larger town across a valley, producing misleading "near X" results).
	extra.Set("radius", "1")

	fullURL, err := buildProviderURL(base, extra)
	if err != nil {
		return nil, fmt.Errorf("photon build url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("photon build request: %w", err)
	}
	req.Header.Set("User-Agent", CurrentUserAgent())
	req.Header.Set("Accept", "application/json")

	client := p.Client
	if client == nil {
		timeout := p.Timeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("photon request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("photon status %d", resp.StatusCode)
	}

	var body photonResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("photon decode: %w", err)
	}
	if len(body.Features) == 0 {
		return nil, nil
	}
	props := body.Features[0].Properties

	r := &Result{
		City:        firstNonEmpty(props.City, props.County),
		State:       props.State,
		Country:     props.Country,
		CountryCode: strings.ToUpper(props.CountryCode),
	}
	if r.Empty() {
		return nil, nil
	}
	return r, nil
}

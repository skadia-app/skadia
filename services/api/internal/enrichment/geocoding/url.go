package geocoding

import (
	"fmt"
	"net/url"
)

// buildProviderURL composes a provider request URL by merging `extra` query
// parameters into whatever query string may already be present on `base`.
//
// Why this helper exists: the previous implementation concatenated base +
// "?" + extra.Encode() with a naive "does base contain '?'" check. That
// approach breaks on two real-world shapes:
//
//  1. A user-configured base URL that carries a `#fragment` (e.g. an admin
//     pastes the browser URL bar for a self-hosted Nominatim instance):
//     naively appending "?lat=…" yields ".../reverse#frag?lat=…" — the
//     fragment swallows the query, the server sees no parameters, and
//     reverse lookup silently returns nothing.
//  2. A base URL with existing query parameters followed by a fragment
//     (".../reverse?api_key=X#frag"): naive concat would produce
//     ".../reverse?api_key=X#frag&lat=…", again keeping the fragment before
//     the new query.
//
// The safe approach is url.Parse → merge Query() → re-encode RawQuery →
// keep any Fragment in its proper position via u.String(). url.Parse is
// lenient enough that we do NOT double-validate here — ValidateBaseURL is
// already called upstream on admin-configured inputs.
func buildProviderURL(base string, extra url.Values) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base %q: %w", base, err)
	}
	q := u.Query()
	for k, vs := range extra {
		// extra always sets single values in this package; replace wins.
		q[k] = vs
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

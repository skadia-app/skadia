package geocoding

import (
	"context"

	"golang.org/x/time/rate"
)

// nominatimLimiter is the global (process-wide, NOT per-user) rate limiter for
// Nominatim requests. The public Nominatim API at nominatim.openstreetmap.org
// enforces a strict "max 1 req/s" policy; exceeding it risks the whole Skadia
// instance being blocked, which would affect ALL users. A single global bucket
// with burst=1 is the minimal safe configuration.
var nominatimLimiter = rate.NewLimiter(rate.Limit(1), 1)

// photonLimiter is the global rate limiter for Photon (komoot.io) requests.
// Photon has no documented absolute limit, but the operator publishes a
// fair-use expectation; unthrottled backfill jobs that go wide on cache
// misses can still draw operator attention. A 5 req/s bucket keeps us well
// inside fair-use while being generous enough that interactive
// city-enrichment on upload feels instant (cache miss → single request).
// Burst=5 lets a short burst soak into the bucket without stalling.
var photonLimiter = rate.NewLimiter(rate.Limit(5), 5)

// waitNominatim blocks until the global Nominatim rate limiter allows another
// request or the context is cancelled. It returns the context's error on
// cancellation, nil otherwise.
//
// Caller contract: the context's deadline should be checked BEFORE calling
// this function if the caller does not want to spend a token on a request
// that will be aborted immediately. rate.Limiter.Wait consumes a reservation
// even if the context is already cancelled, which (since our buckets are
// size 1 for Nominatim) would delay the next legitimate request by up to 1s.
func waitNominatim(ctx context.Context) error {
	// Fast-path cancellation check: avoid consuming a token when the caller
	// has already given up. Cheap select; mirrors the stdlib idiom.
	if err := ctx.Err(); err != nil {
		return err
	}
	return nominatimLimiter.Wait(ctx)
}

// waitPhoton blocks until the global Photon rate limiter allows another
// request or the context is cancelled. See waitNominatim for the rationale
// behind the pre-check.
func waitPhoton(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return photonLimiter.Wait(ctx)
}

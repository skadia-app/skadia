// errors.go — Typed routing errors.
package routing

import "errors"

// ErrNoRoute indicates that no route could be found between the given endpoints.
// Used by findRouteUnified when the frontier is exhausted without reaching the destination,
// and by snapToEdge when no suitable edge is found within range.
var ErrNoRoute = errors.New("no route found")

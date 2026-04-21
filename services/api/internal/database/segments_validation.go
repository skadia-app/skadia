package database

import (
	"regexp"
	"strings"
)

var (
	sacScaleRe = regexp.MustCompile(`^T[1-6]$`)
	mtbScaleRe = regexp.MustCompile(`^S[0-5]$`)
)

// validSurfaces spiegelt frontend/src/components/segment-editor/constants.ts
// SURFACE_DROPDOWN_OPTIONS — minus "Wechselnd" (editor-only, nicht filterbar)
var validSurfaces = map[string]bool{
	"Asphalt":         true,
	"Pflaster":        true,
	"Schotter (fein)": true,
	"Schotter (grob)": true,
	"Naturweg":        true,
	"Sand":            true,
	"Gras":            true,
	"Fels":            true,
	"Schnee":          true,
	"Sportbelag":      true,
}

func IsValidSacList(s string) bool {
	if s == "" {
		return true
	}
	for _, v := range strings.Split(s, ",") {
		if !sacScaleRe.MatchString(v) {
			return false
		}
	}
	return true
}

func IsValidMtbList(s string) bool {
	if s == "" {
		return true
	}
	for _, v := range strings.Split(s, ",") {
		if !mtbScaleRe.MatchString(v) {
			return false
		}
	}
	return true
}

func IsValidSurfaceList(s string) bool {
	if s == "" {
		return true
	}
	for _, v := range strings.Split(s, ",") {
		if !validSurfaces[v] {
			return false
		}
	}
	return true
}

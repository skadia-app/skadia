package enrichment

type bbox struct {
	minLat, minLon, maxLat, maxLon float64
}

func streamBBox(stream []EnrichmentStreamPoint) bbox {
	b := bbox{minLat: 90, minLon: 180, maxLat: -90, maxLon: -180}
	for _, sp := range stream {
		if sp.Lat == nil || sp.Lng == nil {
			continue
		}
		b.minLat = min(b.minLat, *sp.Lat)
		b.maxLat = max(b.maxLat, *sp.Lat)
		b.minLon = min(b.minLon, *sp.Lng)
		b.maxLon = max(b.maxLon, *sp.Lng)
	}
	return b
}

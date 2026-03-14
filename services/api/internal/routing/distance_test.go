package routing

import (
	"math"
	"testing"
)

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6_371_000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func TestEquirectDist_DACH_Accuracy(t *testing.T) {
	cases := []struct {
		name            string
		lat1, lon1      float64
		lat2, lon2      float64
		maxErrorPercent float64
	}{
		{"Innsbruck-Brenner_short", 47.2692, 11.3933, 47.0000, 11.5000, 0.5},
		{"Munich-Salzburg_100km", 48.1351, 11.5820, 47.8095, 13.0550, 0.5},
		{"Zurich-Vienna_600km", 47.3769, 8.5417, 48.2082, 16.3738, 1.0},
		{"Hamburg-Munich_north-south", 53.5511, 9.9937, 48.1351, 11.5820, 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lat1i := int32(tc.lat1 * 1e7)
			lon1i := int32(tc.lon1 * 1e7)
			lat2i := int32(tc.lat2 * 1e7)
			lon2i := int32(tc.lon2 * 1e7)

			got := equirectDist(lat1i, lon1i, lat2i, lon2i)
			want := haversine(tc.lat1, tc.lon1, tc.lat2, tc.lon2)
			errPct := math.Abs(float64(got)-want) / want * 100
			if errPct > tc.maxErrorPercent {
				t.Errorf("error %.2f%% > %.2f%% (got %.1fm, want %.1fm)",
					errPct, tc.maxErrorPercent, got, want)
			}
		})
	}
}

func TestEquirectDist_ZeroDistance(t *testing.T) {
	d := equirectDist(471000000, 113000000, 471000000, 113000000)
	if d > 1.0 {
		t.Errorf("same point should be ~0, got %f", d)
	}
}

func BenchmarkEquirectDist(b *testing.B) {
	for i := 0; i < b.N; i++ {
		equirectDist(472692000, 113933000, 470000000, 115000000)
	}
}

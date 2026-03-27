package coverage

import "image/color"

type PaletteFunc func(uint8) color.NRGBA

var Palettes = map[string]PaletteFunc{
	"forest":          paletteForest,
	"water":           paletteWater,
	"river_proximity": paletteRiverProximity,
	"landuse":         paletteLanduse,
	"protected":       paletteProtected,
	"urbanization":    paletteUrbanization,
	"traffic":         paletteTraffic,
	"noise":           paletteNoise,
	"quiet":           paletteQuiet,
}

var VisLayerNames = []string{
	"forest", "water", "river_proximity", "landuse",
	"protected", "urbanization", "traffic", "noise", "quiet",
}

func paletteForest(v uint8) color.NRGBA {
	if v == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{R: 0x16, G: 0x65, B: 0x34, A: 180}
}

func paletteWater(v uint8) color.NRGBA {
	if v == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{R: 0x1e, G: 0x40, B: 0xaf, A: 180}
}

func paletteRiverProximity(v uint8) color.NRGBA {
	if v == 0 {
		return color.NRGBA{}
	}
	a := uint8(min(180, int(v)*180/255))
	return color.NRGBA{R: 0x1e, G: 0x40, B: 0xaf, A: a}
}

func paletteLanduse(v uint8) color.NRGBA {
	switch v {
	case 0:
		return color.NRGBA{}
	case 1:
		return color.NRGBA{R: 0x16, G: 0x65, B: 0x34, A: 100}
	case 2:
		return color.NRGBA{R: 0xd4, G: 0xa5, B: 0x74, A: 120}
	case 3:
		return color.NRGBA{R: 0xf9, G: 0xa8, B: 0xd4, A: 120}
	case 4:
		return color.NRGBA{R: 0x9c, G: 0xa3, B: 0xaf, A: 120}
	case 5:
		return color.NRGBA{R: 0xfb, G: 0xbf, B: 0x24, A: 120}
	case 6:
		return color.NRGBA{R: 0x78, G: 0x71, B: 0x6c, A: 120}
	case 7:
		return color.NRGBA{R: 0x6b, G: 0x72, B: 0x80, A: 120}
	case 8:
		return color.NRGBA{R: 0x92, G: 0x40, B: 0x0e, A: 120}
	default:
		return color.NRGBA{}
	}
}

func paletteProtected(v uint8) color.NRGBA {
	switch v {
	case 0:
		return color.NRGBA{}
	case 1:
		return color.NRGBA{R: 0x15, G: 0x80, B: 0x3d, A: 60}
	case 2:
		return color.NRGBA{R: 0x15, G: 0x80, B: 0x3d, A: 100}
	case 3:
		return color.NRGBA{R: 0x15, G: 0x80, B: 0x3d, A: 150}
	default:
		return color.NRGBA{R: 0x15, G: 0x80, B: 0x3d, A: 150}
	}
}

func paletteUrbanization(v uint8) color.NRGBA {
	if v == 0 {
		return color.NRGBA{}
	}
	t := float64(v) / 255.0
	r := uint8(0xa3 + t*(0xf9-0xa3))
	g := uint8(0xa3 + t*(0x73-0xa3))
	b := uint8(0xa3 + t*(0x16-0xa3))
	a := uint8(40 + t*160)
	return color.NRGBA{R: r, G: g, B: b, A: a}
}

var trafficColors = [7]color.NRGBA{
	{},
	{R: 0x22, G: 0xc5, B: 0x5e, A: 140},
	{R: 0x84, G: 0xcc, B: 0x16, A: 140},
	{R: 0xea, G: 0xb3, B: 0x08, A: 140},
	{R: 0xf9, G: 0x73, B: 0x16, A: 150},
	{R: 0xef, G: 0x44, B: 0x44, A: 160},
	{R: 0x99, G: 0x1b, B: 0x1b, A: 180},
}

func paletteTraffic(v uint8) color.NRGBA {
	if int(v) >= len(trafficColors) {
		return trafficColors[6]
	}
	return trafficColors[v]
}

func paletteNoise(v uint8) color.NRGBA {
	if v == 0 {
		return color.NRGBA{}
	}
	t := float64(v) / 255.0
	var r, g, b uint8
	if t < 0.5 {
		// Green (#22c55e) → Yellow (#eab308)
		s := t * 2
		r = uint8(float64(0x22) + s*float64(0xea-0x22))
		g = uint8(float64(0xc5) + s*float64(0xb3-0xc5))
		b = uint8(float64(0x5e) + s*float64(0x08-0x5e))
	} else {
		// Yellow (#eab308) → Red (#ef4444)
		s := (t - 0.5) * 2
		r = uint8(float64(0xea) + s*float64(0xef-0xea))
		g = uint8(float64(0xb3) - s*float64(0xb3-0x44))
		b = uint8(float64(0x08) + s*float64(0x44-0x08))
	}
	a := uint8(min(255, 60+int(t*195)))
	return color.NRGBA{R: r, G: g, B: b, A: a}
}

// paletteQuiet renders the same gradient as paletteNoise: green (low v = quiet) → red (high v = loud).
// The quiet layer stores noise levels identically to the noise layer; the "quiet" label reflects
// the user-facing framing (good quiet zones = green, bad loud zones = red).
func paletteQuiet(v uint8) color.NRGBA {
	return paletteNoise(v)
}

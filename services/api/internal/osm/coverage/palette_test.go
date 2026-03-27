package coverage

import "testing"

func TestForestPalette(t *testing.T) {
	p := Palettes["forest"]
	c0 := p(0)
	if c0.A != 0 {
		t.Errorf("forest(0) alpha = %d, want 0", c0.A)
	}
	c1 := p(1)
	if c1.A == 0 {
		t.Error("forest(1) should not be transparent")
	}
	if c1.G < c1.R {
		t.Error("forest should be green-ish")
	}
}

func TestNoisePalette(t *testing.T) {
	p := Palettes["noise"]
	if p(0).A != 0 {
		t.Error("noise(0) should be transparent")
	}
	c := p(255)
	if c.R < 200 || c.A < 200 {
		t.Errorf("noise(255) = %v, expected red+opaque", c)
	}
}

func TestQuietPalette(t *testing.T) {
	p := Palettes["quiet"]
	c0 := p(0)
	if c0.A != 0 {
		t.Errorf("quiet(0) should be transparent, got A=%d", c0.A)
	}
	c1 := p(1)
	if c1.A == 0 {
		t.Error("quiet(1) should not be transparent")
	}
	c255 := p(255)
	if c255.R < 200 || c255.A < 200 {
		t.Errorf("quiet(255) should be reddish+opaque, got R=%d A=%d", c255.R, c255.A)
	}
}

func TestTrafficPalette(t *testing.T) {
	p := Palettes["traffic"]
	if p(0).A != 0 {
		t.Error("traffic(0) should be transparent")
	}
	c := p(6)
	if c.A == 0 {
		t.Error("traffic(6) should not be transparent")
	}
}

func TestAllVisLayerPalettes(t *testing.T) {
	required := []string{
		"forest", "water", "river_proximity", "landuse",
		"protected", "urbanization", "traffic", "noise", "quiet",
	}
	for _, name := range required {
		if _, ok := Palettes[name]; !ok {
			t.Errorf("missing palette for %q", name)
		}
	}
}

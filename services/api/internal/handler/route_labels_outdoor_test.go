package handler

import "testing"

func TestTvDisplayName(t *testing.T) {
	if tvDisplayName(4) != "schlecht sichtbar" {
		t.Errorf("tv(4) want 'schlecht sichtbar'")
	}
	if tvDisplayName(0) != "" || tvDisplayName(99) != "" {
		t.Errorf("tv bounds")
	}
}

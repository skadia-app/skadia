package gpximport

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseGPXLiftStreamsDeriveSpeedAndPreserveGapHazard(t *testing.T) {
	data, err := ParseGPX(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="synthetic">
  <trk>
    <name>synthetic lift loader</name>
    <type>hiking</type>
    <trkseg>
      <trkpt lat="47.000000" lon="12.000000"><ele>1000</ele><time>1970-01-01T00:00:00Z</time></trkpt>
      <trkpt lat="47.000100" lon="12.000000"><ele>1001</ele><time>1970-01-01T00:00:10Z</time></trkpt>
      <trkpt lat="47.000300" lon="12.000000"><ele>1002</ele><time>1970-01-01T00:03:30Z</time></trkpt>
    </trkseg>
  </trk>
</gpx>`), "synthetic.gpx")
	if err != nil {
		t.Fatalf("ParseGPX returned error: %v", err)
	}
	if len(data.Streams) != 3 {
		t.Fatalf("stream count = %d, want 3", len(data.Streams))
	}
	if data.Streams[0].SpeedMS != nil {
		t.Fatalf("first sample SpeedMS = %v, want nil", *data.Streams[0].SpeedMS)
	}
	if data.Streams[1].SpeedMS == nil || *data.Streams[1].SpeedMS <= 0 {
		t.Fatalf("second sample SpeedMS = %v, want positive derived speed", data.Streams[1].SpeedMS)
	}
	gap := data.Streams[2].Time.Sub(data.Streams[1].Time)
	if gap != 200*time.Second {
		t.Fatalf("stream gap = %s, want 200s", gap)
	}
	if data.Streams[2].SpeedMS == nil {
		t.Fatal("200s gap sample SpeedMS = nil, want parser's current <=300s derived-speed behavior")
	}
}

func TestParseGPXLiftStreamsSkipInvalidTimeAndKeepOptionalElevation(t *testing.T) {
	data, err := ParseGPX(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="synthetic">
  <trk>
    <name>synthetic lift loader</name>
    <type>hiking</type>
    <trkseg>
      <trkpt lat="47.000000" lon="12.000000"><ele>1000</ele><time>1970-01-01T00:00:00Z</time></trkpt>
      <trkpt lat="47.000100" lon="12.000000"><ele>9999</ele><time>not-a-time</time></trkpt>
      <trkpt lat="47.000200" lon="12.000000"><time>1970-01-01T00:00:10Z</time></trkpt>
    </trkseg>
  </trk>
</gpx>`), "synthetic.gpx")
	if err != nil {
		t.Fatalf("ParseGPX returned error: %v", err)
	}
	if len(data.Streams) != 2 {
		t.Fatalf("stream count = %d, want invalid-time sample skipped", len(data.Streams))
	}
	if data.Streams[0].AltitudeM == nil || math.Abs(*data.Streams[0].AltitudeM-1000) > 0.001 {
		t.Fatalf("first sample altitude = %v, want 1000m", data.Streams[0].AltitudeM)
	}
	if data.Streams[1].AltitudeM != nil {
		t.Fatalf("second sample altitude = %v, want nil for missing <ele>", *data.Streams[1].AltitudeM)
	}
	if data.Streams[1].DistanceM == nil || *data.Streams[1].DistanceM <= 0 {
		t.Fatalf("second sample DistanceM = %v, want distance from previous valid sample", data.Streams[1].DistanceM)
	}
	if data.Streams[1].SpeedMS == nil || *data.Streams[1].SpeedMS <= 0 {
		t.Fatalf("second sample SpeedMS = %v, want speed from previous valid sample", data.Streams[1].SpeedMS)
	}
}

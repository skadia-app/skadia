package tcximport

import (
	"strings"
	"testing"
)

func TestParseTCX_FloatInIntegerFields(t *testing.T) {
	tcxData := `<?xml version="1.0"?>
<TrainingCenterDatabase xmlns="http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2">
  <Activities>
    <Activity Sport="Running">
      <Id>2025-01-01T10:00:00Z</Id>
      <Creator><Name>Samsung Health</Name></Creator>
      <Lap StartTime="2025-01-01T10:00:00Z">
        <TotalTimeSeconds>1800.0</TotalTimeSeconds>
        <DistanceMeters>5000.0</DistanceMeters>
        <Calories>711.0</Calories>
        <AverageHeartRateBpm><Value>163.0</Value></AverageHeartRateBpm>
        <MaximumHeartRateBpm><Value>186.0</Value></MaximumHeartRateBpm>
        <Cadence>82.0</Cadence>
        <Track>
          <Trackpoint>
            <Time>2025-01-01T10:00:00Z</Time>
            <HeartRateBpm><Value>143.56</Value></HeartRateBpm>
            <Cadence>80.5</Cadence>
          </Trackpoint>
          <Trackpoint>
            <Time>2025-01-01T10:00:01Z</Time>
            <HeartRateBpm><Value>150.0</Value></HeartRateBpm>
          </Trackpoint>
        </Track>
      </Lap>
    </Activity>
  </Activities>
</TrainingCenterDatabase>`

	data, err := ParseTCX(strings.NewReader(tcxData), "samsung_test.tcx")
	if err != nil {
		t.Fatalf("ParseTCX failed: %v", err)
	}

	if data.MaxHeartRate != 186 {
		t.Errorf("MaxHeartRate = %d, want 186", data.MaxHeartRate)
	}
	if len(data.Streams) != 2 {
		t.Fatalf("Streams = %d, want 2", len(data.Streams))
	}
	if data.Streams[0].HeartRate == nil || *data.Streams[0].HeartRate != 144 {
		t.Errorf("Stream[0].HeartRate = %v, want 144 (rounded from 143.56)", data.Streams[0].HeartRate)
	}
}

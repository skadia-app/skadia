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

func TestParseTCX_LapsExtracted(t *testing.T) {
	tcxData := `<?xml version="1.0"?>
<TrainingCenterDatabase xmlns="http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2">
  <Activities>
    <Activity Sport="Running">
      <Id>2025-01-01T10:00:00Z</Id>
      <Creator><Name>Polar Flow</Name></Creator>
      <Lap StartTime="2025-01-01T10:00:00Z">
        <TotalTimeSeconds>600</TotalTimeSeconds>
        <DistanceMeters>2000</DistanceMeters>
        <Calories>120</Calories>
        <AverageHeartRateBpm><Value>150</Value></AverageHeartRateBpm>
        <MaximumHeartRateBpm><Value>165</Value></MaximumHeartRateBpm>
        <Track>
          <Trackpoint><Time>2025-01-01T10:00:00Z</Time></Trackpoint>
          <Trackpoint><Time>2025-01-01T10:10:00Z</Time></Trackpoint>
        </Track>
      </Lap>
      <Lap StartTime="2025-01-01T10:10:00Z">
        <TotalTimeSeconds>500</TotalTimeSeconds>
        <DistanceMeters>1800</DistanceMeters>
        <Calories>100</Calories>
        <AverageHeartRateBpm><Value>155</Value></AverageHeartRateBpm>
        <MaximumHeartRateBpm><Value>170</Value></MaximumHeartRateBpm>
        <Track>
          <Trackpoint><Time>2025-01-01T10:10:00Z</Time></Trackpoint>
          <Trackpoint><Time>2025-01-01T10:18:20Z</Time></Trackpoint>
        </Track>
      </Lap>
    </Activity>
  </Activities>
</TrainingCenterDatabase>`

	data, err := ParseTCX(strings.NewReader(tcxData), "polar_laps.tcx")
	if err != nil {
		t.Fatalf("ParseTCX failed: %v", err)
	}

	if len(data.Laps) != 2 {
		t.Fatalf("Laps = %d, want 2", len(data.Laps))
	}
	if data.Laps[0].TotalElapsedTimeS != 600 {
		t.Errorf("Lap[0].TotalElapsedTimeS = %v, want 600", data.Laps[0].TotalElapsedTimeS)
	}
	if data.Laps[1].TotalDistanceM != 1800 {
		t.Errorf("Lap[1].TotalDistanceM = %v, want 1800", data.Laps[1].TotalDistanceM)
	}
	if data.Laps[0].AvgHeartRate != 150 {
		t.Errorf("Lap[0].AvgHeartRate = %d, want 150", data.Laps[0].AvgHeartRate)
	}
	if data.Laps[0].MaxHeartRate != 165 {
		t.Errorf("Lap[0].MaxHeartRate = %d, want 165", data.Laps[0].MaxHeartRate)
	}
	if data.Laps[0].TotalCalories != 120 {
		t.Errorf("Lap[0].TotalCalories = %d, want 120", data.Laps[0].TotalCalories)
	}
}

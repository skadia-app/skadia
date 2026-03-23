package fitimport

import (
	"math"
	"testing"
	"time"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
)

func TestExtractHrData_AllFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	hrs := []*mesgdef.Hr{
		{
			Timestamp:           now,
			FractionalTimestamp: 16384,                             // /32768 = 0.5
			FilteredBpm:        []uint8{120, 121},                 // []int16{120, 121}
			EventTimestamp:      []uint32{1024000, 1025024},       // /1024 = [1000.0, 1001.0]
		},
	}

	result := ExtractHrData(hrs)
	if len(result) != 1 {
		t.Fatalf("expected 1 hr record, got %d", len(result))
	}

	rec := result[0]

	// FractionalTimestamp: 16384 / 32768.0 = 0.5
	if rec.FractionalTimestamp == nil || *rec.FractionalTimestamp != 0.5 {
		t.Errorf("FractionalTimestamp: got %v, want 0.5", rec.FractionalTimestamp)
	}

	// FilteredBpm
	if len(rec.FilteredBpm) != 2 {
		t.Fatalf("FilteredBpm: got %d elements, want 2", len(rec.FilteredBpm))
	}
	if rec.FilteredBpm[0] != 120 || rec.FilteredBpm[1] != 121 {
		t.Errorf("FilteredBpm: got %v, want [120 121]", rec.FilteredBpm)
	}

	// EventTimestamp: [1024000/1024, 1025024/1024] = [1000.0, 1001.0]
	if len(rec.EventTimestamp) != 2 {
		t.Fatalf("EventTimestamp: got %d elements, want 2", len(rec.EventTimestamp))
	}
	if math.Abs(rec.EventTimestamp[0]-1000.0) > 0.001 {
		t.Errorf("EventTimestamp[0]: got %f, want 1000.0", rec.EventTimestamp[0])
	}
	if math.Abs(rec.EventTimestamp[1]-1001.0) > 0.001 {
		t.Errorf("EventTimestamp[1]: got %f, want 1001.0", rec.EventTimestamp[1])
	}
}

func TestExtractHrData_InvalidFieldsSkipped(t *testing.T) {
	hrs := []*mesgdef.Hr{
		{
			Timestamp:           time.Now().UTC(),
			FractionalTimestamp: basetype.Uint16Invalid, // skip
			FilteredBpm:        nil,                     // skip
			EventTimestamp:      nil,                     // skip
		},
	}

	result := ExtractHrData(hrs)
	if len(result) != 1 {
		t.Fatalf("expected 1 hr record, got %d", len(result))
	}

	rec := result[0]
	if rec.FractionalTimestamp != nil {
		t.Errorf("FractionalTimestamp: expected nil, got %v", *rec.FractionalTimestamp)
	}
	if rec.FilteredBpm != nil {
		t.Errorf("FilteredBpm: expected nil, got %v", rec.FilteredBpm)
	}
	if rec.EventTimestamp != nil {
		t.Errorf("EventTimestamp: expected nil, got %v", rec.EventTimestamp)
	}
}

func TestExtractHrData_EventTimestampInvalidFiltered(t *testing.T) {
	hrs := []*mesgdef.Hr{
		{
			Timestamp:      time.Now().UTC(),
			FractionalTimestamp: basetype.Uint16Invalid,
			EventTimestamp: []uint32{1024000, basetype.Uint32Invalid, 2048000},
		},
	}

	result := ExtractHrData(hrs)
	rec := result[0]

	// Invalid entries filtered out, so only 2 valid entries
	if len(rec.EventTimestamp) != 2 {
		t.Fatalf("EventTimestamp: got %d elements, want 2 (invalid filtered)", len(rec.EventTimestamp))
	}
	if math.Abs(rec.EventTimestamp[0]-1000.0) > 0.001 {
		t.Errorf("EventTimestamp[0]: got %f, want 1000.0", rec.EventTimestamp[0])
	}
	if math.Abs(rec.EventTimestamp[1]-2000.0) > 0.001 {
		t.Errorf("EventTimestamp[1]: got %f, want 2000.0", rec.EventTimestamp[1])
	}
}

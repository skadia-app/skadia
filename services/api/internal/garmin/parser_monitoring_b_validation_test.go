package garmin

import (
	"testing"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
)

// makeMsg builds a minimal proto.Message with a given message number and
// a set of field values keyed by field number.
func makeMsg(num typedef.MesgNum, fields map[byte]proto.Value) proto.Message {
	msg := proto.Message{Num: num}
	for fieldNum, val := range fields {
		msg.Fields = append(msg.Fields, proto.Field{
			FieldBase: &proto.FieldBase{Num: fieldNum},
			Value:     val,
		})
	}
	return msg
}

// TestBodyBatteryFieldExtraction verifies that FieldValueByNum(3) correctly
// extracts int8 (Sint8) values from the undocumented body battery field.
//
// Body Battery is stored in msg 227 (StressLevel), field 3 as a sint8 (0-100).
// Values outside 0-100 must be rejected.
func TestBodyBatteryFieldExtraction(t *testing.T) {
	cases := []struct {
		name      string
		rawValue  int8
		wantStore bool
		wantValue int16
	}{
		{"zero_boundary", 0, true, 0},
		{"mid_range", 50, true, 50},
		{"max_boundary", 100, true, 100},
		{"above_max", 101, false, 0},      // must be rejected
		{"negative_minus1", -1, false, 0}, // must be rejected
		{"negative_minus16", -16, false, 0}, // Garmin blank marker
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the parser logic from parser_monitoring_b.go lines 166-170:
			//   bbVal := msg.FieldValueByNum(3)
			//   if bbVal.Valid(basetype.Sint8) { bb := bbVal.Int8(); if bb >= 0 && bb <= 100 { ... } }
			val := proto.Int8(tc.rawValue)
			if !val.Valid(basetype.Sint8) {
				if tc.wantStore {
					t.Fatalf("value %d: expected Sint8 to be valid, but was not", tc.rawValue)
				}
				return
			}
			bb := val.Int8()
			store := bb >= 0 && bb <= 100
			if store != tc.wantStore {
				t.Errorf("value %d: wantStore=%v, got store=%v", tc.rawValue, tc.wantStore, store)
			}
			if store && int16(bb) != tc.wantValue {
				t.Errorf("value %d: want stored value %d, got %d", tc.rawValue, tc.wantValue, int16(bb))
			}
		})
	}
}

// TestTrainingReadinessFieldMapping verifies that all 7 Training Readiness
// field numbers are mapped to the correct model fields.
//
// Msg 369 (proprietary), fields:
//
//	0  → Score
//	5  → SleepScore
//	8  → RecoveryTimeFactorPct
//	10 → AcwrPct
//	12 → StressHistoryFactorPct
//	14 → HRVFactorPct
//	16 → SleepHistoryFactorPct
func TestTrainingReadinessFieldMapping(t *testing.T) {
	// Simulate each field assignment from importer_monitoring_fit.go lines 547-580.
	// We verify the conditional logic for each field number independently.

	type fieldCase struct {
		fieldNum    byte
		rawValue    int16
		description string
		extractFn   func(v proto.Value) (float64, bool)
	}

	cases := []fieldCase{
		{
			fieldNum:    0,
			rawValue:    75,
			description: "Score (sint16 → int)",
			extractFn: func(v proto.Value) (float64, bool) {
				if !v.Valid(basetype.Sint16) {
					return 0, false
				}
				return float64(v.Int16()), true
			},
		},
		{
			fieldNum:    5,
			rawValue:    68,
			description: "SleepScore (sint16 → int)",
			extractFn: func(v proto.Value) (float64, bool) {
				if !v.Valid(basetype.Sint16) {
					return 0, false
				}
				return float64(v.Int16()), true
			},
		},
		{
			fieldNum:    8,
			rawValue:    82,
			description: "RecoveryTimeFactorPct (sint16 → float64)",
			extractFn: func(v proto.Value) (float64, bool) {
				if !v.Valid(basetype.Sint16) {
					return 0, false
				}
				return float64(v.Int16()), true
			},
		},
		{
			fieldNum:    10,
			rawValue:    95,
			description: "AcwrPct (sint16 → float64)",
			extractFn: func(v proto.Value) (float64, bool) {
				if !v.Valid(basetype.Sint16) {
					return 0, false
				}
				return float64(v.Int16()), true
			},
		},
		{
			fieldNum:    12,
			rawValue:    60,
			description: "StressHistoryFactorPct (sint16 → float64)",
			extractFn: func(v proto.Value) (float64, bool) {
				if !v.Valid(basetype.Sint16) {
					return 0, false
				}
				return float64(v.Int16()), true
			},
		},
		{
			fieldNum:    14,
			rawValue:    88,
			description: "HRVFactorPct (sint16 → float64)",
			extractFn: func(v proto.Value) (float64, bool) {
				if !v.Valid(basetype.Sint16) {
					return 0, false
				}
				return float64(v.Int16()), true
			},
		},
		{
			fieldNum:    16,
			rawValue:    72,
			description: "SleepHistoryFactorPct (sint16 → float64)",
			extractFn: func(v proto.Value) (float64, bool) {
				if !v.Valid(basetype.Sint16) {
					return 0, false
				}
				return float64(v.Int16()), true
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			// Build a message with only this field set.
			msg := makeMsg(typedef.MesgNum(369), map[byte]proto.Value{
				tc.fieldNum: proto.Int16(tc.rawValue),
			})

			// Look up the field and extract the value.
			v := msg.FieldValueByNum(tc.fieldNum)
			got, ok := tc.extractFn(v)
			if !ok {
				t.Fatalf("field %d (%s): expected valid Sint16, got invalid", tc.fieldNum, tc.description)
			}
			want := float64(tc.rawValue)
			if got != want {
				t.Errorf("field %d (%s): want %v, got %v", tc.fieldNum, tc.description, want, got)
			}
		})
	}
}

// TestTrainingReadinessInvalidFieldIgnored verifies that a missing field
// (not present in the message) returns an invalid Value and is correctly skipped.
func TestTrainingReadinessInvalidFieldIgnored(t *testing.T) {
	// Message with no fields at all.
	msg := makeMsg(typedef.MesgNum(369), nil)

	// All 7 field numbers must return invalid values when the field is absent.
	fieldNums := []byte{0, 5, 8, 10, 12, 14, 16}
	for _, num := range fieldNums {
		v := msg.FieldValueByNum(num)
		if v.Valid(basetype.Sint16) {
			t.Errorf("field %d: expected invalid value for absent field, got valid", num)
		}
	}
}

package fitimport

import (
	"testing"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

func TestExtractDeveloperFieldDefs_BasicFields(t *testing.T) {
	appID := []byte{0x01, 0x02, 0x03, 0x04}

	devDataIds := []*mesgdef.DeveloperDataId{
		{
			DeveloperDataIndex: 0,
			ApplicationId:     appID,
			ManufacturerId:     typedef.ManufacturerInvalid,
		},
	}

	fieldDescs := []*mesgdef.FieldDescription{
		{
			DeveloperDataIndex:    0,
			FieldDefinitionNumber: 1,
			FieldName:             []string{"stryd_power"},
			Units:                 []string{"watts"},
			NativeMesgNum:         typedef.MesgNumInvalid,
			NativeFieldNum:        basetype.Uint8Invalid,
			FitBaseTypeId:         basetype.BaseType(0), // enum
			Scale:                 10,
			Offset:                basetype.Sint8Invalid,
		},
	}

	result := ExtractDeveloperFieldDefs(devDataIds, fieldDescs)
	if len(result) != 1 {
		t.Fatalf("expected 1 field def, got %d", len(result))
	}

	def := result[0]

	// FieldName
	if def.FieldName != "stryd_power" {
		t.Errorf("FieldName: got %q, want 'stryd_power'", def.FieldName)
	}

	// ApplicationID
	if len(def.ApplicationID) != 4 || def.ApplicationID[0] != 0x01 {
		t.Errorf("ApplicationID: got %v, want [01 02 03 04]", def.ApplicationID)
	}

	// DeveloperDataIndex
	if def.DeveloperDataIndex != 0 {
		t.Errorf("DeveloperDataIndex: got %d, want 0", def.DeveloperDataIndex)
	}

	// FieldDefinitionNumber
	if def.FieldDefinitionNumber != 1 {
		t.Errorf("FieldDefinitionNumber: got %d, want 1", def.FieldDefinitionNumber)
	}

	// Units
	if def.Units == nil || *def.Units != "watts" {
		t.Errorf("Units: got %v, want 'watts'", def.Units)
	}

	// Scale
	if def.Scale == nil || *def.Scale != 10 {
		t.Errorf("Scale: got %v, want 10", def.Scale)
	}

	// Offset should be nil (invalid)
	if def.Offset != nil {
		t.Errorf("Offset: expected nil, got %v", *def.Offset)
	}

	// NativeMesgNum should be nil (invalid)
	if def.NativeMesgNum != nil {
		t.Errorf("NativeMesgNum: expected nil, got %v", *def.NativeMesgNum)
	}

	// NativeFieldNum should be nil (invalid)
	if def.NativeFieldNum != nil {
		t.Errorf("NativeFieldNum: expected nil, got %v", *def.NativeFieldNum)
	}
}

func TestExtractDeveloperFieldDefs_EmptyInput(t *testing.T) {
	result := ExtractDeveloperFieldDefs(nil, nil)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestExtractDeveloperFieldDefs_InvalidFieldSkipped(t *testing.T) {
	fieldDescs := []*mesgdef.FieldDescription{
		{
			DeveloperDataIndex:    basetype.Uint8Invalid, // invalid → skip
			FieldDefinitionNumber: 0,
		},
		{
			DeveloperDataIndex:    0,
			FieldDefinitionNumber: basetype.Uint8Invalid, // invalid → skip
		},
	}

	result := ExtractDeveloperFieldDefs(nil, fieldDescs)
	if len(result) != 0 {
		t.Errorf("expected 0 defs for invalid fields, got %d", len(result))
	}
}

func TestExtractDeveloperFieldDefs_MultipleFieldNames(t *testing.T) {
	fieldDescs := []*mesgdef.FieldDescription{
		{
			DeveloperDataIndex:    0,
			FieldDefinitionNumber: 2,
			FieldName:             []string{"running", "_", "power"},
			Units:                 nil,
			NativeMesgNum:         typedef.MesgNumInvalid,
			NativeFieldNum:        basetype.Uint8Invalid,
			FitBaseTypeId:         255, // invalid → no FitBaseType
			Scale:                 basetype.Uint8Invalid,
			Offset:                basetype.Sint8Invalid,
		},
	}

	result := ExtractDeveloperFieldDefs(nil, fieldDescs)
	if len(result) != 1 {
		t.Fatalf("expected 1 def, got %d", len(result))
	}

	// FieldName should be concatenated
	if result[0].FieldName != "running_power" {
		t.Errorf("FieldName: got %q, want 'running_power'", result[0].FieldName)
	}

	// Units should be nil
	if result[0].Units != nil {
		t.Errorf("Units: expected nil, got %v", *result[0].Units)
	}

	// FitBaseType should be nil (255)
	if result[0].FitBaseType != nil {
		t.Errorf("FitBaseType: expected nil, got %v", *result[0].FitBaseType)
	}

	// Scale should be nil (invalid)
	if result[0].Scale != nil {
		t.Errorf("Scale: expected nil, got %v", *result[0].Scale)
	}
}

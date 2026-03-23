package fitimport

import (
	"strings"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractDeveloperFieldDefs(devDataIds []*mesgdef.DeveloperDataId, fieldDescs []*mesgdef.FieldDescription) []model.DeveloperFieldDef {
	if len(fieldDescs) == 0 {
		return nil
	}

	// Build index: developer_data_index → ApplicationId
	appIDs := make(map[uint8][]byte, len(devDataIds))
	for _, d := range devDataIds {
		if d.DeveloperDataIndex != basetype.Uint8Invalid && d.ApplicationId != nil {
			appIDs[d.DeveloperDataIndex] = d.ApplicationId
		}
	}

	var result []model.DeveloperFieldDef

	for _, fd := range fieldDescs {
		if fd.DeveloperDataIndex == basetype.Uint8Invalid || fd.FieldDefinitionNumber == basetype.Uint8Invalid {
			continue
		}

		def := model.DeveloperFieldDef{
			ApplicationID:         appIDs[fd.DeveloperDataIndex],
			DeveloperDataIndex:    int16(fd.DeveloperDataIndex),
			FieldDefinitionNumber: int16(fd.FieldDefinitionNumber),
		}

		// FieldName ([]string → joined single string)
		if len(fd.FieldName) > 0 {
			def.FieldName = strings.Join(fd.FieldName, "")
		}

		// Units ([]string → single string)
		if len(fd.Units) > 0 {
			joined := strings.Join(fd.Units, "")
			if joined != "" {
				def.Units = &joined
			}
		}

		// NativeMesgNum
		if fd.NativeMesgNum != typedef.MesgNumInvalid {
			v := int16(fd.NativeMesgNum)
			def.NativeMesgNum = &v
		}

		// NativeFieldNum
		if fd.NativeFieldNum != basetype.Uint8Invalid {
			v := int16(fd.NativeFieldNum)
			def.NativeFieldNum = &v
		}

		// FitBaseType
		if fd.FitBaseTypeId != 255 {
			v := int16(fd.FitBaseTypeId)
			def.FitBaseType = &v
		}

		// Scale
		if fd.Scale != basetype.Uint8Invalid {
			v := int16(fd.Scale)
			def.Scale = &v
		}

		// Offset
		if fd.Offset != basetype.Sint8Invalid {
			v := int16(fd.Offset)
			def.Offset = &v
		}

		result = append(result, def)
	}

	return result
}

package fitimport

import (
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractHrData(hrs []*mesgdef.Hr) []model.HrDataRecord {
	var result []model.HrDataRecord

	for _, h := range hrs {
		rec := model.HrDataRecord{
			Timestamp: h.Timestamp,
		}

		// FractionalTimestamp (Scale: 32768 → seconds)
		if h.FractionalTimestamp != basetype.Uint16Invalid {
			v := float64(h.FractionalTimestamp) / 32768.0
			rec.FractionalTimestamp = &v
		}

		// FilteredBpm ([]uint8 → []int16)
		if len(h.FilteredBpm) > 0 {
			bpms := make([]int16, len(h.FilteredBpm))
			for i, b := range h.FilteredBpm {
				bpms[i] = int16(b)
			}
			rec.FilteredBpm = bpms
		}

		// EventTimestamp ([]uint32, Scale: 1024 → []float64)
		if len(h.EventTimestamp) > 0 {
			ts := make([]float64, 0, len(h.EventTimestamp))
			for _, v := range h.EventTimestamp {
				if v != basetype.Uint32Invalid {
					ts = append(ts, float64(v)/1024.0)
				}
			}
			if len(ts) > 0 {
				rec.EventTimestamp = ts
			}
		}

		result = append(result, rec)
	}

	return result
}

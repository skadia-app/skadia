package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

type taxonomyResponse struct {
	Categories []categoryResponse `json:"categories"`
}

type categoryResponse struct {
	Key    string          `json:"key"`
	Label  string          `json:"label"`
	Sports []sportResponse `json:"sports"`
}

type sportResponse struct {
	SportType       string            `json:"sport_type"`
	Label           string            `json:"label"`
	TypicallyHasGPS bool              `json:"typically_has_gps"`
	Color           string            `json:"color"`
	SubTypes        []subTypeResponse `json:"sub_types"`
}

type subTypeResponse struct {
	SubType string `json:"sub_type"`
	Label   string `json:"label"`
}

func (h *Handler) GetSportsTaxonomy(w http.ResponseWriter, r *http.Request) {
	var resp taxonomyResponse
	for _, cat := range sport.AllCategories() {
		cr := categoryResponse{
			Key:   cat.Key,
			Label: cat.LabelDE,
		}
		for _, s := range sport.SportsInCategory(cat.Key) {
			sr := sportResponse{
				SportType:       s.SportType,
				Label:           s.LabelDE,
				TypicallyHasGPS: s.TypicallyHasGPS,
				Color:           cat.Color,
			}
			sr.SubTypes = subTypeResponses(s)
			cr.Sports = append(cr.Sports, sr)
		}
		if cr.Sports == nil {
			cr.Sports = []sportResponse{}
		}
		resp.Categories = append(resp.Categories, cr)
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

func subTypeResponses(def sport.SportDef) []subTypeResponse {
	subTypes := sport.OrderedSubTypes(def)
	out := make([]subTypeResponse, 0, len(subTypes))
	for _, st := range subTypes {
		out = append(out, subTypeResponse{
			SubType: st.SubType,
			Label:   st.LabelDE,
		})
	}
	return out
}

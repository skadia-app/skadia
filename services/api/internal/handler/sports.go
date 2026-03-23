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
	// Category colors matching frontend
	catColors := map[string]string{
		"run": "#EF4444", "bike": "#F97316", "winter": "#06B6D4",
		"water": "#3B82F6", "fitness": "#8B5CF6", "game": "#10B981",
		"climb": "#A855F7", "outdoor": "#84CC16", "multi": "#F59E0B",
		"other": "#6B7280",
	}

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
				Color:           catColors[s.CategoryKey],
			}
			for stKey, st := range s.SubTypes {
				_ = stKey
				sr.SubTypes = append(sr.SubTypes, subTypeResponse{
					SubType: st.SubType,
					Label:   st.LabelDE,
				})
			}
			if sr.SubTypes == nil {
				sr.SubTypes = []subTypeResponse{}
			}
			cr.Sports = append(cr.Sports, sr)
		}
		if cr.Sports == nil {
			cr.Sports = []sportResponse{}
		}
		resp.Categories = append(resp.Categories, cr)
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

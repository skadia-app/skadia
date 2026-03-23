package handler

import (
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func (h *Handler) GetPMC(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "from and to query parameters are required (YYYY-MM-DD)", r.URL.Path)
		return
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid from date, expected YYYY-MM-DD", r.URL.Path)
		return
	}

	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid to date, expected YYYY-MM-DD", r.URL.Path)
		return
	}

	if to.Before(from) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "to must be after from", r.URL.Path)
		return
	}

	days, err := h.DB.GetPMCRange(r.Context(), userID, from, to)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch PMC data", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, days)
}

// GetPowerCurve returns the best-effort power duration curve for a user.
func (h *Handler) GetPowerCurve(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}
	q := r.URL.Query()
	period := q.Get("period")
	fromStr := q.Get("from")
	toStr := q.Get("to")
	sportType := q.Get("sport_type")

	now := time.Now()
	var fromTime, toTime time.Time

	if fromStr != "" && toStr != "" {
		var err error
		fromTime, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid from date", r.URL.Path)
			return
		}
		toTime, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid to date", r.URL.Path)
			return
		}
	} else {
		toTime = now
		switch period {
		case "90d":
			fromTime = now.AddDate(0, 0, -90)
		case "all":
			fromTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		default: // ytd
			fromTime = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		}
	}

	powerStreams, err := h.DB.GetPowerStreamsForPeriod(r.Context(), userID, fromTime, toTime, sportType)
	if err != nil {
		slog.Error("get power streams for period failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load power data", r.URL.Path)
		return
	}

	durations := []int{1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600}
	bestPower := make(map[int]float64)
	for _, duration := range durations {
		bestPower[duration] = 0
	}

	for _, stream := range powerStreams {
		if len(stream) == 0 {
			continue
		}
		for _, duration := range durations {
			if len(stream) < duration {
				continue
			}
			// Sliding window mean
			var windowSum float64
			for i := range duration {
				windowSum += float64(stream[i])
			}
			best := windowSum / float64(duration)
			for i := duration; i < len(stream); i++ {
				windowSum += float64(stream[i]) - float64(stream[i-duration])
				avg := windowSum / float64(duration)
				if avg > best {
					best = avg
				}
			}
			if best > bestPower[duration] {
				bestPower[duration] = best
			}
		}
	}

	profile, _ := h.DB.GetUserProfile(r.Context(), userID)

	var points []model.PowerCurvePoint
	for _, duration := range durations {
		if bestPower[duration] > 0 {
			pt := model.PowerCurvePoint{
				DurationS: duration,
				PowerW:    math.Round(bestPower[duration]*10) / 10,
			}
			if profile != nil && profile.WeightKg != nil && *profile.WeightKg > 0 {
				wkg := math.Round(float64(bestPower[duration])/float64(*profile.WeightKg)*100) / 100
				pt.PowerWPerKg = &wkg
			}
			points = append(points, pt)
		}
	}

	resp := model.PowerCurveResponse{Points: points}
	if profile != nil {
		resp.FtpW = profile.FtpW
		resp.WeightKg = profile.WeightKg
	}
	if resp.Points == nil {
		resp.Points = []model.PowerCurvePoint{}
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

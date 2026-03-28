package pmc

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// Thresholds contains resolved threshold values for TSS computation.
// Each field is resolved independently through a 4-level cascade.
type Thresholds struct {
	LTHR           int               `json:"lthr"`
	FTP            int               `json:"ftp"`
	MaxHR          int               `json:"max_hr"`
	RestingHR      int               `json:"resting_hr"`
	SpeedThreshold *float64          `json:"speed_threshold,omitempty"`
	Sources        map[string]string `json:"sources"`
}

// HasHeuristicDefaults returns true if any field uses heuristic fallback.
func (t *Thresholds) HasHeuristicDefaults() bool {
	for _, v := range t.Sources {
		if strings.HasPrefix(v, "heuristic") {
			return true
		}
	}
	return false
}

// DefaultFields returns the list of fields using heuristic defaults.
func (t *Thresholds) DefaultFields() []string {
	var fields []string
	for k, v := range t.Sources {
		if strings.HasPrefix(v, "heuristic") {
			fields = append(fields, k)
		}
	}
	sort.Strings(fields)
	return fields
}

// EffectiveThresholds resolves sport-specific thresholds using a 4-level cascade:
// 1. training_zones for exact sport+sub_sport
// 2. training_zones for sport with sub_sport=0
// 3. user profile (lthr_bpm, ftp_w, EffectiveMaxHR, resting_hr_bpm)
// 4. heuristics (LTHR=88% MaxHR, FTP=200W, MaxHR=190, RestingHR=60)
//
// Resting HR special rule: between levels 2 and 3, health_daily_summary
// for the activity date is checked (daily RHR has priority over profile).
//
// SpeedThreshold (CSS/FTPace) has NO level 3+4 fallback — nil if not set.
func EffectiveThresholds(ctx context.Context, db *database.DB, userID string,
	sport, subSport int16, activityDate time.Time) (*Thresholds, error) {

	t := &Thresholds{
		Sources: make(map[string]string),
	}

	// Level 1: training_zones exact match (sport + sub_sport)
	if subSport != 0 {
		zones, err := db.GetTrainingZones(ctx, userID, &sport, &subSport)
		if err == nil {
			applyZones(t, zones, fmt.Sprintf("training_zones:%d/%d", sport, subSport))
		}
	}

	// Level 2: training_zones generic (sport, sub_sport=0)
	var zero int16
	zones, err := db.GetTrainingZones(ctx, userID, &sport, &zero)
	if err == nil {
		applyZones(t, zones, fmt.Sprintf("training_zones:%d/generic", sport))
	}

	// Resting HR special: daily value from health_daily_summary
	if t.RestingHR == 0 {
		dateStr := activityDate.Format("2006-01-02")
		summaries, err := db.GetHealthDailySummaries(ctx, userID, dateStr, dateStr)
		if err == nil && len(summaries) > 0 && summaries[0].RestingHeartRate != nil {
			t.RestingHR = *summaries[0].RestingHeartRate
			t.Sources["resting_hr"] = "health_daily_summary:" + dateStr
		}
	}

	// Level 3: User profile
	profile, err := db.GetUserProfile(ctx, userID)
	if err == nil && profile != nil {
		if t.LTHR == 0 {
			if profile.LthrBpm != nil && *profile.LthrBpm > 0 {
				t.LTHR = *profile.LthrBpm
				t.Sources["lthr"] = "user_profile"
			}
		}
		if t.FTP == 0 {
			if profile.FtpW != nil && *profile.FtpW > 0 {
				t.FTP = *profile.FtpW
				t.Sources["ftp"] = "user_profile"
			}
		}
		if t.MaxHR == 0 {
			t.MaxHR = profile.EffectiveMaxHR()
			t.Sources["max_hr"] = "user_profile"
		}
		if t.RestingHR == 0 {
			t.RestingHR = profile.EffectiveRestingHR()
			t.Sources["resting_hr"] = "user_profile"
		}
	}

	// Level 4: Heuristics
	if t.MaxHR == 0 {
		t.MaxHR = 190
		t.Sources["max_hr"] = "heuristic:190"
	}
	if t.LTHR == 0 {
		t.LTHR = int(float64(t.MaxHR) * 0.88)
		t.Sources["lthr"] = fmt.Sprintf("heuristic:88%%_of_%d", t.MaxHR)
	}
	if t.FTP == 0 {
		t.FTP = 200
		t.Sources["ftp"] = "heuristic:200W"
	}
	if t.RestingHR == 0 {
		t.RestingHR = 60
		t.Sources["resting_hr"] = "heuristic:60"
	}

	return t, nil
}

func applyZones(t *Thresholds, zones []model.TrainingZone, source string) {
	for _, z := range zones {
		if t.LTHR == 0 && z.ThresholdHeartRate != nil && *z.ThresholdHeartRate > 0 {
			t.LTHR = int(*z.ThresholdHeartRate)
			t.Sources["lthr"] = source
		}
		if t.FTP == 0 && z.FunctionalThresholdPower != nil && *z.FunctionalThresholdPower > 0 {
			t.FTP = int(*z.FunctionalThresholdPower)
			t.Sources["ftp"] = source
		}
		if t.MaxHR == 0 && z.MaxHeartRate != nil && *z.MaxHeartRate > 0 {
			t.MaxHR = int(*z.MaxHeartRate)
			t.Sources["max_hr"] = source
		}
		if t.SpeedThreshold == nil && z.ThresholdSpeedMps != nil && *z.ThresholdSpeedMps > 0 {
			v := float64(*z.ThresholdSpeedMps)
			t.SpeedThreshold = &v
			t.Sources["speed_threshold"] = source
		}
	}
}

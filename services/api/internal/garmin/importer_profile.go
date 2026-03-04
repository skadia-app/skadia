package garmin

import (
	"context"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ProfileImporter reads user profile and bio-metric data from a Garmin export.
// Real Garmin export structure (DI_CONNECT/):
//   - DI-Connect-User/user_profile.json          — basic profile (userName, firstName, gender, birthDate)
//   - DI-Connect-Wellness/112424438_userBioMetricProfileData.json — height, weight, FTP, LTHR, VO2Max
type ProfileImporter struct{}

func (p *ProfileImporter) Name() string        { return "profile" }
func (p *ProfileImporter) Description() string { return "Nutzerprofil (Geburtstag, Größe, Gewicht, FTP)" }
func (p *ProfileImporter) Priority() int       { return 10 }

func (p *ProfileImporter) Detect(src ExportSource) (bool, int, error) {
	if src.Exists("DI_CONNECT/DI-Connect-User/user_profile.json") {
		return true, 1, nil
	}
	return false, 0, nil
}

// garminUserProfile matches DI_CONNECT/DI-Connect-User/user_profile.json
type garminUserProfile struct {
	Gender    string `json:"gender"`
	BirthDate string `json:"birthDate"` // e.g. "1980-06-15"
}

// garminBioMetricProfile matches DI_CONNECT/DI-Connect-Wellness/*_userBioMetricProfileData.json
type garminBioMetricProfile struct {
	Height                    *float64 `json:"height"`                    // cm
	Weight                    *float64 `json:"weight"`                    // grams (Garmin stores in grams)
	FunctionalThresholdPower  *float64 `json:"functionalThresholdPower"`  // watts (may arrive as float)
	LactateThresholdHeartRate *float64 `json:"lactateThresholdHeartRate"` // bpm (arrives as float e.g. 167.0)
	VO2Max                    *float64 `json:"vo2Max"`
}

func (p *ProfileImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}

	// 1. Basic profile
	var profile garminUserProfile
	if err := params.Source.ReadJSON("DI_CONNECT/DI-Connect-User/user_profile.json", &profile); err != nil {
		slog.Warn("garmin profile: cannot read user_profile.json", "error", err)
	}

	// 2. Bio-metric profile (find by glob since the user ID prefix is unknown)
	var bioProfiles []garminBioMetricProfile
	bioFiles, _ := params.Source.FindFiles("DI_CONNECT/DI-Connect-Wellness/*_userBioMetricProfileData.json")
	if len(bioFiles) > 0 {
		if err := params.Source.ReadJSON(bioFiles[0], &bioProfiles); err != nil {
			slog.Warn("garmin profile: cannot read bioMetricProfileData", "error", err)
		}
	}

	// Build UserProfileImport
	imp := &model.UserProfileImport{}

	if profile.Gender != "" {
		switch profile.Gender {
		case "MALE":
			imp.Gender = "male"
		case "FEMALE":
			imp.Gender = "female"
		}
	}
	if profile.BirthDate != "" {
		// SetUserProfileFieldsIfNull expects AgeYears, but we can set birth_date directly via UpdateUserProfile.
		// Use the dedicated method below that accepts an ISO date string.
	}

	if len(bioProfiles) > 0 {
		bio := bioProfiles[0]
		if bio.Height != nil && *bio.Height > 0 {
			imp.HeightCm = int(*bio.Height)
		}
		if bio.Weight != nil && *bio.Weight > 0 {
			imp.WeightKg = float32(*bio.Weight / 1000.0) // Garmin stores weight in grams
		}
		if bio.FunctionalThresholdPower != nil && *bio.FunctionalThresholdPower > 0 {
			imp.FTP = int(*bio.FunctionalThresholdPower)
		}
		if bio.LactateThresholdHeartRate != nil && *bio.LactateThresholdHeartRate > 0 {
			imp.LTHR = int(*bio.LactateThresholdHeartRate)
		}
	}

	if err := params.DB.SetUserProfileFieldsIfNull(ctx, params.UserID, imp); err != nil {
		slog.Warn("garmin profile: set user profile fields failed", "error", err)
	}

	// Set birth_date directly if we have one
	if profile.BirthDate != "" {
		if err := params.DB.SetUserBirthDateIfNull(ctx, params.UserID, profile.BirthDate); err != nil {
			slog.Warn("garmin profile: set birth_date failed", "error", err)
		}
	}

	slog.Info("garmin profile import done", "user", params.UserID)
	result.Imported = 1
	return result, nil
}

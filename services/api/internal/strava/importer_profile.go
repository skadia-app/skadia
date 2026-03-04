package strava

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ProfileImporter reads user profile data from profile.csv and general_preferences.csv.
//
// profile.csv columns (German):
//
//	ID des Sportlers, E-Mail-Adresse, Vorname, Nachname, Geschlecht, Beschreibung,
//	Gewicht, Stadt, Bundesland (optional), Land, ...
//
// general_preferences.csv columns (German):
//
//	Geburtsdatum, Gewicht, Funktionale Schwellenleistung, Maximale Herzfrequenz, ...
type ProfileImporter struct{}

func (p *ProfileImporter) Name() string        { return "profile" }
func (p *ProfileImporter) Description() string { return "Nutzerprofil (Geburtstag, Gewicht, FTP, max. HR)" }
func (p *ProfileImporter) Priority() int       { return 10 }

func (p *ProfileImporter) Detect(src ExportSource) (bool, int, error) {
	if src.Exists("profile.csv") {
		return true, 1, nil
	}
	return false, 0, nil
}

func (p *ProfileImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}
	imp := &model.UserProfileImport{}
	var birthDate string

	// --- 1. profile.csv ---
	if rows, err := params.Source.ReadCSV("profile.csv"); err == nil && len(rows) >= 2 {
		hdr := csvHeader(rows[0])
		row := rows[1]

		// Gender: "Männlich" / "Weiblich"
		if idx, ok := hdr["Geschlecht"]; ok {
			g := colVal(row, idx)
			switch {
			case strings.EqualFold(g, "Männlich") || strings.EqualFold(g, "Mannlich") || strings.EqualFold(g, "Male"):
				imp.Gender = "male"
			case strings.EqualFold(g, "Weiblich") || strings.EqualFold(g, "Female"):
				imp.Gender = "female"
			}
		}

		// Weight (kg) from profile: "84.0" (dot decimal, no unit)
		if idx, ok := hdr["Gewicht"]; ok {
			if w, ok2 := parseGermanFloat(colVal(row, idx)); ok2 && w > 0 {
				imp.WeightKg = float32(w)
			}
		}
	} else if err != nil {
		slog.Warn("strava profile: cannot read profile.csv", "error", err)
	}

	// --- 2. general_preferences.csv ---
	if rows, err := params.Source.ReadCSV("general_preferences.csv"); err == nil && len(rows) >= 2 {
		hdr := csvHeader(rows[0])
		row := rows[1]

		// Geburtsdatum: "14.07.1990" (DD.MM.YYYY)
		if idx, ok := hdr["Geburtsdatum"]; ok {
			dateStr := colVal(row, idx)
			if t, err := time.Parse("02.01.2006", dateStr); err == nil {
				birthDate = t.Format("2006-01-02") // ISO 8601
			}
		}

		// Gewicht: "84,0 kg" — strip " kg" suffix
		if idx, ok := hdr["Gewicht"]; ok {
			weightStr := colVal(row, idx)
			weightStr = strings.ReplaceAll(weightStr, " kg", "")
			weightStr = strings.ReplaceAll(weightStr, "kg", "")
			if w, ok2 := parseGermanFloat(weightStr); ok2 && w > 0 && imp.WeightKg == 0 {
				imp.WeightKg = float32(w)
			}
		}

		// FTP: "186 W" → strip " W"
		if idx, ok := hdr["Funktionale Schwellenleistung"]; ok {
			ftpStr := colVal(row, idx)
			ftpStr = strings.ReplaceAll(ftpStr, " W", "")
			ftpStr = strings.ReplaceAll(ftpStr, "W", "")
			if ftp, ok2 := parseGermanInt(ftpStr); ok2 && ftp > 0 {
				imp.FTP = ftp
			}
		}

		// Max HR: "185"
		if idx, ok := hdr["Maximale Herzfrequenz"]; ok {
			if hr, ok2 := parseGermanInt(colVal(row, idx)); ok2 && hr > 0 {
				imp.MaxHR = hr
			}
		}
	} else if err != nil {
		slog.Warn("strava profile: cannot read general_preferences.csv", "error", err)
	}

	// Apply to DB (only sets fields that are NULL)
	if err := params.DB.SetUserProfileFieldsIfNull(ctx, params.UserID, imp); err != nil {
		slog.Warn("strava profile: set user profile fields failed", "error", err)
	} else {
		slog.Info("strava profile: applied profile fields", "user", params.UserID,
			"weight", imp.WeightKg, "ftp", imp.FTP, "max_hr", imp.MaxHR, "gender", imp.Gender)
	}

	if birthDate != "" {
		if err := params.DB.SetUserBirthDateIfNull(ctx, params.UserID, birthDate); err != nil {
			slog.Warn("strava profile: set birth_date failed", "error", err)
		} else {
			slog.Info("strava profile: set birth_date", "date", birthDate)
		}
	}

	slog.Info("strava profile import done", "user", params.UserID)
	result.Imported = 1
	return result, nil
}

package model

import (
	"strings"
	"time"
)

// DateOnly is a time.Time that marshals/unmarshals as "YYYY-MM-DD" in JSON.
type DateOnly struct{ time.Time }

func (d DateOnly) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.Format("2006-01-02") + `"`), nil
}

func (d *DateOnly) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" || s == "" {
		return nil
	}
	// Accept both "2006-01-02" and full RFC3339
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return err
		}
	}
	d.Time = t
	return nil
}

// User represents a Skadia user, mapped from OIDC via OIDC.
type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName *string   `json:"display_name,omitempty"`
	Email       *string   `json:"email,omitempty"`
	LegacyOIDCSubject string    `json:"-"`
	IsAdmin     bool      `json:"is_admin"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserProfile holds physiological data for training zone calculations.
type UserProfile struct {
	BirthDate        *DateOnly `json:"birth_date,omitempty"`
	Gender           string    `json:"gender"`
	HeightCm         *int      `json:"height_cm,omitempty"`
	WeightKg         *float32  `json:"weight_kg,omitempty"`
	FtpW             *int      `json:"ftp_w,omitempty"`
	LthrBpm          *int      `json:"lthr_bpm,omitempty"`
	MaxHrBpm         *int      `json:"max_hr_bpm,omitempty"`
	RestingHrBpm     *int      `json:"resting_hr_bpm,omitempty"`
	MaxHrFormulaBpm  *int      `json:"max_hr_formula,omitempty"`
	MaxHrDetectedBpm *int      `json:"max_hr_detected,omitempty"`
	EffectiveMaxHr   int       `json:"effective_max_hr"`
}

// EffectiveMaxHR returns the best available max heart rate estimate.
// Priority: manual override > detected from data > Tanaka formula > default 220.
func (p *UserProfile) EffectiveMaxHR() int {
	if p.MaxHrBpm != nil && *p.MaxHrBpm > 0 {
		return *p.MaxHrBpm
	}
	if p.MaxHrDetectedBpm != nil && *p.MaxHrDetectedBpm > 0 {
		return *p.MaxHrDetectedBpm
	}
	if p.MaxHrFormulaBpm != nil && *p.MaxHrFormulaBpm > 0 {
		return *p.MaxHrFormulaBpm
	}
	return 220
}

// EffectiveRestingHR returns resting HR or a default of 60.
func (p *UserProfile) EffectiveRestingHR() int {
	if p.RestingHrBpm != nil && *p.RestingHrBpm > 0 {
		return *p.RestingHrBpm
	}
	return 60
}

// UserProfileUpdate is the PATCH request body for physiological profile.
type UserProfileUpdate struct {
	BirthDate    *DateOnly `json:"birth_date,omitempty"`
	Gender       *string   `json:"gender,omitempty"`
	HeightCm     *int      `json:"height_cm,omitempty"`
	WeightKg     *float32  `json:"weight_kg,omitempty"`
	FtpW         *int      `json:"ftp_w,omitempty"`
	LthrBpm      *int      `json:"lthr_bpm,omitempty"`
	MaxHrBpm     *int      `json:"max_hr_bpm,omitempty"`
	RestingHrBpm *int      `json:"resting_hr_bpm,omitempty"`
}

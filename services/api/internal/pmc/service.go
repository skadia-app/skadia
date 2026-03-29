package pmc

import (
	"context"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// Service handles PMC recalculation triggered by events.
type Service struct {
	DB *database.DB
}

// RecomputeAllTSS recalculates TSS for all activities of a user,
// then recalculates the PMC. Used after threshold changes or initial migration.
func (s *Service) RecomputeAllTSS(ctx context.Context, userID string) error {
	rows, err := s.DB.Pool.Query(ctx, `
		SELECT id, fit_sport_id, fit_sub_sport_id, moving_time_s,
		       avg_power_w, avg_heart_rate, distance_m, started_at
		FROM activities WHERE user_id = $1 AND NOT exclude_from_stats
		ORDER BY started_at
	`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id        string
			sportID   *int16
			subSport  *int16
			movTimeS  *int
			avgPower  *float64
			avgHR     *float64
			distM     *float64
			startedAt time.Time
		)
		if err := rows.Scan(&id, &sportID, &subSport, &movTimeS,
			&avgPower, &avgHR, &distM, &startedAt); err != nil {
			slog.Warn("RecomputeAllTSS: scan failed", "err", err)
			continue
		}
		sport := int16(0)
		if sportID != nil {
			sport = *sportID
		}
		sub := int16(0)
		if subSport != nil {
			sub = *subSport
		}
		movTime := 0
		if movTimeS != nil {
			movTime = *movTimeS
		}

		th, err := EffectiveThresholds(ctx, s.DB, userID, sport, sub, startedAt)
		if err != nil {
			slog.Warn("RecomputeAllTSS: thresholds failed", "activity", id, "err", err)
			continue
		}

		streams, _ := s.DB.GetStreamPoints(ctx, id)
		result := ComputeTSS(sport, sub, movTime,
			avgPower, avgHR, distM,
			ExtractPowerStream(streams), ExtractHRStream(streams),
			ExtractSpeedStream(streams), ExtractGradeStream(streams),
			nil, th)

		if err := s.DB.UpdateActivityTSS(ctx, id,
			result.ComputedTSS, result.ComputedNPW, result.ComputedNGPMps,
			result.ComputedIF, result.TRIMP, result.TSSMethod,
		); err != nil {
			slog.Warn("RecomputeAllTSS: update failed", "activity", id, "err", err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return s.RecalculatePMC(ctx, userID, time.Time{})
}

// RecalculatePMC recalculates PMC from the user's earliest activity up to 90 days from now.
// This satisfies the event.ActivityHandler interface.
func (s *Service) RecalculatePMC(ctx context.Context, userID string, activityDate time.Time) error {
	// Start from the user's earliest activity (minus 42 days for CTL seeding window).
	// Fall back to 180 days if no activities exist.
	earliest, err := s.DB.GetEarliestActivityDate(ctx, userID)
	if err != nil {
		return err
	}
	if earliest.IsZero() {
		return nil // no activities yet, nothing to calculate
	}
	// Start 42 days before the first activity so CTL/ATL can seed properly.
	from := earliest.AddDate(0, 0, -42)
	to := time.Now().AddDate(0, 0, 90)

	// Get previous day's CTL/ATL to seed the calculation
	prevCTL, prevATL, err := s.DB.GetPMCDayBefore(ctx, userID, from)
	if err != nil {
		return err
	}

	// Get daily TSS from activities
	dailyTSS, err := s.DB.GetDailyTSS(ctx, userID, from, to)
	if err != nil {
		return err
	}

	// Calculate PMC series
	days := Calculate(from, to, dailyTSS, prevCTL, prevATL)
	if len(days) == 0 {
		return nil
	}

	// Set userID on all days for batch upsert
	for i := range days {
		days[i].UserID = userID
	}

	// Batch upsert
	if err := s.DB.UpsertPMCDays(ctx, userID, days); err != nil {
		return err
	}

	slog.Info("PMC recalculated",
		"user_id", userID,
		"from", from.Format("2006-01-02"),
		"to", to.Format("2006-01-02"),
		"days", len(days),
	)
	return nil
}

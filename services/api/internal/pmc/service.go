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

// RecalculatePMC recalculates PMC from the user's earliest activity up to 90 days from now.
// This satisfies the event.ActivityHandler interface.
func (s *Service) RecalculatePMC(ctx context.Context, userID string, activityDate time.Time) error {
	// Start from the user's earliest activity (minus 42 days for CTL seeding window).
	// Fall back to 180 days if no activities exist.
	earliest, err := s.DB.GetEarliestActivityDate(ctx, userID)
	if err != nil {
		return err
	}
	from := time.Now().AddDate(0, 0, -180)
	if !earliest.IsZero() {
		seed := earliest.AddDate(0, 0, -42)
		if seed.Before(from) {
			from = seed
		}
	}
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

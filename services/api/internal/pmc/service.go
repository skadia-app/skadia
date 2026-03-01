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
	// Calculate from 180 days ago to 90 days from now (covers full CTL window)
	from := time.Now().AddDate(0, 0, -180)
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

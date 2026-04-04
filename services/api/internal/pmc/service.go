package pmc

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// pmcLockBase is the domain prefix for PMC advisory lock keys. "PMCC" = 0x504D4343.
const pmcLockBase = int64(0x504D4343)

// pmcLockKey returns a stable per-user int64 advisory lock key for PMC operations.
func pmcLockKey(userID string) int64 {
	h := fnv.New32a()
	h.Write([]byte(userID)) //nolint:errcheck
	return pmcLockBase ^ int64(h.Sum32())
}

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
		FROM activities WHERE user_id = $1
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
		streams = DeduplicateStreamsByTimestamp(streams)
		result := ComputeTSS(sport, sub, movTime,
			avgPower, avgHR, distM,
			ExtractPowerStream(streams, sport), ExtractHRStream(streams),
			ExtractSpeedStream(streams), ExtractGradeStream(streams),
			ExtractGAPStream(streams), th)

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

// RecalculatePMC recalculates PMC (combined + per-sport) from the user's earliest activity
// up to 90 days from now. This satisfies the event.ActivityHandler interface.
//
// A non-blocking advisory lock is used to prevent parallel recalcs for the same user.
// If another recalc is already running for this user, the call is a no-op (returns nil).
func (s *Service) RecalculatePMC(ctx context.Context, userID string, activityDate time.Time) error {
	// Prevent parallel PMC recalcs for the same user. pg_try_advisory_lock is
	// non-blocking — if a recalc is already in progress we skip this one.
	lockKey := pmcLockKey(userID)
	var acquired bool
	if err := s.DB.Pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired); err != nil {
		return fmt.Errorf("pmc: try advisory lock: %w", err)
	}
	if !acquired {
		slog.DebugContext(ctx, "pmc: skipping recalc, another recalc is running", "user_id", userID)
		return nil
	}
	defer s.DB.Pool.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockKey) //nolint:errcheck

	// Start from the user's earliest activity (minus 42 days for CTL seeding window).
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

	// Seed combined CTL/ATL from day before `from`.
	prevCTL, prevATL, err := s.DB.GetPMCDayBefore(ctx, userID, from)
	if err != nil {
		return err
	}

	// Seed per-sport CTL from day before `from`.
	prevSportCTL, err := s.DB.GetPMCSportCTLBefore(ctx, userID, from)
	if err != nil {
		return err
	}

	// Get daily TSS split by sport group.
	dailyTSSBySport, err := s.DB.GetDailyTSSBySport(ctx, userID, from, to)
	if err != nil {
		return err
	}

	// Compute combined daily TSS for ACWR_RA (which only applies to combined rows).
	dailyTSS := make(map[string]float64, len(dailyTSSBySport))
	for dateStr, sportMap := range dailyTSSBySport {
		var total float64
		for _, tss := range sportMap {
			total += tss
		}
		dailyTSS[dateStr] = total
	}

	// Calculate multi-sport PMC series in a single pass.
	days := CalculateMultiSport(from, to, dailyTSSBySport, prevCTL, prevATL, prevSportCTL)
	if len(days) == 0 {
		return nil
	}

	// Populate ACWR_RA for combined rows only.
	for i, d := range days {
		if d.SportGroup != "__combined__" {
			continue
		}
		dayDate, err := time.Parse("2006-01-02", d.Date)
		if err != nil {
			continue
		}
		days[i].ACWRRA = CalcACWR_RA(dailyTSS, dayDate)
	}

	// Set userID on all days for batch upsert.
	for i := range days {
		days[i].UserID = userID
	}

	// Delete stale per-sport rows before upserting the freshly computed series.
	// This removes sport-group rows that no longer have any activities (e.g. after
	// a sport-type edit or activity deletion) and prevents phantom sport entries
	// from persisting indefinitely.
	if err := s.DB.DeleteStalePMCSportRows(ctx, userID, from, to); err != nil {
		return fmt.Errorf("pmc: delete stale sport rows: %w", err)
	}

	// Batch upsert.
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

// ComputeActivityTSS calculates and stores TSS for a single activity.
// Called after FIT import to immediately populate computed_tss.
func (s *Service) ComputeActivityTSS(ctx context.Context, userID, activityID string,
	sport, subSport int16, movingTimeS int, avgPowerW, avgHR, distanceM *float64,
	startedAt time.Time) error {

	th, err := EffectiveThresholds(ctx, s.DB, userID, sport, subSport, startedAt)
	if err != nil {
		return err
	}

	streams, _ := s.DB.GetStreamPoints(ctx, activityID)
	streams = DeduplicateStreamsByTimestamp(streams)
	result := ComputeTSS(sport, subSport, movingTimeS,
		avgPowerW, avgHR, distanceM,
		ExtractPowerStream(streams, sport), ExtractHRStream(streams),
		ExtractSpeedStream(streams), ExtractGradeStream(streams),
		ExtractGAPStream(streams), th)

	return s.DB.UpdateActivityTSS(ctx, activityID,
		result.ComputedTSS, result.ComputedNPW, result.ComputedNGPMps,
		result.ComputedIF, result.TRIMP, result.TSSMethod)
}

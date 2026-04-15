package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetFusionQueueActivities gibt Aktivitäten eines Users zurück
// die GPS-Daten haben und noch nicht als Fusion-Quelle oder Merge-Ziel bekannt sind.
// limit <= 0 gibt alle zurück; limit > 0 beschränkt auf die neuesten N.
func (db *DB) GetFusionQueueActivities(ctx context.Context, userID string, limit int) ([]string, error) {
	query := `
		SELECT a.id
		FROM activities a
		WHERE a.user_id = $1
		  AND a.is_group_secondary = FALSE
		  AND a.has_gps = TRUE
		  AND NOT EXISTS (
		      SELECT 1 FROM activity_fusion_memberships m WHERE m.source_id = a.id
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM activity_fusion_memberships m WHERE m.merged_id = a.id
		  )
		ORDER BY a.started_at DESC`
	var rows pgx.Rows
	var err error
	if limit > 0 {
		rows, err = db.Pool.Query(ctx, query+" LIMIT $2", userID, limit)
	} else {
		rows, err = db.Pool.Query(ctx, query, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// FusionSources mappt Stream-Typen auf Quell-Aktivitäts-IDs.
// z.B. {"gps": "uuid-A", "hr": "uuid-B", "power": "uuid-B"}
type FusionSources map[string]string

// SetFusionSources setzt die fusion_sources einer Aktivität.
// userID wird als zusätzlicher Guard in die WHERE-Klausel einbezogen,
// damit der DB-Layer auch bei direkten Aufrufen ownership-sicher ist.
func (db *DB) SetFusionSources(ctx context.Context, activityID string, userID string, sources FusionSources) error {
	data, err := json.Marshal(sources)
	if err != nil {
		return fmt.Errorf("marshal fusion sources: %w", err)
	}
	_, err = db.Pool.Exec(ctx,
		`UPDATE activities SET fusion_sources = $2 WHERE id = $1 AND user_id = $3`,
		activityID, data, userID,
	)
	return err
}

// GetFusionSources liest die fusion_sources einer Aktivität.
// Gibt nil zurück wenn keine vorhanden.
func (db *DB) GetFusionSources(ctx context.Context, activityID string) (FusionSources, error) {
	var raw []byte
	err := db.Pool.QueryRow(ctx,
		`SELECT fusion_sources FROM activities WHERE id = $1`, activityID,
	).Scan(&raw)
	if err != nil || raw == nil {
		return nil, err
	}
	var sources FusionSources
	return sources, json.Unmarshal(raw, &sources)
}

// MarkAsFusionSource markiert eine Aktivität als Quell-Aktivität einer Fusion.
// Fusion-Sources werden in der Aktivitätsliste ausgeblendet.
func (db *DB) MarkAsFusionSource(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET is_fusion_source = TRUE WHERE id = $1`, activityID,
	)
	return err
}

// FindFusionCandidates sucht Aktivitäten die zeitlich nahe beieinander liegen.
// timeWindowMin gibt das maximale Zeitfenster in Minuten an (Default 30).
// Sport-Typ wird bewusst NICHT gefiltert, da dieselbe Aktivität
// auf verschiedenen Geräten mit unterschiedlichem Sport-Typ geloggt werden kann
// (z.B. Wanderung als "cycling" auf Radcomputer + "hiking" auf GPS-Gerät).
// Gibt IDs von Kandidaten zurück (max. 10).
func (db *DB) FindFusionCandidates(ctx context.Context, userID string, activityID string, timeWindowMin int) ([]string, error) {
	if timeWindowMin <= 0 {
		timeWindowMin = 30
	}
	var candidateIDs []string
	rows, err := db.Pool.Query(ctx, `
        SELECT b.id FROM activities a
        JOIN activities b ON b.user_id = a.user_id
            AND b.id != a.id
            AND b.is_group_secondary = FALSE
            AND NOT EXISTS (SELECT 1 FROM activity_fusion_memberships m WHERE m.source_id = b.id)
            AND (
                -- Zeitlicher Overlap: Aktivitäten überschneiden sich
                (b.ended_at IS NOT NULL AND a.ended_at IS NOT NULL
                 AND b.started_at < a.ended_at AND b.ended_at > a.started_at)
                OR
                -- Fallback: Start-Differenz innerhalb Zeitfenster
                ABS(EXTRACT(EPOCH FROM (b.started_at - a.started_at))) <= $3
            )
        WHERE a.id = $1 AND a.user_id = $2
        LIMIT 10`,
		activityID, userID, timeWindowMin*60,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		candidateIDs = append(candidateIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candidateIDs, nil
}

// FusionCandidateRow ist das Ergebnis einer GetCachedFusionCandidates-Abfrage,
// inklusive der zugehörigen Aktivitäts-Metadaten für beide Seiten des Paares.
type FusionCandidateRow struct {
	ActivityIDA        string
	ActivityIDB        string
	OverlapPercent     float64
	FieldRichnessA     int
	FieldRichnessB     int
	TimeDiffSec        float64
	TitleA             *string
	TitleAutoA         *string
	StartedAtA         time.Time
	SportTypeA         string
	SubTypeA           *string
	IsElectricA        bool
	DistanceMA         *float64
	SourceDeviceA      *string
	SourceA            *string
	OriginalFileFormatA *string
	TitleB             *string
	TitleAutoB         *string
	StartedAtB         time.Time
	SportTypeB         string
	SubTypeB           *string
	IsElectricB        bool
	DistanceMB         *float64
	SourceDeviceB      *string
	SourceB            *string
	OriginalFileFormatB *string
}

// FusionCandidateInsert beschreibt einen einzelnen Eintrag für ReplaceFusionCandidates.
type FusionCandidateInsert struct {
	ActivityIDA    string
	ActivityIDB    string
	OverlapPercent float64
	FieldRichnessA int
	FieldRichnessB int
	TimeDiffSec    float64
}

// GetAllFusionCandidatePairs gibt alle Paar-Daten aus fusion_candidates für einen User zurück,
// sortiert nach overlap_percent DESC. Wird für die Clique-Berechnung verwendet.
func (db *DB) GetAllFusionCandidatePairs(ctx context.Context, userID string) ([]model.PairData, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT activity_id_a, activity_id_b, overlap_percent, field_richness_a, field_richness_b, time_diff_sec
		FROM fusion_candidates
		WHERE user_id = $1
		ORDER BY overlap_percent DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get all fusion candidate pairs: %w", err)
	}
	defer rows.Close()

	var pairs []model.PairData
	for rows.Next() {
		var p model.PairData
		if err := rows.Scan(&p.ActivityIDA, &p.ActivityIDB, &p.OverlapPercent, &p.FieldRichnessA, &p.FieldRichnessB, &p.TimeDiffSec); err != nil {
			return nil, fmt.Errorf("scan fusion candidate pair: %w", err)
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fusion candidate pairs: %w", err)
	}
	if pairs == nil {
		pairs = []model.PairData{}
	}
	return pairs, nil
}

// GetMultisportChildIDs gibt die Teilmenge der übergebenen IDs zurück,
// die aus der Clique-Bildung ausgeschlossen werden müssen:
// Multisport-Teilaktivitäten (parent_id IS NOT NULL) oder
// Fusion-Quell-Aktivitäten (Spec §3.4, geprüft via activity_fusion_memberships).
func (db *DB) GetMultisportChildIDs(ctx context.Context, activityIDs []string) (map[string]bool, error) {
	if len(activityIDs) == 0 {
		return map[string]bool{}, nil
	}
	rows, err := db.Pool.Query(ctx,
		`SELECT a.id FROM activities a WHERE a.id = ANY($1) AND (
		    a.parent_id IS NOT NULL
		    OR EXISTS (SELECT 1 FROM activity_fusion_memberships m WHERE m.source_id = a.id)
		)`,
		activityIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("get multisport child ids: %w", err)
	}
	defer rows.Close()

	children := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan multisport child id: %w", err)
		}
		children[id] = true
	}
	return children, rows.Err()
}

// IsFusionCacheStale prüft ob der Fusion-Cache eines Users veraltet ist.
// Gibt stale=true zurück wenn fusion_invalidation_seq > fusion_computed_seq.
// invalidationSeq wird zurückgegeben damit der Background-Job den Wert snapshotten kann.
// Wenn der User nicht gefunden wird, wird stale=true, invalidationSeq=0, err=nil zurückgegeben.
func (db *DB) IsFusionCacheStale(ctx context.Context, userID string) (stale bool, invalidationSeq int64, err error) {
	var invalidation, computed int64
	err = db.Pool.QueryRow(ctx,
		`SELECT fusion_invalidation_seq, fusion_computed_seq FROM users WHERE id = $1`,
		userID,
	).Scan(&invalidation, &computed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, 0, nil
		}
		return false, 0, fmt.Errorf("check fusion cache staleness: %w", err)
	}
	return invalidation > computed, invalidation, nil
}

// GetCachedFusionCandidates liest die gecachten Fusion-Kandidaten eines Users
// aus fusion_candidates, angereichert mit den Aktivitäts-Metadaten beider Seiten.
// Gibt einen leeren Slice (nie nil) zurück wenn keine Einträge vorhanden sind.
func (db *DB) GetCachedFusionCandidates(ctx context.Context, userID string, limit int) ([]FusionCandidateRow, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			fc.activity_id_a, fc.activity_id_b,
			fc.overlap_percent, fc.field_richness_a, fc.field_richness_b, fc.time_diff_sec,
			a1.title, a1.title_auto, a1.started_at, a1.sport_type, a1.sub_type, a1.is_electric, a1.distance_m,
			a1.source_device, a1.source, a1.original_file_format,
			a2.title, a2.title_auto, a2.started_at, a2.sport_type, a2.sub_type, a2.is_electric, a2.distance_m,
			a2.source_device, a2.source, a2.original_file_format
		FROM fusion_candidates fc
		JOIN activities a1 ON a1.id = fc.activity_id_a
		JOIN activities a2 ON a2.id = fc.activity_id_b
		WHERE fc.user_id = $1
		ORDER BY fc.overlap_percent DESC
		LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query cached fusion candidates: %w", err)
	}
	defer rows.Close()
	result := []FusionCandidateRow{}
	for rows.Next() {
		var r FusionCandidateRow
		if err := rows.Scan(
			&r.ActivityIDA, &r.ActivityIDB,
			&r.OverlapPercent, &r.FieldRichnessA, &r.FieldRichnessB, &r.TimeDiffSec,
			&r.TitleA, &r.TitleAutoA, &r.StartedAtA, &r.SportTypeA, &r.SubTypeA, &r.IsElectricA, &r.DistanceMA,
			&r.SourceDeviceA, &r.SourceA, &r.OriginalFileFormatA,
			&r.TitleB, &r.TitleAutoB, &r.StartedAtB, &r.SportTypeB, &r.SubTypeB, &r.IsElectricB, &r.DistanceMB,
			&r.SourceDeviceB, &r.SourceB, &r.OriginalFileFormatB,
		); err != nil {
			return nil, fmt.Errorf("scan fusion candidate row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fusion candidates: %w", err)
	}
	return result, nil
}

// ReplaceFusionCandidates ersetzt alle gecachten Fusion-Kandidaten eines Users
// in einer einzelnen Transaktion: löscht alte Einträge, fügt neue ein und
// aktualisiert fusion_computed_seq auf den übergebenen computedSeq-Wert.
// Wenn candidates leer ist, werden nur alte Einträge gelöscht und computed_seq gesetzt.
func (db *DB) ReplaceFusionCandidates(ctx context.Context, userID string, candidates []FusionCandidateInsert, computedSeq int64) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`DELETE FROM fusion_candidates WHERE user_id = $1`,
		userID,
	); err != nil {
		return fmt.Errorf("delete old fusion candidates: %w", err)
	}

	if len(candidates) > 0 {
		batch := &pgx.Batch{}
		for _, c := range candidates {
			batch.Queue(
				`INSERT INTO fusion_candidates
					(user_id, activity_id_a, activity_id_b, overlap_percent, field_richness_a, field_richness_b, time_diff_sec)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				userID, c.ActivityIDA, c.ActivityIDB, c.OverlapPercent, c.FieldRichnessA, c.FieldRichnessB, c.TimeDiffSec,
			)
		}
		br := tx.SendBatch(ctx, batch)
		for i := range candidates {
			if _, err := br.Exec(); err != nil {
				br.Close() //nolint:errcheck
				return fmt.Errorf("insert fusion candidate %d: %w", i, err)
			}
		}
		if err := br.Close(); err != nil {
			return fmt.Errorf("close batch: %w", err)
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET fusion_computed_seq = $2 WHERE id = $1`,
		userID, computedSeq,
	); err != nil {
		return fmt.Errorf("update fusion_computed_seq: %w", err)
	}

	return tx.Commit(ctx)
}

// InvalidateFusionCache erhöht fusion_invalidation_seq eines Users um 1.
// Wird aufgerufen wenn sich Import-Einstellungen ändern, die den Fusion-Queue beeinflussen.
func (db *DB) InvalidateFusionCache(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET fusion_invalidation_seq = fusion_invalidation_seq + 1 WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("invalidate fusion cache: %w", err)
	}
	return nil
}

// DeleteFusionCandidatesForActivity löscht alle gecachten Kandidatenpaare
// in denen die gegebene Aktivität auf einer der beiden Seiten vorkommt.
// Wird nach einer erfolgreichen Fusion-Merge aufgerufen um veraltete Paare sofort zu entfernen.
func (db *DB) DeleteFusionCandidatesForActivity(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM fusion_candidates WHERE activity_id_a = $1 OR activity_id_b = $1`,
		activityID,
	)
	if err != nil {
		return fmt.Errorf("delete fusion candidates for activity: %w", err)
	}
	return nil
}

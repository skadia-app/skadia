package database

import (
	"context"
	"encoding/json"
	"fmt"
)

// FusionSources mappt Stream-Typen auf Quell-Aktivitäts-IDs.
// z.B. {"gps": "uuid-A", "hr": "uuid-B", "power": "uuid-B"}
type FusionSources map[string]string

// SetFusionSources setzt die fusion_sources einer Aktivität.
func (db *DB) SetFusionSources(ctx context.Context, activityID string, sources FusionSources) error {
	data, err := json.Marshal(sources)
	if err != nil {
		return fmt.Errorf("marshal fusion sources: %w", err)
	}
	_, err = db.Pool.Exec(ctx,
		`UPDATE activities SET fusion_sources = $2 WHERE id = $1`,
		activityID, data,
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
            AND ABS(EXTRACT(EPOCH FROM (b.started_at - a.started_at))) <= $3
            AND b.is_fusion_source = FALSE
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
		_ = rows.Scan(&id)
		candidateIDs = append(candidateIDs, id)
	}
	return candidateIDs, nil
}

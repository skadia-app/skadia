package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// DeveloperFieldInfo describes a single developer field key present in an activity's streams.
type DeveloperFieldInfo struct {
	FieldKey string `json:"field_key"`
	Units    string `json:"units,omitempty"`
	AppID    string `json:"app_id,omitempty"`
}

// GetDeveloperFieldsForActivity returns the distinct developer field keys
// actually present in the streams of the given activity, enriched with
// metadata (units, app name) from developer_field_definitions where possible.
func (db *DB) GetDeveloperFieldsForActivity(ctx context.Context, activityID string) ([]DeveloperFieldInfo, error) {
	rows, err := db.Pool.Query(ctx, `
		WITH stream_keys AS (
			SELECT DISTINCT jsonb_object_keys(developer_fields) AS field_key
			FROM (
				SELECT developer_fields
				FROM activity_streams
				WHERE activity_id = $1
				  AND developer_fields IS NOT NULL
				LIMIT 5000
			) sub
		)
		SELECT DISTINCT ON (sk.field_key)
			sk.field_key,
			COALESCE(dfd.units, '') AS units,
			COALESCE(encode(dfd.application_id, 'hex'), '') AS app_id
		FROM stream_keys sk
		LEFT JOIN developer_field_definitions dfd
			ON dfd.field_name = sk.field_key
		ORDER BY sk.field_key, dfd.units DESC NULLS LAST
	`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get developer fields for activity: %w", err)
	}
	defer rows.Close()

	var result []DeveloperFieldInfo
	for rows.Next() {
		var info DeveloperFieldInfo
		if err := rows.Scan(&info.FieldKey, &info.Units, &info.AppID); err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, rows.Err()
}

func (db *DB) UpsertDeveloperFieldDefs(ctx context.Context, defs []model.DeveloperFieldDef) error {
	if len(defs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, d := range defs {
		batch.Queue(`
			INSERT INTO developer_field_definitions (
				application_id, developer_data_index, field_definition_number,
				field_name, units, native_mesg_num, native_field_num,
				fit_base_type, scale, "offset"
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (application_id, developer_data_index, field_definition_number) DO NOTHING`,
			d.ApplicationID, d.DeveloperDataIndex, d.FieldDefinitionNumber,
			d.FieldName, d.Units, d.NativeMesgNum, d.NativeFieldNum,
			d.FitBaseType, d.Scale, d.Offset,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range defs {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("upsert developer field def: %w", err)
		}
	}
	return nil
}

package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

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

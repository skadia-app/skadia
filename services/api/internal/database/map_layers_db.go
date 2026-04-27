package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

// Sentinel errors for the map-layer DB layer. Callers MUST use errors.Is.
var (
	ErrSystemLayerImmutable    = errors.New("system layer immutable")
	ErrDuplicateName           = errors.New("duplicate name")
	ErrLimitReached            = errors.New("limit reached")
	ErrStaleUpdate             = errors.New("stale updated_at")
	ErrURLSourceKindMismatch   = errors.New("url_template/style_url inconsistent with source_kind")
	// ErrBulkSortItemNotFound is returned by BulkSort* when an item ID does not
	// match any row owned by the caller. The transaction is rolled back before return.
	ErrBulkSortItemNotFound    = errors.New("bulk sort item not found")
)

// SortItem is a (id, sort_order) tuple used by BulkSort* functions.
type SortItem struct {
	ID        int64 `json:"id"`
	SortOrder int32 `json:"sort_order"`
}

// ─── map_layers ──────────────────────────────────────────────────────────────

// ListMapLayers returns rows from map_layers ordered by (role, sort_order, id).
// When includeDisabled is false, only rows with enabled=true are returned —
// use false for the public authenticated endpoint, true for the admin endpoint.
func (db *DB) ListMapLayers(ctx context.Context, includeDisabled bool) ([]model.MapLayer, error) {
	query := `
		SELECT id, key, role, name, source_kind, url_template, style_url, attribution,
		       min_zoom, max_zoom, default_opacity, is_system, enabled, sort_order,
		       proxy_through_backend, requires_capability, created_at, updated_at
		FROM map_layers`
	if !includeDisabled {
		query += " WHERE enabled = true"
	}
	query += " ORDER BY role, sort_order, id"
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMapLayers(rows)
}

// InsertMapLayer inserts a new admin-created layer (is_system forced to false by the caller).
// On success, m.ID / m.CreatedAt / m.UpdatedAt are populated from RETURNING.
func (db *DB) InsertMapLayer(ctx context.Context, m *model.MapLayer) error {
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO map_layers (key, role, name, source_kind, url_template, style_url, attribution,
		                        min_zoom, max_zoom, default_opacity, is_system, enabled, sort_order,
		                        proxy_through_backend, requires_capability)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,false,$11,$12,$13,$14)
		RETURNING id, created_at, updated_at
	`,
		m.Key, m.Role, m.Name, m.SourceKind, m.URLTemplate, m.StyleURL, m.Attribution,
		m.MinZoom, m.MaxZoom, m.DefaultOpacity,
		m.Enabled, m.SortOrder, m.ProxyThroughBackend, m.RequiresCapability,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err, "map_layers_key_key") {
			return ErrDuplicateName
		}
		if isCheckViolation(err, "map_layers_url_consistency") {
			return ErrURLSourceKindMismatch
		}
		return err
	}
	return nil
}

// UpdateMapLayer applies a field-mask PATCH to one row inside a transaction.
//
// Concurrency: the row is locked with SELECT … FOR UPDATE before reading so
// that a concurrent PATCH cannot interleave between read and write.
//
// If ifMatchUpdatedAt is set and the current row's updated_at differs → ErrStaleUpdate.
// If the row has is_system=true and any key in patch is in model.SystemLayerImmutableFields
// → ErrSystemLayerImmutable.
// If the row does not exist → ErrNotFound.
func (db *DB) UpdateMapLayer(ctx context.Context, id int64, patch map[string]any, ifMatchUpdatedAt *time.Time) error {
	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var isSystem bool
	var currentUpdatedAt time.Time
	err = tx.QueryRow(ctx,
		`SELECT is_system, updated_at FROM map_layers WHERE id = $1 FOR UPDATE`, id,
	).Scan(&isSystem, &currentUpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if isSystem {
		for field := range patch {
			if _, blocked := model.SystemLayerImmutableFields[field]; blocked {
				return ErrSystemLayerImmutable
			}
		}
	}

	if ifMatchUpdatedAt != nil && !currentUpdatedAt.Equal(*ifMatchUpdatedAt) {
		return ErrStaleUpdate
	}

	setClauses := make([]string, 0, len(patch))
	args := make([]any, 0, len(patch)+1)
	i := 1
	for k, v := range patch {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", mapLayerPgIdent(k), i))
		args = append(args, v)
		i++
	}
	if len(setClauses) == 0 {
		return tx.Commit(ctx)
	}
	args = append(args, id)
	query := fmt.Sprintf("UPDATE map_layers SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), i)
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		if isUniqueViolation(err, "map_layers_key_key") {
			return ErrDuplicateName
		}
		if isCheckViolation(err, "map_layers_url_consistency") {
			return ErrURLSourceKindMismatch
		}
		return err
	}
	return tx.Commit(ctx)
}

// DeleteMapLayer deletes a non-system layer. Returns ErrSystemLayerImmutable for
// is_system=true rows and ErrNotFound if the id does not exist.
func (db *DB) DeleteMapLayer(ctx context.Context, id int64) error {
	var isSystem bool
	err := db.Pool.QueryRow(ctx, `SELECT is_system FROM map_layers WHERE id = $1`, id).Scan(&isSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if isSystem {
		return ErrSystemLayerImmutable
	}
	_, err = db.Pool.Exec(ctx, `DELETE FROM map_layers WHERE id = $1`, id)
	return err
}

// BulkSortMapLayers updates sort_order for a list of (id, sort_order) pairs in one
// SERIALIZABLE transaction to prevent torn-ordering when two admins drag-sort concurrently.
// Caller MUST retry on serialization failure (SQLSTATE 40001).
func (db *DB) BulkSortMapLayers(ctx context.Context, items []SortItem) error {
	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Lock all affected rows up-front so the snapshot is consistent across the loop.
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM map_layers WHERE id = ANY($1) FOR UPDATE`, ids); err != nil {
		return err
	}
	for _, it := range items {
		tag, err := tx.Exec(ctx,
			`UPDATE map_layers SET sort_order = $1 WHERE id = $2`, it.SortOrder, it.ID,
		)
		if err != nil {
			return err
		}
		// Reject silently-ignored IDs (unknown or not matched) so callers get 422
		// instead of a spurious 200 that hides a stale client payload.
		if tag.RowsAffected() != 1 {
			return ErrBulkSortItemNotFound
		}
	}
	return tx.Commit(ctx)
}

// ─── system_layer_visibility ─────────────────────────────────────────────────

// ListSystemLayerVisibility returns all visibility overrides ordered by layer_key.
func (db *DB) ListSystemLayerVisibility(ctx context.Context) ([]model.SystemLayerVisibility, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT layer_key, enabled, default_enabled_for_new_users, default_opacity, label_override, updated_at
		FROM system_layer_visibility
		ORDER BY layer_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.SystemLayerVisibility
	for rows.Next() {
		var v model.SystemLayerVisibility
		if err := rows.Scan(
			&v.LayerKey, &v.Enabled, &v.DefaultEnabledForNewUsers,
			&v.DefaultOpacity, &v.LabelOverride, &v.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateSystemLayerVisibility UPSERTs a single system_layer_visibility row.
// The patch map may contain any subset of: enabled, default_enabled_for_new_users,
// default_opacity, label_override. Unknown keys are rejected by the caller (handler),
// which also validates `key` against model.SystemLayerKeySet() — so the closed-set
// invariant holds before this function runs.
//
// Behavior: when a row for `key` exists, the patch fields are written via the
// ON CONFLICT DO UPDATE branch. When the row is missing (e.g. deleted
// out-of-band, partial down/up cycle), the INSERT branch creates it with the
// patch values inlined into the VALUES list — unpatched columns fall back to
// the migration 000333 column DEFAULTs. This way a fresh row created by an
// admin PATCH carries the patched value, not just the column default.
//
// An empty patch is a no-op and does not create a missing row — there is no
// observable change to apply, and silent row creation on an empty PATCH would
// be surprising.
func (db *DB) UpdateSystemLayerVisibility(ctx context.Context, key string, patch map[string]any) error {
	if len(patch) == 0 {
		return nil
	}
	// Build INSERT column list, VALUES placeholders, and the ON CONFLICT DO UPDATE
	// SET clause from a single ordered iteration so column/parameter ordering stays
	// in lock-step. EXCLUDED.<col> on the UPDATE branch references the row we
	// tried to insert, which keeps the SET clause one source-of-truth derived
	// from the patch keys.
	cols := make([]string, 0, len(patch)+1)
	placeholders := make([]string, 0, len(patch)+1)
	setClauses := make([]string, 0, len(patch))
	args := make([]any, 0, len(patch)+1)

	cols = append(cols, "layer_key")
	placeholders = append(placeholders, "$1")
	args = append(args, key)
	i := 2 // $1 is reserved for layer_key
	for k, v := range patch {
		col := sysVisibilityPgIdent(k)
		cols = append(cols, col)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		setClauses = append(setClauses, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
		args = append(args, v)
		i++
	}
	query := fmt.Sprintf(
		`INSERT INTO system_layer_visibility (%s) VALUES (%s)
		 ON CONFLICT (layer_key) DO UPDATE SET %s`,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(setClauses, ", "),
	)
	if _, err := db.Pool.Exec(ctx, query, args...); err != nil {
		return err
	}
	return nil
}

// ─── user_map_layers ─────────────────────────────────────────────────────────

// ListUserMapLayers returns all layers owned by userID, sorted by (sort_order, id).
// userID is a UUID string matching the users.id column.
func (db *DB) ListUserMapLayers(ctx context.Context, userID string) ([]model.UserMapLayer, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, name, url_template, attribution, min_zoom, max_zoom,
		       default_opacity, enabled, sort_order, created_at, updated_at
		FROM user_map_layers
		WHERE user_id = $1
		ORDER BY sort_order, id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUserMapLayers(rows)
}

// InsertUserMapLayer enforces the per-user limit (checked inside the transaction)
// before inserting. Returns ErrLimitReached when count >= limit, ErrDuplicateName
// on a case-insensitive name collision within the same user.
// m.UserID must be set by the caller before calling.
func (db *DB) InsertUserMapLayer(ctx context.Context, m *model.UserMapLayer, limit int) error {
	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var n int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_map_layers WHERE user_id = $1`, m.UserID,
	).Scan(&n); err != nil {
		return err
	}
	if n >= limit {
		return ErrLimitReached
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO user_map_layers (user_id, name, url_template, attribution, min_zoom, max_zoom,
		                             default_opacity, enabled, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at, updated_at
	`,
		m.UserID, m.Name, m.URLTemplate, m.Attribution, m.MinZoom, m.MaxZoom,
		m.DefaultOpacity, m.Enabled, m.SortOrder,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err, "user_map_layers_user_name") {
			return ErrDuplicateName
		}
		return err
	}
	return tx.Commit(ctx)
}

// UpdateUserMapLayer PATCHes a row owned by userID (user-isolation: WHERE user_id=$1 AND id=$2).
// The patch map accepts: name, url_template, attribution, min_zoom, max_zoom, default_opacity,
// enabled, sort_order. The caller must validate/sanitize before calling.
// Returns ErrNotFound when no row matches (non-existent or belongs to a different user).
func (db *DB) UpdateUserMapLayer(ctx context.Context, userID string, id int64, patch map[string]any) error {
	setClauses := make([]string, 0, len(patch))
	args := make([]any, 0, len(patch)+2)
	i := 1
	for k, v := range patch {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", userMapLayerPgIdent(k), i))
		args = append(args, v)
		i++
	}
	if len(setClauses) == 0 {
		return nil
	}
	args = append(args, userID, id)
	query := fmt.Sprintf("UPDATE user_map_layers SET %s WHERE user_id = $%d AND id = $%d",
		strings.Join(setClauses, ", "), i, i+1)
	tag, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		if isUniqueViolation(err, "user_map_layers_user_name") {
			return ErrDuplicateName
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUserMapLayer deletes a layer owned by userID (user-isolation: WHERE user_id=$1 AND id=$2).
// Returns ErrNotFound when no row matches.
func (db *DB) DeleteUserMapLayer(ctx context.Context, userID string, id int64) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM user_map_layers WHERE user_id = $1 AND id = $2`, userID, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// BulkSortUserMapLayers updates sort_order for a list of (id, sort_order) pairs
// belonging to userID. Uses SERIALIZABLE isolation (same as BulkSortMapLayers).
// Caller MUST retry on SQLSTATE 40001.
func (db *DB) BulkSortUserMapLayers(ctx context.Context, userID string, items []SortItem) error {
	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	// Lock only rows that belong to this user to prevent cross-user lock escalation.
	if _, err := tx.Exec(ctx,
		`SELECT id FROM user_map_layers WHERE user_id = $1 AND id = ANY($2) FOR UPDATE`,
		userID, ids,
	); err != nil {
		return err
	}
	for _, it := range items {
		tag, err := tx.Exec(ctx,
			`UPDATE user_map_layers SET sort_order = $1 WHERE user_id = $2 AND id = $3`,
			it.SortOrder, userID, it.ID,
		)
		if err != nil {
			return err
		}
		// Reject items that did not match the user's own rows — prevents silent no-op
		// when a client sends an ID belonging to a different user or that does not exist.
		if tag.RowsAffected() != 1 {
			return ErrBulkSortItemNotFound
		}
	}
	return tx.Commit(ctx)
}

// ─── scan helpers ────────────────────────────────────────────────────────────

func scanMapLayers(rows pgx.Rows) ([]model.MapLayer, error) {
	var out []model.MapLayer
	for rows.Next() {
		var m model.MapLayer
		if err := rows.Scan(
			&m.ID, &m.Key, &m.Role, &m.Name, &m.SourceKind,
			&m.URLTemplate, &m.StyleURL, &m.Attribution,
			&m.MinZoom, &m.MaxZoom, &m.DefaultOpacity,
			&m.IsSystem, &m.Enabled, &m.SortOrder,
			&m.ProxyThroughBackend, &m.RequiresCapability,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanUserMapLayers(rows pgx.Rows) ([]model.UserMapLayer, error) {
	var out []model.UserMapLayer
	for rows.Next() {
		var m model.UserMapLayer
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.Name, &m.URLTemplate, &m.Attribution,
			&m.MinZoom, &m.MaxZoom, &m.DefaultOpacity, &m.Enabled, &m.SortOrder,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ─── SQL-injection defense: column whitelists ─────────────────────────────────

// mapLayerPgIdent whitelists columns PATCHable on map_layers.
// key and source_kind are intentionally absent — they are immutable after creation
// and rejected by the handler before reaching this function.
// Panics on unknown fields — callers must validate before calling.
func mapLayerPgIdent(field string) string {
	allowed := map[string]string{
		"name":                "name",
		"enabled":             "enabled",
		"sort_order":          "sort_order",
		"default_opacity":     "default_opacity",
		"attribution":         "attribution",
		"url_template":        "url_template",
		"style_url":           "style_url",
		"min_zoom":            "min_zoom",
		"max_zoom":            "max_zoom",
		"requires_capability": "requires_capability",
	}
	if col, ok := allowed[field]; ok {
		return col
	}
	panic("unknown map_layers patch field: " + field)
}

// sysVisibilityPgIdent whitelists columns PATCHable on system_layer_visibility.
func sysVisibilityPgIdent(field string) string {
	allowed := map[string]string{
		"enabled":                      "enabled",
		"default_enabled_for_new_users": "default_enabled_for_new_users",
		"default_opacity":               "default_opacity",
		"label_override":                "label_override",
	}
	if col, ok := allowed[field]; ok {
		return col
	}
	panic("unknown system_layer_visibility patch field: " + field)
}

// userMapLayerPgIdent whitelists columns PATCHable on user_map_layers.
func userMapLayerPgIdent(field string) string {
	allowed := map[string]string{
		"name":            "name",
		"url_template":    "url_template",
		"attribution":     "attribution",
		"min_zoom":        "min_zoom",
		"max_zoom":        "max_zoom",
		"default_opacity": "default_opacity",
		"enabled":         "enabled",
		"sort_order":      "sort_order",
	}
	if col, ok := allowed[field]; ok {
		return col
	}
	panic("unknown user_map_layers patch field: " + field)
}

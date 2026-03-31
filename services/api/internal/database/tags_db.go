package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// tagSelectCols are the columns returned by tag queries.
const tagSelectCols = `id, user_id, name, color, text_color, category, is_system,
	is_auto, conditions, condition_groups, conditions_logic, description, activity_count, created_at,
	lat, lng, has_automation, cover_file_id`

// scanTag scans a tag row into a model.Tag.
func scanTag(row interface{ Scan(...any) error }) (model.Tag, error) {
	var t model.Tag
	err := row.Scan(
		&t.ID, &t.UserID, &t.Name, &t.Color, &t.TextColor,
		&t.Category, &t.IsSystem, &t.IsAuto, &t.Conditions,
		&t.ConditionGroups, &t.ConditionsLogic,
		&t.Description, &t.ActivityCount, &t.CreatedAt,
		&t.Lat, &t.Lng, &t.HasAutomation, &t.CoverFileID,
	)
	return t, err
}

// ListTags returns all tags for a user, ordered by activity_count DESC, name ASC.
func (db *DB) ListTags(ctx context.Context, userID string) ([]model.Tag, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+tagSelectCols+`
		FROM tags
		WHERE user_id = $1
		ORDER BY activity_count DESC, name ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	if tags == nil {
		tags = []model.Tag{}
	}

	// Enrich with zone data in a single query.
	zones, err := db.getTagZonesByUserID(ctx, userID)
	if err != nil {
		// Non-fatal: log and continue without zone data
		slog.Warn("list tags: failed to load zones", "error", err)
	} else {
		for i := range tags {
			if z, ok := zones[tags[i].ID]; ok {
				tags[i].Zone = z
			}
		}
	}

	// Append orphan tags: tag names in activities.tags TEXT[] with no tags table entry
	orphanRows, orphanErr := db.Pool.Query(ctx, `
		SELECT unnested.name, COUNT(DISTINCT a.id) AS activity_count
		FROM activities a
		CROSS JOIN LATERAL unnest(a.tags) AS unnested(name)
		WHERE a.user_id = $1
		  AND unnested.name != ''
		  AND NOT EXISTS (
		      SELECT 1 FROM tags t
		      WHERE t.user_id = $1 AND lower(t.name) = lower(unnested.name)
		  )
		GROUP BY unnested.name
		ORDER BY activity_count DESC, unnested.name ASC`,
		userID,
	)
	if orphanErr == nil {
		for orphanRows.Next() {
			var name string
			var count int
			if scanErr := orphanRows.Scan(&name, &count); scanErr == nil {
				tags = append(tags, model.Tag{
					ID:            "",
					UserID:        userID,
					Name:          name,
					Color:         "#9CA3AF",
					TextColor:     "#FFFFFF",
					Category:      "orphan",
					ActivityCount: count,
				})
			}
		}
		orphanRows.Close()
	}

	return tags, nil
}

// GetTagByID returns a single tag by ID, scoped to the given user.
func (db *DB) GetTagByID(ctx context.Context, tagID, userID string) (*model.Tag, error) {
	t, err := scanTag(db.Pool.QueryRow(ctx, `
		SELECT `+tagSelectCols+`
		FROM tags
		WHERE id = $1 AND user_id = $2`, tagID, userID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tag by id: %w", err)
	}
	return &t, nil
}

// GetTagActivityIDs returns the IDs of all activities associated with the given tag.
func (db *DB) GetTagActivityIDs(ctx context.Context, tagID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT activity_id FROM activity_tags WHERE tag_id = $1`, tagID,
	)
	if err != nil {
		return nil, fmt.Errorf("get tag activity ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan activity id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get tag activity ids: %w", err)
	}
	return ids, nil
}

// CreateTag creates a new tag for a user.
func (db *DB) CreateTag(ctx context.Context, userID, name, color, textColor string, isAuto bool, conditions, conditionGroups []byte, conditionsLogic string, description *string, lat, lng *float64) (*model.Tag, error) {
	if conditionsLogic == "" {
		conditionsLogic = "and"
	}
	category := "custom"
	if lat != nil && lng != nil {
		category = "standort"
	}
	t, err := scanTag(db.Pool.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, is_auto, conditions, condition_groups, conditions_logic, description, lat, lng, category)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+tagSelectCols,
		userID, name, color, textColor, isAuto, conditions, conditionGroups, conditionsLogic, description, lat, lng, category,
	))
	if err != nil {
		return nil, fmt.Errorf("create tag: %w", err)
	}
	return &t, nil
}

// UpdateTag updates an existing tag.
// lat and lng are treated as optional nullable fields:
//   - locationPresent=false → lat/lng columns not touched
//   - locationPresent=true, lat/lng=nil → SET lat=NULL, lng=NULL
//   - locationPresent=true, lat/lng=non-nil → SET lat=*lat, lng=*lng
func (db *DB) UpdateTag(ctx context.Context, tagID, userID, name, color, textColor string, isAuto *bool, conditions, conditionGroups *[]byte, conditionsLogic *string, description *string, lat, lng *float64, locationPresent bool) (*model.Tag, error) {
	b := newSetBuilder(3)

	if name != "" {
		b.Set("name", name)
	}
	if color != "" {
		b.Set("color", color)
	}
	if textColor != "" {
		b.Set("text_color", textColor)
	}
	if isAuto != nil {
		b.Set("is_auto", *isAuto)
	}
	if conditions != nil {
		b.Set("conditions", *conditions)
	}
	if conditionGroups != nil {
		b.Set("condition_groups", *conditionGroups)
	}
	if conditionsLogic != nil {
		b.Set("conditions_logic", *conditionsLogic)
	}
	if description != nil {
		b.Set("description", *description)
	}
	if locationPresent {
		// lat and lng are either both NULL or both a value — update both together.
		b.Set("lat", lat) // pgx treats nil *float64 as SQL NULL
		b.Set("lng", lng)
		// Auto-set category based on location
		if lat != nil {
			b.Set("category", "standort")
		} else {
			// lat removed → revert standort back to custom (only if it was standort)
			b.SetExpr("category = CASE WHEN category = 'standort' THEN 'custom' ELSE category END")
		}
	}

	args := append([]any{tagID, userID}, b.Args()...)

	if !b.HasSets() {
		t, err := scanTag(db.Pool.QueryRow(ctx,
			`SELECT `+tagSelectCols+` FROM tags WHERE id = $1 AND user_id = $2`,
			tagID, userID,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get tag: %w", err)
		}
		return &t, nil
	}

	query := fmt.Sprintf(`
		UPDATE tags SET %s
		WHERE id = $1 AND user_id = $2
		RETURNING `+tagSelectCols,
		b.Clause(),
	)

	t, err := scanTag(db.Pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update tag: %w", err)
	}
	return &t, nil
}

// ClearTagCoverFile sets cover_file_id to NULL.
func (db *DB) ClearTagCoverFile(ctx context.Context, tagID, userID string) (*model.Tag, error) {
	t, err := scanTag(db.Pool.QueryRow(ctx,
		`UPDATE tags
		 SET cover_file_id = NULL
		 WHERE id = $1 AND user_id = $2
		 RETURNING `+tagSelectCols,
		tagID, userID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clear tag cover file: %w", err)
	}
	return &t, nil
}

// DeleteTag deletes a tag.
func (db *DB) DeleteTag(ctx context.Context, tagID, userID string) error {
	result, err := db.Pool.Exec(ctx,
		`DELETE FROM tags WHERE id = $1 AND user_id = $2`,
		tagID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("tag not found")
	}
	return nil
}

// GetTagsByActivity returns all tags assigned to an activity via the activity_tags junction table.
func (db *DB) GetTagsByActivity(ctx context.Context, activityID string) ([]model.Tag, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT t.id, t.user_id, t.name, t.color, t.text_color, t.category, t.is_system,
			t.is_auto, t.conditions, t.condition_groups, t.conditions_logic, t.description, t.activity_count, t.created_at,
			t.lat, t.lng, t.has_automation, t.cover_file_id
		FROM tags t
		JOIN activity_tags at ON at.tag_id = t.id
		WHERE at.activity_id = $1
		ORDER BY t.name ASC`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get tags by activity: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get tags by activity: %w", err)
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	return tags, nil
}

// GetTagActivities returns a lightweight list of activities that have the given tag.
func (db *DB) GetTagActivities(ctx context.Context, tagID, userID string) ([]model.TagActivity, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT a.id, a.title, a.title_auto, a.started_at, a.sport_type, a.distance_m, a.timezone
		FROM activities a
		JOIN activity_tags at_join ON at_join.activity_id = a.id
		WHERE at_join.tag_id = $1 AND a.user_id = $2
		ORDER BY a.started_at DESC
		LIMIT 50
	`, tagID, userID)
	if err != nil {
		return nil, fmt.Errorf("get tag activities: %w", err)
	}
	defer rows.Close()
	var activities []model.TagActivity
	for rows.Next() {
		var a model.TagActivity
		if err := rows.Scan(&a.ID, &a.Title, &a.TitleAuto, &a.StartedAt, &a.SportType, &a.DistanceM, &a.Timezone); err != nil {
			return nil, fmt.Errorf("scan tag activity: %w", err)
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get tag activities: %w", err)
	}
	if activities == nil {
		activities = []model.TagActivity{}
	}
	return activities, nil
}

// SeedSystemDefaults creates the default system tags and automation rules for a new user.
func (db *DB) SeedSystemDefaults(ctx context.Context, userID string) error {
	nachtDesc := "Automatisch bei Aktivitaeten die in die astronomische Nacht fallen"
	frostDesc := "Automatisch bei Temperaturen unter 0 Grad C"

	// System tags (is_auto=false — now driven by automation rules, not the old autotag engine)
	seeds := []struct {
		name, color, textColor string
		desc                   string
	}{
		{"Nacht", "#1E293B", "#E2E8F0", nachtDesc},
		{"Frost", "#38BDF8", "#0C4A6E", frostDesc},
	}
	for _, s := range seeds {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
			VALUES ($1, $2, $3, $4, true, false, $5)
			ON CONFLICT (user_id, name) DO NOTHING`,
			userID, s.name, s.color, s.textColor, s.desc,
		); err != nil {
			return fmt.Errorf("seed %s tag: %w", s.name, err)
		}
	}

	// Rundtour — rule-managed (set_field + add_tag via automation rule)
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
		VALUES ($1, 'Rundtour', '#10B981', '#1a1a1a', true, false, 'Automatisch bei Aktivitaeten die als Rundtour erkannt wurden (Sync mit is_round_trip)')
		ON CONFLICT (user_id, name) DO UPDATE SET is_auto = false, conditions = NULL, condition_groups = NULL, conditions_logic = NULL`,
		userID,
	); err != nil {
		return fmt.Errorf("seed Rundtour tag: %w", err)
	}

	// Weather system tags
	weatherTags := []struct{ name, color string }{
		{"sonnig", "#FCD34D"},
		{"bewölkt", "#9CA3AF"},
		{"Regen", "#3B82F6"},
		{"Schnee", "#F3F4F6"},
		{"Nebel", "#D1D5DB"},
		{"Gewitter", "#7C3AED"},
	}
	weatherDesc := "Automatisch erkannt aus Wetterdaten"
	for _, w := range weatherTags {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
			VALUES ($1, $2, $3, $4, 'weather', true, false, $5)
			ON CONFLICT (user_id, name) DO NOTHING`,
			userID, w.name, w.color, contrastTextColor(w.color), weatherDesc,
		); err != nil {
			return fmt.Errorf("seed weather tag %s: %w", w.name, err)
		}
	}

	// E-Bike system tag (no conditions — synced via automation rule with is_electric column).
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
		VALUES ($1, 'E-Bike', '#16A34A', '#FFFFFF', true, false, 'Automatisch bei E-Bike-Aktivitäten (Sync mit is_electric)')
		ON CONFLICT (user_id, name) DO NOTHING`,
		userID,
	); err != nil {
		return fmt.Errorf("seed E-Bike tag: %w", err)
	}

	// Indoor system tag (synced via is_indoor column, analogous to E-Bike).
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
		VALUES ($1, 'Indoor', '#3B82F6', '#FFFFFF', true, false, 'Automatisch bei Indoor-Aktivitäten (Sync mit is_indoor)')
		ON CONFLICT (user_id, name) DO NOTHING`,
		userID,
	); err != nil {
		return fmt.Errorf("seed Indoor tag: %w", err)
	}

	// Migrate existing tags from activities.tags TEXT[] to tags table
	// This ensures all previously used tags appear in the Tags overview
	if err := db.migrateExistingTagsToTable(ctx, userID); err != nil {
		slog.Warn("seed: migrate existing tags failed (non-fatal)", "user_id", userID, "error", err)
	}

	// System automation rules for new users
	// (existing users get them via migration 000176)
	type tagRule struct {
		tagName    string
		systemType string
		priority   int
		conditions string
	}
	tagRules := []tagRule{
		{"Rundtour", "round_trip", -99, `[{"field":"start_end_distance_m","op":"lte","value":200}]`},
		{"Frost", "frost", -98, `[{"field":"min_temperature_c","op":"lte","value":0}]`},
		{"Nacht", "nacht", -97, `[{"field":"sun_elevation","op":"lte","value":-6}]`},
	}
	for _, tr := range tagRules {
		var tagID string
		err := db.Pool.QueryRow(ctx,
			`SELECT id FROM tags WHERE user_id = $1 AND name = $2 AND is_system = true`,
			userID, tr.tagName,
		).Scan(&tagID)
		if err != nil {
			continue
		}
		var actions string
		switch tr.tagName {
		case "Rundtour":
			actions = fmt.Sprintf(`[{"action":"add_tag","params":{"tag_id":"%s"}},{"action":"set_field","params":{"field":"is_round_trip","value":true}}]`, tagID)
		default:
			actions = fmt.Sprintf(`[{"action":"add_tag","params":{"tag_id":"%s"}}]`, tagID)
		}
		_, _ = db.Pool.Exec(ctx, `
			INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
			VALUES ($1, $2, true, $3, true, $4, $5::jsonb, 'and', $6::jsonb)
			ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING`,
			userID, tr.tagName, tr.systemType, tr.priority, tr.conditions, actions)
	}
	// E-Bike rule (no conditions/actions — handled by import pipeline)
	_, _ = db.Pool.Exec(ctx, `
		INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
		VALUES ($1, 'E-Bike', true, 'electric_sync', true, -100, '[]'::jsonb, 'and', '[]'::jsonb)
		ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING`,
		userID)

	// Indoor sync rule (no conditions/actions — handled by import pipeline + trigger)
	_, _ = db.Pool.Exec(ctx, `
		INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
		VALUES ($1, 'Indoor-Sync', true, 'indoor_sync', true, -100, '[]'::jsonb, 'and', '[]'::jsonb)
		ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING`,
		userID)

	// Virtual activity rule (no conditions/actions — synced via is_virtual column)
	_, _ = db.Pool.Exec(ctx, `
		INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
		VALUES ($1, 'virtuell', true, 'virtual_sync', true, -100, '[]'::jsonb, 'and', '[]'::jsonb)
		ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING`,
		userID)

	// Virtual system tag
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
		VALUES ($1, 'virtuell', '#8B5CF6', '#FFFFFF', true, false, 'Automatisch bei virtuellen Aktivitäten (Sync mit is_virtual)')
		ON CONFLICT (user_id, name) DO NOTHING`,
		userID,
	); err != nil {
		return fmt.Errorf("seed virtuell tag: %w", err)
	}

	return nil
}

// migrateExistingTagsToTable scans all activities.tags TEXT[] and creates missing entries in tags table.
// This ensures all previously-used tag names appear in the Tags overview.
func (db *DB) migrateExistingTagsToTable(ctx context.Context, userID string) error {
	// Collect all unique tag names from activities.tags TEXT[] for this user
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT UNNEST(tags) AS tag_name
		FROM activities
		WHERE user_id = $1 AND tags IS NOT NULL AND array_length(tags, 1) > 0
		ORDER BY tag_name
	`, userID)
	if err != nil {
		return fmt.Errorf("query existing tags: %w", err)
	}
	defer rows.Close()

	var tagNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan tag name: %w", err)
		}
		if name != "" {
			tagNames = append(tagNames, name)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate tag names: %w", err)
	}

	if len(tagNames) == 0 {
		return nil // No tags to migrate
	}

	// Color palette for auto-assigned colors to existing tags
	colorPalette := []struct{ color, text string }{
		{"#f97316", "#ffffff"}, // orange
		{"#3b82f6", "#ffffff"}, // blue
		{"#10b981", "#ffffff"}, // green
		{"#8b5cf6", "#ffffff"}, // purple
		{"#f43f5e", "#ffffff"}, // rose
		{"#06b6d4", "#ffffff"}, // cyan
		{"#eab308", "#1a1a1a"}, // yellow
		{"#6b7280", "#ffffff"}, // gray
	}

	// Insert each tag if it doesn't exist
	for i, tagName := range tagNames {
		// Assign color from palette (cycle if needed)
		color := colorPalette[i%len(colorPalette)].color
		textColor := colorPalette[i%len(colorPalette)].text

		// Determine if this is a known auto-tag
		isAuto := false
		category := "custom"
		if tagName == "Nacht" || tagName == "Frost" {
			isAuto = true
		}
		// Rundtour is rule-managed (system rule round_trip), so isAuto stays false
		if tagName == "sonnig" || tagName == "bewölkt" || tagName == "Regen" ||
			tagName == "Schnee" || tagName == "Nebel" || tagName == "Gewitter" {
			isAuto = false // Weather tags are not marked as auto (system flag)
			category = "weather"
		}

		_, err := db.Pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_auto, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
			ON CONFLICT (user_id, name) DO NOTHING
		`, userID, tagName, color, textColor, category, isAuto)
		if err != nil {
			slog.Warn("migrate tag to table", "user_id", userID, "tag_name", tagName, "error", err)
			// Continue with other tags instead of failing
		}
	}

	// Step 2: For EACH tag, find all activities with that tag in activities.tags TEXT[]
	// and create activity_tags junction entries.
	for _, tagName := range tagNames {
		// Get the tag ID that was just created/upserted
		var tagID string
		if err := db.Pool.QueryRow(ctx,
			"SELECT id FROM tags WHERE user_id = $1 AND name = $2",
			userID, tagName).Scan(&tagID); err != nil {
			slog.Warn("migrate tags: get tag id failed", "user_id", userID, "tag_name", tagName, "error", err)
			continue // Skip if tag not found
		}

		// Find all activities that have this tag in their tags TEXT[] array
		rows, err := db.Pool.Query(ctx,
			"SELECT id FROM activities WHERE user_id = $1 AND $2 = ANY(tags)",
			userID, tagName)
		if err != nil {
			slog.Warn("migrate tags: query activities failed", "user_id", userID, "tag_name", tagName, "error", err)
			continue
		}

		// For each activity, insert into activity_tags (skip duplicates via ON CONFLICT)
		for rows.Next() {
			var activityID string
			if err := rows.Scan(&activityID); err != nil {
				slog.Warn("migrate tags: scan activity id failed", "user_id", userID, "tag_name", tagName, "error", err)
				continue
			}

			if _, execErr := db.Pool.Exec(ctx,
				"INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES ($1, $2, 'migration') ON CONFLICT DO NOTHING",
				activityID, tagID); execErr != nil {
				slog.Warn("migrate tags: insert activity_tag failed", "activity_id", activityID, "tag_id", tagID, "error", execErr)
				// Continue with other activities
			}
		}
		rows.Close()
	}

	// Step 3: Update activity_count for all migrated tags
	if _, err := db.Pool.Exec(ctx, `
		UPDATE tags
		SET activity_count = (
			SELECT COUNT(DISTINCT activity_id)
			FROM activity_tags
			WHERE tag_id = tags.id
		)
		WHERE user_id = $1 AND category IN ('custom', 'weather')
	`, userID); err != nil {
		slog.Warn("migrate tags: update activity counts failed", "user_id", userID, "error", err)
		// Non-fatal, continue
	}

	return nil
}

// MergeTag merges sourceTagID into targetTagID in a single transaction:
//  1. All activity_tags rows pointing to sourceTagID → replaced by targetTagID (skip
//     if the activity already has targetTagID to avoid duplicate-key violation).
//  2. activities.tags TEXT[] cache: remove source tag name, add target tag name if
//     not already present.
//  3. The source tag is deleted.
//
// Returns the surviving (target) tag on success.
func (db *DB) MergeTag(ctx context.Context, targetTagID, sourceTagID, userID string) (*model.Tag, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Resolve names of both tags and verify ownership.
	var targetName, sourceName string
	if err := tx.QueryRow(ctx,
		`SELECT name FROM tags WHERE id = $1 AND user_id = $2`,
		targetTagID, userID,
	).Scan(&targetName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("target tag not found")
		}
		return nil, fmt.Errorf("get target tag: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT name FROM tags WHERE id = $1 AND user_id = $2`,
		sourceTagID, userID,
	).Scan(&sourceName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("source tag not found")
		}
		return nil, fmt.Errorf("get source tag: %w", err)
	}
	if targetTagID == sourceTagID {
		return nil, fmt.Errorf("target and source tag must differ")
	}

	// Block merge if source is a system tag
	var sourceIsSystem bool
	if err := tx.QueryRow(ctx,
		`SELECT is_system FROM tags WHERE id = $1 AND user_id = $2`,
		sourceTagID, userID,
	).Scan(&sourceIsSystem); err != nil {
		return nil, fmt.Errorf("check source system: %w", err)
	}
	if sourceIsSystem {
		return nil, fmt.Errorf("source_is_system")
	}

	// Block merge if source tag is referenced by an automation rule
	var hasRule bool
	tx.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM automation_rules
			WHERE user_id = $1 AND actions::text LIKE '%' || $2::text || '%'
		)`,
		userID, sourceTagID,
	).Scan(&hasRule) //nolint:errcheck
	if hasRule {
		return nil, fmt.Errorf("source_has_rule")
	}

	// 2. Move activity_tags rows: for activities that have source but not target,
	//    update the tag_id. For activities that already have both, just delete the
	//    source row (to avoid a unique constraint violation).
	_, err = tx.Exec(ctx, `
		UPDATE activity_tags
		SET tag_id = $1
		WHERE tag_id = $2
		  AND activity_id NOT IN (
		      SELECT activity_id FROM activity_tags WHERE tag_id = $1
		  )`,
		targetTagID, sourceTagID,
	)
	if err != nil {
		return nil, fmt.Errorf("migrate activity_tags: %w", err)
	}
	// Delete any remaining rows that still reference sourceTagID (those activities
	// already had targetTagID so the UPDATE above skipped them).
	_, err = tx.Exec(ctx,
		`DELETE FROM activity_tags WHERE tag_id = $1`,
		sourceTagID,
	)
	if err != nil {
		return nil, fmt.Errorf("delete orphan activity_tags: %w", err)
	}

	// Update activity_count for targetTagID (it now includes activities from sourceTagID)
	_, err = tx.Exec(ctx,
		`UPDATE tags
		 SET activity_count = (SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = $1)
		 WHERE id = $1`,
		targetTagID)
	if err != nil {
		return nil, fmt.Errorf("merge tags: update target count: %w", err)
	}

	// 3. Sync the denormalised activities.tags TEXT[] cache.
	//    Remove sourceName, add targetName (if not already present).
	_, err = tx.Exec(ctx, `
		UPDATE activities
		SET tags = array_append(
		              array_remove(tags, $1::text),
		              $2::text
		          )
		WHERE user_id = $3
		  AND $1 = ANY(tags)
		  AND NOT ($2 = ANY(tags))`,
		sourceName, targetName, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("update activities.tags (add target): %w", err)
	}
	// For activities that already had both names: just remove sourceName.
	_, err = tx.Exec(ctx, `
		UPDATE activities
		SET tags = array_remove(tags, $1::text)
		WHERE user_id = $2
		  AND $1 = ANY(tags)`,
		sourceName, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("update activities.tags (remove source): %w", err)
	}

	// 4a. Migrate activity_tag_removals from source to target.
	_, err = tx.Exec(ctx, `
		UPDATE activity_tag_removals SET tag_id = $1
		WHERE tag_id = $2
		  AND activity_id NOT IN (SELECT activity_id FROM activity_tag_removals WHERE tag_id = $1)`,
		targetTagID, sourceTagID)
	if err != nil {
		return nil, fmt.Errorf("migrate removals: %w", err)
	}
	_, err = tx.Exec(ctx,
		`DELETE FROM activity_tag_removals WHERE tag_id = $1`, sourceTagID)
	if err != nil {
		return nil, fmt.Errorf("delete stale removals: %w", err)
	}

	// 4b. Delete the source tag.
	_, err = tx.Exec(ctx,
		`DELETE FROM tags WHERE id = $1 AND user_id = $2`,
		sourceTagID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("delete source tag: %w", err)
	}

	// 5. Return the surviving target tag (with refreshed activity_count).
	t, err := scanTag(tx.QueryRow(ctx,
		`SELECT `+tagSelectCols+` FROM tags WHERE id = $1`,
		targetTagID,
	))
	if err != nil {
		return nil, fmt.Errorf("get merged tag: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit merge: %w", err)
	}
	return &t, nil
}

// DeleteOrphanTag removes a tag name from all activities.tags TEXT[] for a user.
// Used to clean up orphan tag names that have no corresponding tags table entry.
func (db *DB) DeleteOrphanTag(ctx context.Context, userID, tagName string) (int64, error) {
	result, err := db.Pool.Exec(ctx, `
		UPDATE activities
		SET tags = array_remove(tags, $2)
		WHERE user_id = $1 AND $2 = ANY(tags)`,
		userID, tagName,
	)
	if err != nil {
		return 0, fmt.Errorf("delete orphan tag: %w", err)
	}
	return result.RowsAffected(), nil
}

// GetTagIDsByNamesAndCategory returns a map of tag names to tag IDs
// for tags matching the given user, category, and names (case-insensitive).
func (db *DB) GetTagIDsByNamesAndCategory(ctx context.Context, userID, category string, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return make(map[string]string), nil
	}

	rows, err := db.Pool.Query(ctx,
		`SELECT id, name FROM tags WHERE user_id = $1 AND category = $2 AND name ILIKE ANY($3)`,
		userID, category, names)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		result[name] = id
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return result, nil
}

// AssignActivitiesToTag links multiple activities to a tag (manual assignment).
// Inserts into activity_tags junction table and syncs activities.tags TEXT[] cache.
func (db *DB) AssignActivitiesToTag(ctx context.Context, tagID, userID string, activityIDs []string) error {
	if len(activityIDs) == 0 {
		return nil
	}
	// Resolve tag name for updating the denormalized TEXT[] cache.
	var tagName string
	if err := db.Pool.QueryRow(ctx,
		`SELECT name FROM tags WHERE id = $1 AND user_id = $2`, tagID, userID,
	).Scan(&tagName); err != nil {
		return fmt.Errorf("assign activities to tag: resolve tag name: %w", err)
	}
	for _, actID := range activityIDs {
		if err := db.LinkActivityTag(ctx, actID, tagID, "manual"); err != nil {
			return fmt.Errorf("assign activities to tag: link %s: %w", actID, err)
		}
		// Keep denormalized TEXT[] cache in sync.
		// COALESCE guards against NULL tags array: NOT ($2 = ANY(NULL)) → NULL → FALSE, skips update.
		_, _ = db.Pool.Exec(ctx,
			`UPDATE activities SET tags = array_append(COALESCE(tags, '{}'), $2)
			 WHERE id = $1 AND user_id = $3 AND NOT ($2 = ANY(COALESCE(tags, '{}')))`,
			actID, tagName, userID)
	}
	return nil
}

// UnassignActivitiesFromTag removes multiple activities from a tag.
// Deletes from activity_tags junction table and syncs activities.tags TEXT[] cache.
func (db *DB) UnassignActivitiesFromTag(ctx context.Context, tagID, userID string, activityIDs []string) error {
	if len(activityIDs) == 0 {
		return nil
	}
	// Resolve tag name for updating the denormalized TEXT[] cache.
	var tagName string
	if err := db.Pool.QueryRow(ctx,
		`SELECT name FROM tags WHERE id = $1 AND user_id = $2`, tagID, userID,
	).Scan(&tagName); err != nil {
		return fmt.Errorf("unassign activities from tag: resolve tag name: %w", err)
	}
	for _, actID := range activityIDs {
		_, err := db.Pool.Exec(ctx,
			`DELETE FROM activity_tags WHERE activity_id = $1 AND tag_id = $2`,
			actID, tagID)
		if err != nil {
			return fmt.Errorf("unassign activities from tag: delete %s: %w", actID, err)
		}
		// Keep denormalized TEXT[] cache in sync.
		_, _ = db.Pool.Exec(ctx,
			`UPDATE activities SET tags = array_remove(tags, $2)
			 WHERE id = $1 AND user_id = $3`,
			actID, tagName, userID)
	}

	// Update activity_count from junction table
	_, err := db.Pool.Exec(ctx,
		`UPDATE tags
		 SET activity_count = (SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = $1)
		 WHERE id = $1`,
		tagID)
	if err != nil {
		return fmt.Errorf("unassign activities from tag: update count: %w", err)
	}
	return nil
}

// LinkActivityTag links a tag to an activity with the given assignment type.
// If the link already exists, it's silently ignored (ON CONFLICT DO NOTHING).
// Also updates the tag's activity_count denormalized field.
func (db *DB) LinkActivityTag(ctx context.Context, activityID, tagID, assignedBy string) error {
	// Insert link with ON CONFLICT DO NOTHING
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES ($1, $2, $3)
		 ON CONFLICT (activity_id, tag_id) DO NOTHING`,
		activityID, tagID, assignedBy)
	if err != nil {
		return fmt.Errorf("link activity tag: %w", err)
	}

	// Update activity_count from junction table
	_, err = db.Pool.Exec(ctx,
		`UPDATE tags
		 SET activity_count = (SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = $1)
		 WHERE id = $1`,
		tagID)
	if err != nil {
		return fmt.Errorf("link activity tag: update count: %w", err)
	}
	return nil
}

// RecordTagRemoval records that a user explicitly removed a system tag from an activity.
// This prevents enrichment from re-adding the tag later.
func (db *DB) RecordTagRemoval(ctx context.Context, activityID, tagID string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_tag_removals (activity_id, tag_id)
		 VALUES ($1, $2)
		 ON CONFLICT (activity_id, tag_id) DO NOTHING`,
		activityID, tagID)
	return err
}

// ClearTagRemoval deletes a removal record, allowing enrichment to re-add the tag.
// Called when a user manually re-assigns a system tag to an activity.
func (db *DB) ClearTagRemoval(ctx context.Context, activityID, tagID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM activity_tag_removals WHERE activity_id = $1 AND tag_id = $2`,
		activityID, tagID)
	return err
}

// IsTagRemovalRecorded checks whether a user has explicitly removed a system tag from an activity.
func (db *DB) IsTagRemovalRecorded(ctx context.Context, activityID, tagID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activity_tag_removals WHERE activity_id = $1 AND tag_id = $2)`,
		activityID, tagID).Scan(&exists)
	return exists, err
}

// contrastTextColor returns a readable text color (#1a1a1a or #ffffff) for a given
// hex background color using the YIQ luminance formula, matching the frontend logic.
func contrastTextColor(hex string) string {
	if len(hex) != 7 || hex[0] != '#' {
		return "#ffffff"
	}
	r, errR := strconv.ParseInt(hex[1:3], 16, 32)
	g, errG := strconv.ParseInt(hex[3:5], 16, 32)
	b, errB := strconv.ParseInt(hex[5:7], 16, 32)
	if errR != nil || errG != nil || errB != nil {
		return "#ffffff"
	}
	luminance := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255
	if luminance > 0.5 {
		return "#1a1a1a"
	}
	return "#ffffff"
}

package activitytitle

// DisplayTitleSQL returns the SQL expression for Skadia's display-title precedence.
func DisplayTitleSQL(alias string) string {
	title := qualifyActivityTitleColumn(alias, "title")
	titleAuto := qualifyActivityTitleColumn(alias, "title_auto")
	titleIsManual := qualifyActivityTitleColumn(alias, "title_is_manual")
	userID := qualifyActivityTitleColumn(alias, "user_id")
	trimmedTitle := "NULLIF(BTRIM(" + title + "), '')"
	trimmedTitleAuto := "NULLIF(BTRIM(" + titleAuto + "), '')"
	enableAutoNaming := "(SELECT ues.enable_auto_naming FROM user_enrichment_settings ues WHERE ues.user_id = " + userID + ")"
	displayPolicy := "(SELECT ues.auto_title_display_policy FROM user_enrichment_settings ues WHERE ues.user_id = " + userID + ")"

	return `COALESCE(
		CASE
			WHEN COALESCE(` + titleIsManual + `, false) AND ` + trimmedTitle + ` IS NOT NULL THEN ` + trimmedTitle + `
			WHEN NOT COALESCE(` + enableAutoNaming + `, true) THEN ` + trimmedTitle + `
			WHEN COALESCE(` + displayPolicy + `, 'source_unless_generic') = 'auto_first' THEN COALESCE(` + trimmedTitleAuto + `, ` + trimmedTitle + `)
			WHEN COALESCE(` + displayPolicy + `, 'source_unless_generic') = 'source_first' THEN COALESCE(` + trimmedTitle + `, ` + trimmedTitleAuto + `)
			WHEN ` + trimmedTitle + ` IS NULL OR (` + GenericSourceTitleSQL(trimmedTitle) + `) THEN COALESCE(` + trimmedTitleAuto + `, ` + trimmedTitle + `)
			ELSE ` + trimmedTitle + `
		END,
		''
	)`
}

func qualifyActivityTitleColumn(alias, column string) string {
	if alias == "" {
		return "activities." + column
	}
	return alias + "." + column
}

func GenericSourceTitleSQL(_ string) string {
	// Placeholder for a future language-aware classifier. No provider default titles are suppressed today.
	return "false"
}

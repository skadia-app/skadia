package database

// ActivityDisplayTitleSQL returns the SQL expression for Skadia's display-title precedence.
func ActivityDisplayTitleSQL(alias string) string {
	title := qualifyActivityTitleColumn(alias, "title")
	titleAuto := qualifyActivityTitleColumn(alias, "title_auto")
	titleIsManual := qualifyActivityTitleColumn(alias, "title_is_manual")

	return `COALESCE(
		CASE
			WHEN COALESCE(` + titleIsManual + `, false) AND NULLIF(` + title + `, '') IS NOT NULL THEN ` + title + `
			ELSE COALESCE(NULLIF(` + titleAuto + `, ''), NULLIF(` + title + `, ''))
		END,
		''
	)`
}

func qualifyActivityTitleColumn(alias, column string) string {
	if alias == "" {
		return column
	}
	return alias + "." + column
}

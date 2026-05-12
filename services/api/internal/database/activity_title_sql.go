package database

// ActivityDisplayTitleSQL returns the SQL expression for Skadia's display-title precedence.
func ActivityDisplayTitleSQL(alias string) string {
	title := qualifyActivityTitleColumn(alias, "title")
	titleAuto := qualifyActivityTitleColumn(alias, "title_auto")
	titleIsManual := qualifyActivityTitleColumn(alias, "title_is_manual")
	trimmedTitle := "NULLIF(BTRIM(" + title + "), '')"
	trimmedTitleAuto := "NULLIF(BTRIM(" + titleAuto + "), '')"

	return `COALESCE(
		CASE
			WHEN COALESCE(` + titleIsManual + `, false) AND ` + trimmedTitle + ` IS NOT NULL THEN ` + trimmedTitle + `
			ELSE COALESCE(` + trimmedTitleAuto + `, ` + trimmedTitle + `)
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

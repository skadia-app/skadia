package database

import "github.com/skadia-app/skadia/services/api/internal/activitytitle"

// ActivityDisplayTitleSQL returns the SQL expression for Skadia's display-title precedence.
func ActivityDisplayTitleSQL(alias string) string {
	return activitytitle.DisplayTitleSQL(alias)
}

func ActivityGenericSourceTitleSQL(sqlExpression string) string {
	return activitytitle.GenericSourceTitleSQL(sqlExpression)
}

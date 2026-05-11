package automation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const ruleWarningGearReferenceInvalid = "gear_reference_invalid"

func warnSkippedGearAction(ctx context.Context, db *database.DB, rule model.AutomationRule, userID, gearID, reason string) {
	msg := fmt.Sprintf("Ausrüstung %s wurde übersprungen: %s", gearID, reason)
	if err := db.SetRuleWarning(ctx, rule.ID, userID, ruleWarningGearReferenceInvalid, msg); err != nil {
		slog.Warn("set rule warning", "rule_id", rule.ID, "error", err)
	}
}

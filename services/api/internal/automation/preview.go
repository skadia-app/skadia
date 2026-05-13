package automation

import (
	"context"
	"encoding/json"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

type RulePreviewMatch struct {
	ActivityID  string          `json:"activity_id"`
	BranchIndex *int            `json:"branch_index,omitempty"`
	Actions     json.RawMessage `json:"actions"`
}

func PreviewRuleMatches(ctx context.Context, db *database.DB, userID string, rule model.AutomationRule, activityIDs []string) ([]RulePreviewMatch, error) {
	actMaps := make(map[string]map[string]any, len(activityIDs))
	for _, activityID := range activityIDs {
		owned, err := db.ActivityOwnedBy(ctx, activityID, userID)
		if err != nil {
			return nil, err
		}
		if !owned {
			continue
		}
		actMap, err := db.GetActivityAsMap(ctx, activityID)
		if err != nil {
			return nil, err
		}
		if actMap != nil {
			actMaps[activityID] = actMap
		}
	}
	InjectTrackGeofenceResultsBatch(ctx, db, activityIDs, actMaps, []model.AutomationRule{rule})

	matches := make([]RulePreviewMatch, 0)
	for _, activityID := range activityIDs {
		actMap := actMaps[activityID]
		if actMap == nil {
			continue
		}
		matched, branchIndex, actions := previewRuleAgainstActivity(rule, actMap)
		if !matched {
			continue
		}
		matches = append(matches, RulePreviewMatch{
			ActivityID:  activityID,
			BranchIndex: branchIndex,
			Actions:     actions,
		})
	}
	return matches, nil
}

func previewRuleAgainstActivity(rule model.AutomationRule, actMap map[string]any) (bool, *int, json.RawMessage) {
	if rule.HasBranches() {
		branches := rules.ParseBranches(rule.Branches)
		for i, br := range branches {
			branchMatched := false
			if br.Type == "else" {
				if i > 0 {
					branchMatched = true
				}
			} else if len(br.ConditionGroups) > 0 {
				branchMatched = rules.EvaluateGroups(br.ConditionGroups, actMap)
			}
			if branchMatched {
				idx := i
				return true, &idx, br.Actions
			}
		}
		return false, nil, nil
	}
	if !EvaluateRule(rule, actMap) {
		return false, nil, nil
	}
	return true, nil, rule.Actions
}

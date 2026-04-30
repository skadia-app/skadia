package dataimport

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/rules"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func normalizeSportTaxonomyImportRow(table string, row map[string]any, targetCols []targetColumn) (map[string]any, error) {
	switch table {
	case "activities":
		if err := normalizeActivitySportFields(row); err != nil {
			return nil, err
		}
	case "planned_routes":
		if patch, err := normalizeSportPair(row, "sport_type", "sub_type"); err != nil {
			return nil, err
		} else {
			applyModifierPatchToRow(row, patch)
		}
	case "routing_profiles":
		if err := normalizeRoutingProfileSportFields(row, targetCols); err != nil {
			return nil, err
		}
	case "automation_rules":
		if err := normalizeAutomationRuleSportJSON(row); err != nil {
			return nil, err
		}
	case "segments":
		if err := normalizeSegmentSportFields(row); err != nil {
			return nil, err
		}
	case "recording_groups":
		if patch, err := normalizeSportPair(row, "sport_type", "sub_type"); err != nil {
			return nil, err
		} else {
			applyModifierPatchToRow(row, patch)
		}
	}
	return row, nil
}

func normalizeActivitySportFields(row map[string]any) error {
	if patch, err := normalizeSportPair(row, "sport_type", "sub_type"); err != nil {
		return err
	} else {
		applyModifierPatchToRow(row, patch)
	}
	if patch, err := normalizeSportPair(row, "parsed_sport_type", "parsed_sub_type"); err != nil {
		return err
	} else {
		applyModifierPatchToRow(row, patch)
	}
	return nil
}

func normalizeSportPair(row map[string]any, sportKey, subTypeKey string) (sport.ModifierPatch, error) {
	rawSport, ok := stringValue(row[sportKey])
	if !ok || strings.TrimSpace(rawSport) == "" {
		return sport.ModifierPatch{}, nil
	}
	rawSubType, _ := stringValue(row[subTypeKey])
	tuple, patch, err := sport.NormalizeSportTuple(rawSport, rawSubType)
	if err != nil {
		return sport.ModifierPatch{}, err
	}
	row[sportKey] = tuple.SportType
	if tuple.SubType != "" {
		row[subTypeKey] = tuple.SubType
	} else if _, exists := row[subTypeKey]; exists {
		row[subTypeKey] = nil
	}
	return patch, nil
}

func normalizeSingleSportType(row map[string]any, sportKey string) error {
	rawSport, ok := stringValue(row[sportKey])
	if !ok || strings.TrimSpace(rawSport) == "" {
		return nil
	}
	tuple, _, err := sport.NormalizeSportTuple(rawSport, "")
	if err != nil {
		return err
	}
	row[sportKey] = tuple.SportType
	return nil
}

func applyModifierPatchToRow(row map[string]any, patch sport.ModifierPatch) {
	if patch.IsElectric != nil && *patch.IsElectric && !boolValue(row["is_electric"]) {
		row["is_electric"] = true
	}
	if patch.IsIndoor != nil && *patch.IsIndoor && !boolValue(row["is_indoor"]) {
		row["is_indoor"] = true
	}
}

func mergeImportModifierPatch(a, b sport.ModifierPatch) sport.ModifierPatch {
	if b.IsElectric != nil {
		a.IsElectric = b.IsElectric
	}
	if b.IsIndoor != nil {
		a.IsIndoor = b.IsIndoor
	}
	return a
}

func applyPatchToModifierFilters(filters map[string]any, patch sport.ModifierPatch) {
	if patch.IsElectric != nil {
		filters["is_electric"] = *patch.IsElectric
	}
	if patch.IsIndoor != nil {
		filters["is_indoor"] = *patch.IsIndoor
	}
}

func modifierFilterMap(value any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[key] = val
		}
		return out, nil
	case json.RawMessage, []byte, string:
		raw, _, err := rawMessageFromRowValue(v)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
			return map[string]any{}, nil
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		if out == nil {
			out = map[string]any{}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected object, got %T", value)
	}
}

func normalizeRoutingProfileSportFields(row map[string]any, targetCols []targetColumn) error {
	var modifierPatch sport.ModifierPatch
	if predicatesValue, ok := row["predicates"]; ok && predicatesValue != nil {
		predicates, patch, err := normalizePredicateList(predicatesValue)
		if err != nil {
			return fmt.Errorf("predicates: %w", err)
		}
		if len(predicates) == 0 {
			return fmt.Errorf("predicates: empty predicate list is not allowed")
		}
		row["predicates"] = predicates
		modifierPatch = mergeImportModifierPatch(modifierPatch, patch)
	} else if targetHasColumn(targetCols, "predicates") {
		sportTypes, err := stringList(row["sport_types"])
		if err != nil {
			return fmt.Errorf("sport_types: %w", err)
		}
		predicates, patch, err := predicatesFromSportTypes(sportTypes)
		if err != nil {
			return err
		}
		if len(predicates) == 0 {
			return fmt.Errorf("routing_profiles: empty predicate list is not allowed")
		}
		row["predicates"] = predicates
		modifierPatch = mergeImportModifierPatch(modifierPatch, patch)
	}

	if sportTypes, err := stringList(row["sport_types"]); err == nil && len(sportTypes) > 0 {
		canonical := make([]any, 0, len(sportTypes))
		seen := map[string]bool{}
		for _, rawSport := range sportTypes {
			tuple, _, err := sport.NormalizeSportTuple(rawSport, "")
			if err != nil {
				return fmt.Errorf("sport_types: %w", err)
			}
			if !seen[tuple.SportType] {
				seen[tuple.SportType] = true
				canonical = append(canonical, tuple.SportType)
			}
		}
		row["sport_types"] = canonical
	}
	if targetHasColumn(targetCols, "modifier_filters") {
		filters, err := modifierFilterMap(row["modifier_filters"])
		if err != nil {
			return fmt.Errorf("modifier_filters: %w", err)
		}
		applyPatchToModifierFilters(filters, modifierPatch)
		row["modifier_filters"] = filters
	}
	return nil
}

func predicatesFromSportTypes(sportTypes []string) ([]any, sport.ModifierPatch, error) {
	predicates := make([]any, 0, len(sportTypes))
	var modifierPatch sport.ModifierPatch
	for _, rawSport := range sportTypes {
		tuple, patch, err := sport.NormalizeSportTuple(rawSport, "")
		if err != nil {
			return nil, sport.ModifierPatch{}, fmt.Errorf("sport_types: %w", err)
		}
		predicate := map[string]any{"sport": tuple.SportType}
		if tuple.SubType != "" {
			predicate["sub_type"] = tuple.SubType
		}
		modifierPatch = mergeImportModifierPatch(modifierPatch, patch)
		predicates = append(predicates, predicate)
	}
	return predicates, modifierPatch, nil
}

func normalizePredicateList(value any) ([]any, sport.ModifierPatch, error) {
	items, err := objectList(value)
	if err != nil {
		return nil, sport.ModifierPatch{}, err
	}
	out := make([]any, 0, len(items))
	var modifierPatch sport.ModifierPatch
	for _, predicate := range items {
		rawSport, ok := stringValue(predicate["sport"])
		if !ok || strings.TrimSpace(rawSport) == "" {
			return nil, sport.ModifierPatch{}, fmt.Errorf("predicate sport must be a string")
		}
		rawSubType, _ := stringValue(predicate["sub_type"])
		tuple, patch, err := sport.NormalizeSportTuple(rawSport, rawSubType)
		if err != nil {
			return nil, sport.ModifierPatch{}, err
		}
		normalized := map[string]any{"sport": tuple.SportType}
		if tuple.SubType != "" {
			normalized["sub_type"] = tuple.SubType
		} else if _, ok := predicate["sub_type"]; ok {
			normalized["sub_type"] = ""
		}
		modifierPatch = mergeImportModifierPatch(modifierPatch, patch)
		out = append(out, normalized)
	}
	return out, modifierPatch, nil
}

func normalizeAutomationRuleSportJSON(row map[string]any) error {
	input := rules.SportTaxonomyRuleJSON{}
	if raw, ok, err := rawMessageFromRowValue(row["conditions"]); err != nil {
		return fmt.Errorf("conditions: %w", err)
	} else if ok {
		input.Conditions = raw
	}
	if raw, ok, err := rawMessageFromRowValue(row["condition_groups"]); err != nil {
		return fmt.Errorf("condition_groups: %w", err)
	} else if ok {
		input.ConditionGroups = raw
	}
	if raw, ok, err := rawMessageFromRowValue(row["branches"]); err != nil {
		return fmt.Errorf("branches: %w", err)
	} else if ok {
		input.Branches = raw
	}
	if raw, ok, err := rawMessageFromRowValue(row["actions"]); err != nil {
		return fmt.Errorf("actions: %w", err)
	} else if ok {
		input.Actions = raw
	}

	rewritten, _, err := rules.RewriteSportTaxonomyRuleJSON(input)
	if err != nil {
		return err
	}
	if input.Conditions != nil {
		if err := replaceRuleJSONValue(row, "conditions", rewritten.Conditions); err != nil {
			return fmt.Errorf("conditions: %w", err)
		}
	}
	if input.ConditionGroups != nil {
		if err := replaceRuleJSONValue(row, "condition_groups", rewritten.ConditionGroups); err != nil {
			return fmt.Errorf("condition_groups: %w", err)
		}
	}
	if input.Branches != nil {
		if err := replaceRuleJSONValue(row, "branches", rewritten.Branches); err != nil {
			return fmt.Errorf("branches: %w", err)
		}
	}
	if input.Actions != nil {
		if err := replaceRuleJSONValue(row, "actions", rewritten.Actions); err != nil {
			return fmt.Errorf("actions: %w", err)
		}
	}
	return nil
}

func replaceRuleJSONValue(row map[string]any, key string, raw json.RawMessage) error {
	switch row[key].(type) {
	case json.RawMessage, []byte, string:
		row[key] = raw
		return nil
	default:
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		row[key] = decoded
		return nil
	}
}

func normalizeSegmentSportFields(row map[string]any) error {
	value, ok := row["allowed_predicates"]
	if !ok || value == nil {
		predicates, err := predicatesFromSegmentLegacyArrays(row)
		if err != nil {
			return err
		}
		if predicates != nil {
			row["allowed_predicates"] = predicates
			delete(row, "allowed_sport_types")
			delete(row, "allowed_sub_types")
		}
		return nil
	}
	predicates, _, err := normalizePredicateList(value)
	if err != nil {
		return fmt.Errorf("allowed_predicates: %w", err)
	}
	row["allowed_predicates"] = predicates
	return nil
}

func predicatesFromSegmentLegacyArrays(row map[string]any) ([]any, error) {
	if _, ok := row["allowed_sport_types"]; !ok {
		return nil, nil
	}
	sportTypes, err := stringList(row["allowed_sport_types"])
	if err != nil {
		return nil, fmt.Errorf("allowed_sport_types: %w", err)
	}
	subTypes, err := stringList(row["allowed_sub_types"])
	if err != nil {
		return nil, fmt.Errorf("allowed_sub_types: %w", err)
	}
	if sportTypes == nil {
		return nil, nil
	}
	if len(sportTypes) == 0 {
		return []any{}, nil
	}

	predicates := make([]any, 0, len(sportTypes))
	seen := map[string]bool{}
	for i, rawSport := range sportTypes {
		rawSubType := ""
		if i < len(subTypes) {
			rawSubType = subTypes[i]
		}
		tuple, _, err := sport.NormalizeSportTuple(rawSport, rawSubType)
		if err != nil {
			return nil, fmt.Errorf("allowed_sport_types[%d]: %w", i, err)
		}
		key := tuple.SportType + "\x00" + tuple.SubType
		if seen[key] {
			continue
		}
		seen[key] = true
		predicate := map[string]any{"sport": tuple.SportType}
		if tuple.SubType != "" {
			predicate["sub_type"] = tuple.SubType
		}
		predicates = append(predicates, predicate)
	}
	return predicates, nil
}

func targetHasColumn(cols []targetColumn, name string) bool {
	for _, col := range cols {
		if col.Name == name {
			return true
		}
	}
	return false
}

func augmentInsertColumnsForRow(insertCols []string, geometryCols map[string]bool, targetCols []targetColumn, row map[string]any) ([]string, map[string]bool) {
	existing := make(map[string]bool, len(insertCols))
	for _, col := range insertCols {
		existing[col] = true
	}
	for key := range row {
		if existing[key] {
			continue
		}
		target, ok := targetColumnByName(targetCols, key)
		if !ok {
			continue
		}
		insertCols = append(insertCols, key)
		existing[key] = true
		if geometryCols == nil {
			geometryCols = map[string]bool{}
		}
		if target.UDTName == "geometry" {
			geometryCols[key] = true
		}
	}
	return insertCols, geometryCols
}

func targetColumnByName(cols []targetColumn, name string) (targetColumn, bool) {
	for _, col := range cols {
		if col.Name == name {
			return col, true
		}
	}
	return targetColumn{}, false
}

func stringValue(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case json.Number:
		return v.String(), true
	default:
		return "", false
	}
}

func boolValue(value any) bool {
	v, ok := value.(bool)
	return ok && v
}

func stringList(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := stringValue(item)
			if !ok {
				return nil, fmt.Errorf("expected string list item, got %T", item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected string list, got %T", value)
	}
}

func objectList(value any) ([]map[string]any, error) {
	if raw, ok, err := rawMessageFromRowValue(value); err != nil {
		return nil, err
	} else if ok {
		var out []map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expected object list, got %T", value)
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object list item, got %T", item)
		}
		out = append(out, obj)
	}
	return out, nil
}

func rawMessageFromRowValue(value any) (json.RawMessage, bool, error) {
	if value == nil {
		return nil, false, nil
	}
	switch v := value.(type) {
	case json.RawMessage:
		return v, true, nil
	case []byte:
		return json.RawMessage(v), true, nil
	case string:
		return json.RawMessage(v), true, nil
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, false, err
		}
		return raw, true, nil
	}
}

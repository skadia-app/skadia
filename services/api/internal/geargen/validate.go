package geargen

import (
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/applicablemodifiers"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// MaxSubfieldDepth is the maximum allowed nesting depth for Attribute.Subfields.
// A root attribute counts as depth 0; its immediate subfields are depth 1, and so on.
// Schemas with subfields nested beyond this limit are rejected by Load.
const MaxSubfieldDepth = 5

// checkSubfieldDepth returns an error if any attribute tree in attrs has
// subfield nesting deeper than MaxSubfieldDepth.
func checkSubfieldDepth(attrs []Attribute) error {
	for _, a := range attrs {
		if err := checkAttributeDepth(a, 0); err != nil {
			return err
		}
	}
	return nil
}

func checkAttributeDepth(a Attribute, depth int) error {
	if depth > MaxSubfieldDepth {
		return fmt.Errorf("attribute %q exceeds maximum subfield nesting depth of %d", a.Key, MaxSubfieldDepth)
	}
	for _, sub := range a.Subfields {
		if err := checkAttributeDepth(sub, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// Validate performs cross-reference checks between schema entries and external
// sources (sport taxonomy, applicable modifiers). It does NOT re-validate the
// YAML structure — that is done by Load via the embedded JSONSchema.
//
// taxonomyDir is the directory containing sports.yaml (unused currently — the
// sport registry is loaded via the generated sport package at init time).
// modifiersPath is the path to config/sport-taxonomy/modifiers.yaml.
func Validate(schema *Schema, taxonomyDir, modifiersPath string) error {
	var errs []string

	// Build fast lookup sets.
	roleKeys := make(map[string]struct{}, len(schema.AssignmentRoles))
	for _, r := range schema.AssignmentRoles {
		roleKeys[r.Key] = struct{}{}
	}

	optionSetKeys := make(map[string]struct{}, len(schema.OptionSets))
	for _, os := range schema.OptionSets {
		optionSetKeys[os.Key] = struct{}{}
	}

	attrKeys := make(map[string]struct{})
	collectAttrKeys(schema.Attributes, attrKeys)

	gearTypeKeys := make(map[string]struct{}, len(schema.GearTypes))
	for _, gt := range schema.GearTypes {
		gearTypeKeys[gt.Key] = struct{}{}
	}

	componentTypeKeys := make(map[string]struct{}, len(schema.ComponentTypes))
	for _, ct := range schema.ComponentTypes {
		componentTypeKeys[ct.Key] = struct{}{}
	}

	// Validate option_set_key references in attributes.
	if attrErrs := validateAttributeOptionSetRefs(schema.Attributes, optionSetKeys); len(attrErrs) > 0 {
		errs = append(errs, attrErrs...)
	}

	// Validate unit_group references and allowed_units subset relationship.
	if unitErrs := validateAttributeUnitGroupRefs(schema.Attributes, schema.UnitGroups); len(unitErrs) > 0 {
		errs = append(errs, unitErrs...)
	}

	// Validate show_when.field references against the attribute tree.
	if swErrs := validateAttributeShowWhenRefs(schema.Attributes); len(swErrs) > 0 {
		errs = append(errs, swErrs...)
	}

	// Validate visible_when / applies_to_gear_when ref namespaces against
	// the per-item attribute scope. Catches typos that would silently
	// evaluate to false at runtime.
	if condErrs := validateConditionRefs(schema); len(condErrs) > 0 {
		errs = append(errs, condErrs...)
	}

	// Validate kombi_group cardinality: every kombi_group must contain
	// exactly two attributes within a single gear_type or component_type
	// scope.
	if kgErrs := validateKombiGroups(schema); len(kgErrs) > 0 {
		errs = append(errs, kgErrs...)
	}

	// Load gear modifier audience from modifiers.yaml.
	gearModifierKeys, err := applicablemodifiers.LoadForSubsystem(modifiersPath, "gear")
	if err != nil {
		return fmt.Errorf("load gear modifiers from %s: %w", modifiersPath, err)
	}
	gearModifierSet := make(map[string]struct{}, len(gearModifierKeys))
	for _, k := range gearModifierKeys {
		gearModifierSet[k] = struct{}{}
	}

	for _, gt := range schema.GearTypes {
		prefix := fmt.Sprintf("gear_type %q", gt.Key)

		// parent_key must reference an existing gear type key.
		if gt.ParentKey != "" {
			if _, ok := gearTypeKeys[gt.ParentKey]; !ok {
				errs = append(errs, fmt.Sprintf("%s: parent_key %q not found in gear_types", prefix, gt.ParentKey))
			}
		}

		// default_assignment_role_key must reference an existing role.
		if _, ok := roleKeys[gt.DefaultAssignmentRoleKey]; !ok {
			errs = append(errs, fmt.Sprintf("%s: default_assignment_role_key %q not found in assignment_roles", prefix, gt.DefaultAssignmentRoleKey))
		}

		// attribute_keys must reference existing attributes.
		for _, ak := range gt.AttributeKeys {
			if _, ok := attrKeys[ak]; !ok {
				errs = append(errs, fmt.Sprintf("%s: attribute_key %q not found in attributes", prefix, ak))
			}
		}

		// modifier_keys must be in the gear audience of modifiers.yaml.
		for _, mk := range gt.ModifierKeys {
			if _, ok := gearModifierSet[mk]; !ok {
				errs = append(errs, fmt.Sprintf(
					"%s: modifier_key %q is not in the gear audience of modifiers.yaml; "+
						"add subsystems: [..., gear] to that modifier and re-run "+
						"./scripts/generate-sport-taxonomy.sh && ./scripts/generate-gear-schema.sh",
					prefix, mk,
				))
			}
		}

		// applicable_sports predicates must exist in the sport taxonomy.
		for _, pred := range gt.ApplicableSports {
			if err := validateSportPredicate(pred); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", prefix, err))
			}
		}

		// slots[*].component_type must reference an existing component_type key.
		for _, slot := range gt.Slots {
			if _, ok := componentTypeKeys[slot.ComponentType]; !ok {
				errs = append(errs, fmt.Sprintf("%s: slot %q component_type %q not found in component_types",
					prefix, slot.Key, slot.ComponentType))
			}
		}
	}

	// component_types cross-reference checks.
	for _, ct := range schema.ComponentTypes {
		ctPrefix := fmt.Sprintf("component_type %q", ct.Key)

		// attribute_keys must reference existing attributes.
		for _, ak := range ct.AttributeKeys {
			if _, ok := attrKeys[ak]; !ok {
				errs = append(errs, fmt.Sprintf("%s: attribute_key %q not found in attributes", ctPrefix, ak))
			}
		}

		// subtype_keys must reference existing component_type keys.
		for _, sk := range ct.SubtypeKeys {
			if _, ok := componentTypeKeys[sk]; !ok {
				errs = append(errs, fmt.Sprintf("%s: subtype_key %q not found in component_types", ctPrefix, sk))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("gear schema cross-reference errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// collectAttrKeys recursively collects all attribute keys (including subfields) into dest.
func collectAttrKeys(attrs []Attribute, dest map[string]struct{}) {
	for _, a := range attrs {
		dest[a.Key] = struct{}{}
		collectAttrKeys(a.Subfields, dest)
	}
}

// validateAttributeOptionSetRefs checks that any option_set_key in attrs
// (and their subfields) references a declared option set.
func validateAttributeOptionSetRefs(attrs []Attribute, optionSetKeys map[string]struct{}) []string {
	var errs []string
	for _, a := range attrs {
		if a.OptionSetKey != "" {
			if _, ok := optionSetKeys[a.OptionSetKey]; !ok {
				errs = append(errs, fmt.Sprintf("attribute %q: option_set_key %q not found in option_sets", a.Key, a.OptionSetKey))
			}
		}
		errs = append(errs, validateAttributeOptionSetRefs(a.Subfields, optionSetKeys)...)
	}
	return errs
}

// validateAttributeUnitGroupRefs checks that every attribute (and subfield)
// referencing a unit_group has that group declared and that its allowed_units
// is a subset of the group's allowed_units. An attribute may declare a
// unit_group without an explicit allowed_units list — that inherits the
// group's full allowed_units implicitly.
func validateAttributeUnitGroupRefs(attrs []Attribute, groups []UnitGroup) []string {
	groupSet := make(map[string]map[string]struct{}, len(groups))
	for _, g := range groups {
		units := make(map[string]struct{}, len(g.AllowedUnits))
		for _, u := range g.AllowedUnits {
			units[u] = struct{}{}
		}
		groupSet[g.Key] = units
	}
	var errs []string
	collectUnitGroupErrors(attrs, groupSet, &errs)
	return errs
}

func collectUnitGroupErrors(attrs []Attribute, groupSet map[string]map[string]struct{}, errs *[]string) {
	for _, a := range attrs {
		if a.UnitGroup != "" {
			groupUnits, ok := groupSet[a.UnitGroup]
			if !ok {
				*errs = append(*errs, fmt.Sprintf(
					"attribute %q: unit_group %q not found in unit_groups registry",
					a.Key, a.UnitGroup))
			} else {
				for _, u := range a.AllowedUnits {
					if _, ok := groupUnits[u]; !ok {
						*errs = append(*errs, fmt.Sprintf(
							"attribute %q: allowed_unit %q is not in unit_group %q registry",
							a.Key, u, a.UnitGroup))
					}
				}
			}
		}
		collectUnitGroupErrors(a.Subfields, groupSet, errs)
	}
}

// validateAttributeShowWhenRefs checks that every show_when.field references
// a sibling attribute key in the same scope. For top-level attributes that
// means another top-level key; for subfields that means a sibling subfield
// key under the same parent. This mirrors the runtime validator's scope
// resolution in isAttributeActive — it always looks up the field in the
// same value map that contains the attribute itself.
func validateAttributeShowWhenRefs(attrs []Attribute) []string {
	var errs []string
	collectShowWhenErrors("", attrs, &errs)
	return errs
}

func collectShowWhenErrors(parentKey string, siblings []Attribute, errs *[]string) {
	siblingKeys := make(map[string]struct{}, len(siblings))
	for _, a := range siblings {
		siblingKeys[a.Key] = struct{}{}
	}
	for _, a := range siblings {
		if a.ShowWhen != nil {
			if _, ok := siblingKeys[a.ShowWhen.Field]; !ok {
				scope := "top-level"
				if parentKey != "" {
					scope = fmt.Sprintf("subfields of %q", parentKey)
				}
				*errs = append(*errs, fmt.Sprintf(
					"attribute %q: show_when.field %q has no sibling in scope (%s)",
					a.Key, a.ShowWhen.Field, scope))
			}
		}
		if len(a.Subfields) > 0 {
			collectShowWhenErrors(a.Key, a.Subfields, errs)
		}
	}
}


// validateConditionRefs walks every visible_when / applies_to_gear_when
// condition on each attribute and verifies that ref resolves to a known
// key in the appropriate scope.
//
// Resolution rules mirror the runtime evaluator:
//
//	bare key            — same item's attribute set (the gear_type or
//	                      component_type whose attribute_keys list this
//	                      attribute)
//	gear.<attr>         — attribute defined on the gear_type the
//	                      component is attached to. For cross-item refs
//	                      we accept the attribute as long as it is
//	                      defined on ANY gear_type that uses (directly
//	                      or via a slot) the component_type carrying
//	                      this condition.
//	component.<attr>    — attribute defined on a component_type that the
//	                      current gear_type's slots reference (or, for a
//	                      component condition, on the current component
//	                      or its subtypes).
//
// Bare refs on attributes that appear on both a gear_type and a
// component_type are validated against both scopes — the union is used
// to keep the rule permissive.
func validateConditionRefs(schema *Schema) []string {
	// Build scope maps once.
	gearTypeByKey := make(map[string]GearType, len(schema.GearTypes))
	for _, gt := range schema.GearTypes {
		gearTypeByKey[gt.Key] = gt
	}
	componentTypeByKey := make(map[string]ComponentType, len(schema.ComponentTypes))
	for _, ct := range schema.ComponentTypes {
		componentTypeByKey[ct.Key] = ct
	}

	// gearAttrScope[gt] = union of gt's attribute_keys and all parent gear_types'
	// attribute_keys (parent_key chain). Mirrors the runtime view that a child
	// gear instance carries every attribute its parent type declares.
	gearAttrScope := make(map[string]map[string]struct{}, len(schema.GearTypes))
	for _, gt := range schema.GearTypes {
		scope := map[string]struct{}{}
		cur := gt.Key
		for i := 0; i < 32; i++ {
			node, ok := gearTypeByKey[cur]
			if !ok {
				break
			}
			for _, k := range node.AttributeKeys {
				scope[k] = struct{}{}
			}
			if node.ParentKey == "" {
				break
			}
			cur = node.ParentKey
		}
		gearAttrScope[gt.Key] = scope
	}

	// componentAttrScope[ct] = union of ct's attribute_keys and ancestors' via
	// component_type parent_key.
	componentAttrScope := make(map[string]map[string]struct{}, len(schema.ComponentTypes))
	for _, ct := range schema.ComponentTypes {
		scope := map[string]struct{}{}
		cur := ct.Key
		for i := 0; i < 32; i++ {
			node, ok := componentTypeByKey[cur]
			if !ok {
				break
			}
			for _, k := range node.AttributeKeys {
				scope[k] = struct{}{}
			}
			if node.ParentKey == "" {
				break
			}
			cur = node.ParentKey
		}
		componentAttrScope[ct.Key] = scope
	}

	// component_type -> gear_types that mount it via a slot (directly or via
	// gear_type parent chain — a child inherits the parent's slots).
	// Direct mapping first; expanded to component_type descendants in a
	// second pass so a child component (e.g. bike.air_shock) inherits the
	// gear_types that mount its parent (bike.shock).
	componentToGearTypes := make(map[string]map[string]struct{}, len(schema.ComponentTypes))
	for _, gt := range schema.GearTypes {
		// Walk gear_type parent chain and collect every slot encountered.
		cur := gt.Key
		for i := 0; i < 32; i++ {
			node, ok := gearTypeByKey[cur]
			if !ok {
				break
			}
			for _, slot := range node.Slots {
				if _, ok := componentToGearTypes[slot.ComponentType]; !ok {
					componentToGearTypes[slot.ComponentType] = map[string]struct{}{}
				}
				componentToGearTypes[slot.ComponentType][gt.Key] = struct{}{}
			}
			if node.ParentKey == "" {
				break
			}
			cur = node.ParentKey
		}
	}
	// Propagate parent component_type's gear_type set down to its descendants:
	// a slot for `bike.shock` also mounts its subtypes (`bike.air_shock`, ...).
	for _, ct := range schema.ComponentTypes {
		// Walk component parent_key chain and merge ancestors' gear_type sets.
		cur := ct.Key
		for i := 0; i < 32; i++ {
			node, ok := componentTypeByKey[cur]
			if !ok {
				break
			}
			if node.ParentKey == "" {
				break
			}
			cur = node.ParentKey
			if parentSet, ok := componentToGearTypes[cur]; ok {
				if _, ok := componentToGearTypes[ct.Key]; !ok {
					componentToGearTypes[ct.Key] = map[string]struct{}{}
				}
				for k := range parentSet {
					componentToGearTypes[ct.Key][k] = struct{}{}
				}
			}
		}
	}

	// gear_type -> component_types reachable via its slots (own + inherited).
	gearTypeToComponents := make(map[string]map[string]struct{}, len(schema.GearTypes))
	for _, gt := range schema.GearTypes {
		set := map[string]struct{}{}
		cur := gt.Key
		for i := 0; i < 32; i++ {
			node, ok := gearTypeByKey[cur]
			if !ok {
				break
			}
			for _, slot := range node.Slots {
				set[slot.ComponentType] = struct{}{}
			}
			if node.ParentKey == "" {
				break
			}
			cur = node.ParentKey
		}
		gearTypeToComponents[gt.Key] = set
	}

	// attribute_key -> attribute (for quick lookup during walk)
	allAttrs := map[string]Attribute{}
	collectAttrsByKey(schema.Attributes, allAttrs)

	var errs []string

	// Walk gear_type attributes.
	for _, gt := range schema.GearTypes {
		for _, ak := range gt.AttributeKeys {
			attr, ok := allAttrs[ak]
			if !ok {
				continue // unknown attribute already reported elsewhere
			}
			ctx := refCtx{
				owner:                "gear_type " + gt.Key,
				attrKey:              attr.Key,
				bareScope:            gearAttrScope[gt.Key],
				gearScopes:           []map[string]struct{}{gearAttrScope[gt.Key]},
				componentScopes:      collectScopes(componentAttrScope, gearTypeToComponents[gt.Key]),
				bareScopeDescription: "gear_type " + gt.Key,
			}
			errs = append(errs, walkConditionRefs(attr, ctx)...)
		}
	}

	// Walk component_type attributes.
	for _, ct := range schema.ComponentTypes {
		// Gear scopes for gear.<attr> refs: every gear_type that mounts this component.
		gearScopes := collectScopes(gearAttrScope, componentToGearTypes[ct.Key])
		// Component scopes for component.<attr> refs: self + subtypes.
		componentScopeKeys := map[string]struct{}{ct.Key: {}}
		for _, sk := range ct.SubtypeKeys {
			componentScopeKeys[sk] = struct{}{}
		}
		componentScopes := collectScopes(componentAttrScope, componentScopeKeys)
		for _, ak := range ct.AttributeKeys {
			attr, ok := allAttrs[ak]
			if !ok {
				continue
			}
			ctx := refCtx{
				owner:                "component_type " + ct.Key,
				attrKey:              attr.Key,
				bareScope:            componentAttrScope[ct.Key],
				gearScopes:           gearScopes,
				componentScopes:      componentScopes,
				bareScopeDescription: "component_type " + ct.Key,
			}
			errs = append(errs, walkConditionRefs(attr, ctx)...)
		}
	}
	return errs
}

// refCtx carries the scopes a condition ref may resolve into when
// evaluated from a specific owner (gear_type or component_type).
type refCtx struct {
	owner                string
	attrKey              string
	bareScope            map[string]struct{}   // attributes reachable via bare ref
	gearScopes           []map[string]struct{} // attribute sets of every gear_type the owner could resolve `gear.<attr>` against
	componentScopes      []map[string]struct{} // attribute sets of every component_type the owner could resolve `component.<attr>` against
	bareScopeDescription string
}

func walkConditionRefs(a Attribute, ctx refCtx) []string {
	var errs []string
	for _, c := range a.VisibleWhen {
		if err := checkConditionRef("visible_when", a.Key, c.Ref, ctx); err != "" {
			errs = append(errs, err)
		}
	}
	for _, c := range a.AppliesToGearWhen {
		if err := checkConditionRef("applies_to_gear_when", a.Key, c.Ref, ctx); err != "" {
			errs = append(errs, err)
		}
	}
	// Subfields share the parent's scope (siblings within the same item).
	for _, sub := range a.Subfields {
		subCtx := ctx
		subCtx.attrKey = sub.Key
		errs = append(errs, walkConditionRefs(sub, subCtx)...)
	}
	return errs
}

func checkConditionRef(field, attrKey, ref string, ctx refCtx) string {
	switch {
	case strings.HasPrefix(ref, "gear."):
		key := strings.TrimPrefix(ref, "gear.")
		if scopeContainsAny(ctx.gearScopes, key) {
			return ""
		}
		return fmt.Sprintf(
			"%s, attribute %q: %s.ref %q references gear attribute %q not defined on any gear_type in scope",
			ctx.owner, attrKey, field, ref, key)
	case strings.HasPrefix(ref, "component."):
		key := strings.TrimPrefix(ref, "component.")
		if scopeContainsAny(ctx.componentScopes, key) {
			return ""
		}
		return fmt.Sprintf(
			"%s, attribute %q: %s.ref %q references component attribute %q not defined on any component_type in scope",
			ctx.owner, attrKey, field, ref, key)
	default:
		// Bare ref: must resolve in the owner's own attribute scope.
		if _, ok := ctx.bareScope[ref]; ok {
			return ""
		}
		return fmt.Sprintf(
			"%s, attribute %q: %s.ref %q (bare) has no matching attribute in %s",
			ctx.owner, attrKey, field, ref, ctx.bareScopeDescription)
	}
}

func scopeContainsAny(scopes []map[string]struct{}, key string) bool {
	for _, s := range scopes {
		if _, ok := s[key]; ok {
			return true
		}
	}
	return false
}

func collectScopes(byKey map[string]map[string]struct{}, keys map[string]struct{}) []map[string]struct{} {
	out := make([]map[string]struct{}, 0, len(keys))
	for k := range keys {
		if scope, ok := byKey[k]; ok {
			out = append(out, scope)
		}
	}
	return out
}

func collectAttrsByKey(attrs []Attribute, dest map[string]Attribute) {
	for _, a := range attrs {
		dest[a.Key] = a
		collectAttrsByKey(a.Subfields, dest)
	}
}

// validateKombiGroups verifies that every kombi_group within a single
// gear_type or component_type scope contains exactly two attributes.
// A group of 1 (orphan) or 3+ is rejected.
func validateKombiGroups(schema *Schema) []string {
	allAttrs := map[string]Attribute{}
	collectAttrsByKey(schema.Attributes, allAttrs)
	var errs []string
	check := func(owner string, attrKeys []string) {
		groups := map[string][]string{}
		for _, ak := range attrKeys {
			attr, ok := allAttrs[ak]
			if !ok {
				continue
			}
			if attr.KombiGroup != "" {
				groups[attr.KombiGroup] = append(groups[attr.KombiGroup], attr.Key)
			}
		}
		for name, members := range groups {
			if len(members) != 2 {
				errs = append(errs, fmt.Sprintf(
					"%s: kombi_group %q has %d members (%v); exactly 2 required",
					owner, name, len(members), members))
			}
		}
	}
	for _, gt := range schema.GearTypes {
		check("gear_type "+gt.Key, gt.AttributeKeys)
	}
	for _, ct := range schema.ComponentTypes {
		check("component_type "+ct.Key, ct.AttributeKeys)
	}
	return errs
}

// validateSportPredicate checks that (sport_type, sub_type) exists in the sport registry.
func validateSportPredicate(pred Predicate) error {
	def, ok := sport.GetSportDef(pred.SportType)
	if !ok {
		return fmt.Errorf("applicable_sports: sport_type %q not found in sport taxonomy", pred.SportType)
	}
	if pred.SubType != "" {
		if _, ok := def.SubTypes[pred.SubType]; !ok {
			return fmt.Errorf("applicable_sports: sub_type %q not found for sport_type %q in sport taxonomy", pred.SubType, pred.SportType)
		}
	}
	return nil
}

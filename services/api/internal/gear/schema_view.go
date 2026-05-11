package gear

// GearTypeAppliesToTarget reports whether a gear of sourceTypeKey is
// compatible with a target of targetTypeKey. The relation holds when
// sourceTypeKey equals targetTypeKey, or when sourceTypeKey is an ancestor of
// targetTypeKey in the GearType.ParentKey chain.
//
// Example: source="bike", target="bike.mtb.enduro" → true
// because the chain is bike.mtb.enduro → bike.mtb → bike.
func (s GearSchema) GearTypeAppliesToTarget(sourceTypeKey, targetTypeKey string) bool {
	if sourceTypeKey == "" || targetTypeKey == "" {
		return false
	}
	cur := targetTypeKey
	for i := 0; i < 32; i++ { // depth guard against cycles
		if cur == sourceTypeKey {
			return true
		}
		parent, ok := s.gearTypeParent(cur)
		if !ok || parent == "" {
			return false
		}
		cur = parent
	}
	return false
}

// ComponentTypeAppliesToGear reports whether a component of componentTypeKey
// may be installed on a gear of gearTypeKey. The relation holds when the gear
// type (or any of its GearType ancestors) has a slot whose ComponentType
// matches componentTypeKey or any ancestor of componentTypeKey.
//
// Example: componentTypeKey="bike.frame", gearTypeKey="bike.mtb.enduro" → true
// because bike.mtb (parent of bike.mtb.enduro) has a slot with
// ComponentType="bike.frame".
func (s GearSchema) ComponentTypeAppliesToGear(componentTypeKey, gearTypeKey string) bool {
	if componentTypeKey == "" || gearTypeKey == "" {
		return false
	}
	ancestors := s.componentTypeAncestors(componentTypeKey)
	cur := gearTypeKey
	for i := 0; i < 32; i++ { // depth guard
		if gt, ok := s.gearTypeByKey(cur); ok {
			for _, slot := range gt.Slots {
				if _, hit := ancestors[slot.ComponentType]; hit {
					return true
				}
			}
		}
		parent, ok := s.gearTypeParent(cur)
		if !ok || parent == "" {
			break
		}
		cur = parent
	}
	return false
}

// gearTypeParent returns the ParentKey of the GearType with the given key.
// ok=false means the key is not found in the schema.
func (s GearSchema) gearTypeParent(key string) (parent string, ok bool) {
	for i := range s.GearTypes {
		if s.GearTypes[i].Key == key {
			return s.GearTypes[i].ParentKey, true
		}
	}
	return "", false
}

// gearTypeByKey returns the GearType for the given key.
func (s GearSchema) gearTypeByKey(key string) (GearType, bool) {
	for i := range s.GearTypes {
		if s.GearTypes[i].Key == key {
			return s.GearTypes[i], true
		}
	}
	return GearType{}, false
}

// componentTypeAncestors returns a set containing componentTypeKey and all of
// its ancestors via ComponentType.ParentKey, inclusive.
func (s GearSchema) componentTypeAncestors(componentTypeKey string) map[string]struct{} {
	result := make(map[string]struct{})
	cur := componentTypeKey
	for i := 0; i < 32; i++ { // depth guard
		result[cur] = struct{}{}
		parent := ""
		for j := range s.ComponentTypes {
			if s.ComponentTypes[j].Key == cur {
				parent = s.ComponentTypes[j].ParentKey
				break
			}
		}
		if parent == "" {
			break
		}
		if _, seen := result[parent]; seen {
			break // cycle guard
		}
		cur = parent
	}
	return result
}

// deepEqual compares two values structurally, ignoring object-key order.
//
// Treats `undefined` values as equivalent to missing keys, so a form that
// removes a field (sets value to undefined) reads as "different from initial"
// only when the initial actually had a defined value.
//
// Handles plain objects, arrays, primitives, and `null`. Date / Map / Set /
// class instances are compared by reference; this helper is intended for the
// JSON-shaped values that flow through gear-component forms.
export function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (a === null || b === null) return a === b
  if (typeof a !== typeof b) return false

  if (Array.isArray(a)) {
    if (!Array.isArray(b)) return false
    if (a.length !== b.length) return false
    for (let i = 0; i < a.length; i++) {
      if (!deepEqual(a[i], b[i])) return false
    }
    return true
  }
  if (Array.isArray(b)) return false

  if (typeof a === 'object') {
    if (typeof b !== 'object') return false
    const ao = a as Record<string, unknown>
    const bo = b as Record<string, unknown>
    // Collect defined-only keys from both sides — undefined values are treated
    // as missing so { x: undefined } and {} compare equal.
    const keys = new Set<string>()
    for (const k of Object.keys(ao)) {
      if (ao[k] !== undefined) keys.add(k)
    }
    for (const k of Object.keys(bo)) {
      if (bo[k] !== undefined) keys.add(k)
    }
    for (const k of keys) {
      if (!deepEqual(ao[k], bo[k])) return false
    }
    return true
  }

  // Primitives that aren't === already returned false above.
  return false
}

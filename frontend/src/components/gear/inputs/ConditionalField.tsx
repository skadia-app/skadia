import type { Attribute } from '@/lib/generatedGearSchema'

export interface ConditionalFieldProps {
  attribute: Attribute
  /** The form's current flat value object at the same scope as this attribute */
  siblingValues: Record<string, unknown>
  children: React.ReactNode
}

// Evaluates whether this attribute should be shown based on its show_when spec.
// Returns true when:
//   - attribute has no show_when (always visible)
//   - the sibling field referenced by show_when.field has the exact expected value
function isVisible(attribute: Attribute, siblingValues: Record<string, unknown>): boolean {
  const sw = attribute.showWhen
  if (!sw) return true
  return String(siblingValues[sw.field] ?? '') === sw.value
}

export default function ConditionalField({
  attribute,
  siblingValues,
  children,
}: ConditionalFieldProps) {
  if (!isVisible(attribute, siblingValues)) return null
  return <>{children}</>
}

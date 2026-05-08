import { modifierKeysForCapability, modifierLabel } from './sportModifiers'

export const ROUTE_MODIFIER_LABELS = Object.fromEntries(
  modifierKeysForCapability('route_filter').map((key) => [key, modifierLabel(key)]),
) as Record<RouteModifierKey, string>

export type RouteModifierKey = 'is_electric' | 'is_indoor' | 'is_virtual' | 'is_commute'

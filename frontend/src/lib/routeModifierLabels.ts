import { modifierLabel } from './sportModifiers'
import { RoutingApplicableModifierKeys } from './generatedRoutingApplicableModifiers'

export const ROUTE_MODIFIER_LABELS = Object.fromEntries(
  RoutingApplicableModifierKeys.map((key) => [key, modifierLabel(key as Parameters<typeof modifierLabel>[0])]),
) as Record<RouteModifierKey, string>

export type RouteModifierKey = 'is_electric' | 'is_indoor' | 'is_virtual' | 'is_commute'

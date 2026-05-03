export const ROUTE_MODIFIER_LABELS = {
  is_electric: 'E-Antrieb',
  is_indoor: 'Indoor',
  is_virtual: 'Virtuell',
  is_commute: 'Pendeln',
} as const

export type RouteModifierKey = keyof typeof ROUTE_MODIFIER_LABELS

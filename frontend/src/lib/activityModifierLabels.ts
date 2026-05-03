export interface ActivityModifierLabelState {
  isElectric?: boolean
  isIndoor?: boolean
  isVirtual?: boolean
}

export function activityModifierLabels(state: ActivityModifierLabelState): string[] {
  return [
    state.isElectric ? 'E-Antrieb' : '',
    state.isIndoor ? 'Indoor' : '',
    state.isVirtual ? 'Virtuell' : '',
  ].filter(Boolean)
}

export interface SportModifierFlags {
  isElectric?: boolean
  isIndoor?: boolean
  isVirtual?: boolean
  isCommute?: boolean
}

export function sportModifierSuffix(flags: SportModifierFlags): string {
  return [
    flags.isElectric ? 'E' : '',
    flags.isIndoor ? 'I' : '',
    flags.isVirtual ? 'V' : '',
    flags.isCommute ? 'P' : '',
  ].filter(Boolean).join('/')
}

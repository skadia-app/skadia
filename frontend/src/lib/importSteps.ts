export interface ImportStep {
  key?: string
  label?: string
  status?: string
}

const terminalStepStatuses = new Set(['completed', 'skipped', 'failed'])

export function activeImportStepFromSteps(rawSteps: unknown): { current: number; total: number; label: string } | null {
  if (!Array.isArray(rawSteps)) return null
  const steps = rawSteps.filter((step): step is ImportStep => (
    !!step && typeof step === 'object' && typeof (step as ImportStep).label === 'string'
  ))
  if (steps.length === 0) return null
  const activeIndex = steps.findIndex(step => !terminalStepStatuses.has(step.status ?? ''))
  if (activeIndex < 0) return null
  const activeStep = steps[activeIndex]
  if (!activeStep?.label) return null
  return {
    current: activeIndex + 1,
    total: steps.length,
    label: activeStep.label,
  }
}

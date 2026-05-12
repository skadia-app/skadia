import type { Activity } from '@/api/types'

function nonEmpty(value: string | null | undefined): string | null {
  const trimmed = value?.trim()
  return trimmed ? trimmed : null
}

export function displayActivityTitle(
  activity: Pick<Activity, 'title' | 'display_title' | 'title_auto' | 'title_is_manual'>,
  fallback: string,
): string {
  const manualTitle = nonEmpty(activity.title)
  const displayTitle = nonEmpty(activity.display_title)
  const autoTitle = nonEmpty(activity.title_auto)

  if (activity.title_is_manual && manualTitle) return manualTitle
  if (displayTitle) return displayTitle
  if (manualTitle) return manualTitle
  if (autoTitle) return autoTitle
  return fallback
}

export function editableActivityTitleSeed(
  activity: Pick<Activity, 'title' | 'display_title' | 'title_auto' | 'title_is_manual'>,
): string {
  if (activity.title_is_manual) return nonEmpty(activity.title) ?? ''
  return nonEmpty(activity.display_title) ?? nonEmpty(activity.title_auto) ?? nonEmpty(activity.title) ?? ''
}

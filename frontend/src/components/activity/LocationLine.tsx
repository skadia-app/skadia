import type { Activity } from '@/api/types'

interface Props {
  activity: Activity
}

export function LocationLine({ activity }: Props) {
  const { start_city, end_city, start_state, start_country } = activity

  if (!start_city) return null

  const region = [start_state, start_country].filter(Boolean).join(', ')

  let line: string
  if (end_city && end_city !== start_city) {
    line = `${start_city} → ${end_city}`
  } else {
    line = start_city
  }

  return (
    <p className="text-sm text-neutral-600 dark:text-neutral-400 flex items-center gap-1.5">
      <span className="text-base" aria-hidden>&#x1F4CD;</span>
      <span>{line}{region ? ` · ${region}` : ''}</span>
    </p>
  )
}

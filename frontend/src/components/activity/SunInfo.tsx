import { fmtTime } from '@/lib/format'

interface SunInfoProps {
  sunriseAt?: string | null
  sunsetAt?: string | null
}

export function SunInfo({ sunriseAt, sunsetAt }: SunInfoProps) {
  if (!sunriseAt && !sunsetAt) return null

  return (
    <div className="flex items-center gap-4 text-sm text-neutral-600 dark:text-neutral-400">
      {sunriseAt && (
        <span className="flex items-center gap-1.5">
          <span className="text-amber-500">&#9728;</span>
          <span>{fmtTime(sunriseAt)}</span>
        </span>
      )}
      {sunsetAt && (
        <span className="flex items-center gap-1.5">
          <span className="text-accent-600">&#9790;</span>
          <span>{fmtTime(sunsetAt)}</span>
        </span>
      )}
    </div>
  )
}

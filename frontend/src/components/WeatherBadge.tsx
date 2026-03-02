interface WeatherBadgeProps {
  tempC?: number | null
  condition?: string | null
  humidity?: number | null
  windSpeedMS?: number | null
}

export default function WeatherBadge({ tempC, condition, humidity, windSpeedMS }: WeatherBadgeProps) {
  const parts: string[] = []

  if (tempC != null) parts.push(`${Math.round(tempC)} °C`)
  if (condition) parts.push(condition)
  if (humidity != null) parts.push(`${Math.round(humidity)}% Luftfeuchtigkeit`)
  if (windSpeedMS != null) parts.push(`${windSpeedMS.toFixed(1)} m/s Wind`)

  if (parts.length === 0) return null

  return (
    <div className="inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 text-sm text-neutral-700 dark:text-neutral-300">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4 text-neutral-400 flex-shrink-0">
        <path d="M10 2a.75.75 0 01.75.75v1.5a.75.75 0 01-1.5 0v-1.5A.75.75 0 0110 2zM10 15a.75.75 0 01.75.75v1.5a.75.75 0 01-1.5 0v-1.5A.75.75 0 0110 15zM10 7a3 3 0 100 6 3 3 0 000-6zM15.657 5.404a.75.75 0 10-1.06-1.06l-1.061 1.06a.75.75 0 001.06 1.06l1.06-1.06zM6.464 14.596a.75.75 0 10-1.06-1.06l-1.06 1.06a.75.75 0 001.06 1.06l1.06-1.06zM18 10a.75.75 0 01-.75.75h-1.5a.75.75 0 010-1.5h1.5A.75.75 0 0118 10zM5 10a.75.75 0 01-.75.75h-1.5a.75.75 0 010-1.5h1.5A.75.75 0 015 10zM14.596 15.657a.75.75 0 001.06-1.06l-1.06-1.061a.75.75 0 10-1.06 1.06l1.06 1.06zM5.404 6.464a.75.75 0 001.06-1.06l-1.06-1.06a.75.75 0 10-1.06 1.06l1.06 1.06z" />
      </svg>
      <span>{parts.join(', ')}</span>
    </div>
  )
}

import type { WeatherReading } from '@/api/types'
import { fmtTime } from '@/lib/format'

function getConditionIcon(condition: string | null | undefined): string {
  if (!condition) return '🌡️'
  const c = condition.toLowerCase()
  if (c.includes('klar') || c.includes('sonnig')) return '☀️'
  if (c.includes('gewitter')) return '⛈️'
  if (c.includes('schnee') || c.includes('schneefall')) return '❄️'
  if (c.includes('regen') || c.includes('schauer') || c.includes('niesel')) return '🌧️'
  if (c.includes('nebel') || c.includes('raureif')) return '🌫️'
  if (c.includes('bewölkt') || c.includes('bedeckt') || c.includes('wolkig')) return '☁️'
  return '🌡️'
}

interface WeatherBadgeCardProps {
  reading: WeatherReading
  displayFields?: string[]
  timezone?: string | null
}

function WeatherBadgeCard({ reading, displayFields, timezone }: WeatherBadgeCardProps) {
  const show = (field: string) => !displayFields || displayFields.includes(field)
  const icon = getConditionIcon(reading.condition)
  const tempDiff =
    reading.temp_c != null && reading.apparent_temp_c != null
      ? Math.abs(reading.temp_c - reading.apparent_temp_c)
      : 0

  return (
    <div
      className="inline-flex flex-col gap-0.5 rounded-lg border px-3 py-2 text-sm border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50"
      title={reading.model ? `Datenquelle: ${reading.model}` : undefined}
    >
      {/* Label (Berg / Start etc.) + Uhrzeit */}
      {(reading.label || reading.recorded_at) && (
        <div className="flex items-center gap-3 text-xs font-medium text-neutral-500 dark:text-neutral-400 leading-none mb-0.5">
          {reading.label && <span>{reading.label}</span>}
          {reading.recorded_at && (
            <span className="font-normal tabular-nums">
              {fmtTime(reading.recorded_at, timezone)}
            </span>
          )}
        </div>
      )}

      {/* Hauptzeile: Icon + Temperatur + Kondition */}
      <div className="flex items-center gap-1.5 font-medium text-neutral-800 dark:text-neutral-200">
        <span>{icon}</span>
        {reading.temp_c != null && show('temp_c') && (
          <span>{reading.temp_c.toFixed(1)}°C</span>
        )}
        {reading.condition && show('condition') && (
          <span className="text-neutral-500 dark:text-neutral-400 font-normal">
            {reading.condition}
          </span>
        )}
      </div>

      {/* Gefühlte Temperatur */}
      {reading.apparent_temp_c != null && show('apparent_temp_c') && tempDiff > 1 && (
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          {reading.apparent_temp_c.toFixed(1)}°C gefühlt
        </div>
      )}

      {/* Details-Zeile */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-neutral-500 dark:text-neutral-400">
        {reading.humidity != null && show('humidity') && (
          <span>💧 {reading.humidity}%</span>
        )}
        {reading.wind_speed_ms != null && show('wind_speed_ms') && (
          <span>
            💨 {reading.wind_speed_ms.toFixed(1)} m/s
            {reading.wind_gusts_ms != null && show('wind_gusts_ms') && (
              <> (Böen {reading.wind_gusts_ms.toFixed(1)})</>
            )}
          </span>
        )}
        {reading.precipitation_mm != null && reading.precipitation_mm > 0 && show('precipitation_mm') && (
          <span>🌧 {reading.precipitation_mm.toFixed(1)} mm</span>
        )}
      </div>

      {/* Schneefallgrenze */}
      {reading.freezing_level_m != null && show('freezing_level_m') && (
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          Schneefallgrenze: {Math.round(reading.freezing_level_m)} m
        </div>
      )}

      {/* UV-Index (nur wenn hoch) */}
      {reading.uv_index != null && reading.uv_index > 5 && show('uv_index') && (
        <div className="text-xs font-medium text-orange-500 dark:text-orange-400">
          UV {reading.uv_index.toFixed(0)}
        </div>
      )}
    </div>
  )
}

interface WeatherBadgeProps {
  readings: WeatherReading[]
  displayFields?: string[]
  className?: string
  timezone?: string | null
}

export default function WeatherBadge({ readings, displayFields, className, timezone }: WeatherBadgeProps) {
  if (!readings || readings.length === 0) return null

  return (
    <div className={`flex flex-wrap gap-2${className ? ` ${className}` : ''}`}>
      {readings.map((r) => (
        <WeatherBadgeCard key={r.id} reading={r} displayFields={displayFields} timezone={timezone} />
      ))}
    </div>
  )
}

import type { WeatherReading } from '@/api/types'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import { fmtTime } from '@/lib/format'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { useTranslation } from 'react-i18next'

const UNIT_VALUE = '__SKADIA_UNIT_VALUE__'

const WEATHER_CONDITION_COPY: Record<string, LocalizedString> = {
  'klar': {
    de: 'klar',
    en: 'clear',
    fr: 'dégagé',
    it: 'sereno',
    es: 'despejado',
  },
  'überwiegend klar': {
    de: 'überwiegend klar',
    en: 'mostly clear',
    fr: 'majoritairement dégagé',
    it: 'prevalentemente sereno',
    es: 'mayormente despejado',
  },
  'teilweise bewölkt': {
    de: 'teilweise bewölkt',
    en: 'partly cloudy',
    fr: 'partiellement nuageux',
    it: 'parzialmente nuvoloso',
    es: 'parcialmente nublado',
  },
  'bedeckt': {
    de: 'bedeckt',
    en: 'overcast',
    fr: 'couvert',
    it: 'coperto',
    es: 'cubierto',
  },
  'nebel': {
    de: 'Nebel',
    en: 'fog',
    fr: 'brouillard',
    it: 'nebbia',
    es: 'niebla',
  },
  'nieselregen': {
    de: 'Nieselregen',
    en: 'drizzle',
    fr: 'bruine',
    it: 'pioviggine',
    es: 'llovizna',
  },
  'gefrierender nieselregen': {
    de: 'gefrierender Nieselregen',
    en: 'freezing drizzle',
    fr: 'bruine verglaçante',
    it: 'pioviggine congelante',
    es: 'llovizna engelante',
  },
  'regen': {
    de: 'Regen',
    en: 'rain',
    fr: 'pluie',
    it: 'pioggia',
    es: 'lluvia',
  },
  'gefrierender regen': {
    de: 'gefrierender Regen',
    en: 'freezing rain',
    fr: 'pluie verglaçante',
    it: 'pioggia congelante',
    es: 'lluvia engelante',
  },
  'schneefall': {
    de: 'Schneefall',
    en: 'snowfall',
    fr: 'Chute de neige',
    it: 'nevicata',
    es: 'nevada',
  },
  'schneegriesel': {
    de: 'Schneegriesel',
    en: 'snow grains',
    fr: 'neige en grains',
    it: 'neve granulare',
    es: 'cinarra',
  },
  'regenschauer': {
    de: 'Regenschauer',
    en: 'rain showers',
    fr: 'averses de pluie',
    it: 'rovesci di pioggia',
    es: 'chubascos',
  },
  'schneeschauer': {
    de: 'Schneeschauer',
    en: 'snow showers',
    fr: 'averses de neige',
    it: 'rovesci di neve',
    es: 'chubascos de nieve',
  },
  'gewitter': {
    de: 'Gewitter',
    en: 'thunderstorm',
    fr: 'orage',
    it: 'temporale',
    es: 'tormenta',
  },
  'gewitter mit leichtem hagel': {
    de: 'Gewitter mit leichtem Hagel',
    en: 'thunderstorm with light hail',
    fr: 'orage avec grêle faible',
    it: 'temporale con grandine leggera',
    es: 'tormenta con granizo ligero',
  },
  'gewitter mit starkem hagel': {
    de: 'Gewitter mit starkem Hagel',
    en: 'thunderstorm with heavy hail',
    fr: 'orage avec forte grêle',
    it: 'temporale con grandine forte',
    es: 'tormenta con granizo fuerte',
  },
  'unbekannt': {
    de: 'unbekannt',
    en: 'unknown',
    fr: 'inconnu',
    it: 'sconosciuto',
    es: 'desconocido',
  },
}

const WEATHER_TAG_COPY: Record<string, LocalizedString> = {
  'sonnig': {
    de: 'sonnig',
    en: 'sunny',
    fr: 'ensoleillé',
    it: 'soleggiato',
    es: 'soleado',
  },
  'bewölkt': {
    de: 'bewölkt',
    en: 'cloudy',
    fr: 'nuageux',
    it: 'nuvoloso',
    es: 'nublado',
  },
  'regen': {
    de: 'Regen',
    en: 'rain',
    fr: 'pluie',
    it: 'pioggia',
    es: 'lluvia',
  },
  'schnee': {
    de: 'Schnee',
    en: 'snow',
    fr: 'neige',
    it: 'neve',
    es: 'nieve',
  },
  'nebel': {
    de: 'Nebel',
    en: 'fog',
    fr: 'brouillard',
    it: 'nebbia',
    es: 'niebla',
  },
  'gewitter': {
    de: 'Gewitter',
    en: 'thunderstorm',
    fr: 'orage',
    it: 'temporale',
    es: 'tormenta',
  },
}

const WEATHER_TAG_PRIORITY = ['gewitter', 'schnee', 'regen', 'nebel', 'bewölkt', 'sonnig'] as const

const WEATHER_TAG_ICON: Record<(typeof WEATHER_TAG_PRIORITY)[number], string> = {
  gewitter: '⛈️',
  schnee: '❄️',
  regen: '🌧️',
  nebel: '🌫️',
  'bewölkt': '☁️',
  sonnig: '☀️',
}

function replaceLegacyMetricUnit(template: string, formattedValue: string, legacyUnit: string): string {
  return template
    .replace(`${UNIT_VALUE} ${legacyUnit}`, formattedValue)
    .replace(`${UNIT_VALUE}${legacyUnit}`, formattedValue)
    .replace(UNIT_VALUE, formattedValue)
}

function hasWeatherTag(tags: string[] | undefined, tag: string): boolean {
  return tags?.some((value) => value.toLowerCase() === tag) ?? false
}

function primaryWeatherTag(tags: string[] | undefined): (typeof WEATHER_TAG_PRIORITY)[number] | undefined {
  return WEATHER_TAG_PRIORITY.find((tag) => hasWeatherTag(tags, tag))
}

function documentedConditionKey(condition: string | null | undefined): string | undefined {
  if (!condition) return undefined
  const key = condition.toLowerCase()
  return Object.prototype.hasOwnProperty.call(WEATHER_CONDITION_COPY, key) ? key : undefined
}

function getConditionTextIcon(condition: string | null | undefined): string {
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

function getConditionIcon(condition: string | null | undefined, tags: string[] | undefined): string {
  if (documentedConditionKey(condition)) return getConditionTextIcon(condition)

  const tag = primaryWeatherTag(tags)
  if (tag) return WEATHER_TAG_ICON[tag]

  return getConditionTextIcon(condition)
}

function localizedWeatherCondition(condition: string | null | undefined, tags: string[] | undefined): string {
  const conditionKey = documentedConditionKey(condition)
  if (conditionKey) {
    const conditionLabel = localizeString(WEATHER_CONDITION_COPY[conditionKey])
    if (conditionLabel) return conditionLabel
  }

  const tag = primaryWeatherTag(tags)
  const tagLabel = tag ? localizeString(WEATHER_TAG_COPY[tag]) : ''
  if (tagLabel) return tagLabel

  return condition ?? ''
}

interface WeatherBadgeCardProps {
  reading: WeatherReading
  displayFields?: string[]
  timezone?: string | null
  variant?: 'inline' | 'context'
}

function WeatherBadgeCard({ reading, displayFields, timezone, variant = 'inline' }: WeatherBadgeCardProps) {
  const { t } = useTranslation()
  const { formatElevation, formatPrecipitation, formatSpeed, formatTemperature } = useUnitFormatters()
  const show = (field: string) => !displayFields || displayFields.includes(field)
  const icon = getConditionIcon(reading.condition, reading.tags)
  const conditionLabel = localizedWeatherCondition(reading.condition, reading.tags)
  const tempDiff =
    reading.temp_c != null && reading.apparent_temp_c != null
      ? Math.abs(reading.temp_c - reading.apparent_temp_c)
      : 0

  return (
    <div
      className={`${variant === 'context' ? 'flex w-full py-2' : 'inline-flex rounded-lg border border-neutral-200 bg-neutral-50 px-3 py-2 dark:border-neutral-700 dark:bg-neutral-800/50'} flex-col gap-0.5 text-sm`}
      title={reading.model ? t('activityOperations.weather.dataSource', { source: reading.model }) : undefined}
    >
      {/* Label and timestamp. */}
      {(reading.label || reading.recorded_at) && (
        <div className="flex items-center gap-1.5 text-xs font-semibold text-neutral-500 dark:text-neutral-400 leading-none mb-0.5">
          {reading.label && <span>{reading.label}</span>}
          {reading.label && reading.recorded_at && <span className="font-normal text-neutral-300 dark:text-neutral-600">·</span>}
          {reading.recorded_at && (
            <span className="font-medium tabular-nums">
              {fmtTime(reading.recorded_at, timezone)}
            </span>
          )}
        </div>
      )}

      {/* Main row: icon, temperature, and condition. */}
      <div className="flex items-center gap-1.5 font-medium text-neutral-800 dark:text-neutral-200">
        <span>{icon}</span>
        {reading.temp_c != null && show('temp_c') && (
          <span>{formatTemperature(reading.temp_c)}</span>
        )}
        {conditionLabel && show('condition') && (
          <span className="text-neutral-500 dark:text-neutral-400 font-normal">
            {conditionLabel}
          </span>
        )}
      </div>

      {/* Apparent temperature. */}
      {reading.apparent_temp_c != null && show('apparent_temp_c') && tempDiff > 1 && (
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          {replaceLegacyMetricUnit(
            t('activityOperations.weather.feelsLike', { value: UNIT_VALUE }),
            formatTemperature(reading.apparent_temp_c),
            '°C',
          )}
        </div>
      )}

      {/* Detail row. */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-neutral-500 dark:text-neutral-400">
        {reading.humidity != null && show('humidity') && (
          <span>💧 {reading.humidity}%</span>
        )}
        {reading.wind_speed_ms != null && show('wind_speed_ms') && (
          <span>
            💨 {formatSpeed(reading.wind_speed_ms)}
            {reading.wind_gusts_ms != null && show('wind_gusts_ms') && (
              <> {t('activityOperations.weather.gusts', { value: formatSpeed(reading.wind_gusts_ms) })}</>
            )}
          </span>
        )}
        {reading.precipitation_mm != null && reading.precipitation_mm > 0 && show('precipitation_mm') && (
          <span>🌧 {formatPrecipitation(reading.precipitation_mm)}</span>
        )}
      </div>

      {/* Freezing level. */}
      {reading.freezing_level_m != null && show('freezing_level_m') && (
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          {replaceLegacyMetricUnit(
            t('activityOperations.weather.freezingLevel', { value: UNIT_VALUE }),
            formatElevation(reading.freezing_level_m),
            'm',
          )}
        </div>
      )}

      {/* UV index, only when high. */}
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
  variant?: 'inline' | 'context'
}

export default function WeatherBadge({ readings, displayFields, className, timezone, variant = 'inline' }: WeatherBadgeProps) {
  if (!readings || readings.length === 0) return null

  return (
    <div className={`${variant === 'context' ? 'flex flex-col divide-y divide-neutral-100 dark:divide-neutral-800' : 'flex flex-wrap gap-1.5'}${className ? ` ${className}` : ''}`}>
      {readings.map((r) => (
        <WeatherBadgeCard key={r.id} reading={r} displayFields={displayFields} timezone={timezone} variant={variant} />
      ))}
    </div>
  )
}

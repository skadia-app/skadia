// frontend/src/lib/sportFieldLabels.ts
//
// Sport-specific field labels, units, and formatters.
// Central source of truth — replaces all duplicated fmtPace/fmtSpeed functions.

import { getSegmentGroup } from '@/lib/sportTaxonomy'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'
import {
  distanceInputUnit,
  elevationInputUnit,
  formatDistance as formatUnitDistance,
  formatElevation as formatUnitElevation,
  formatPace as formatUnitPace,
  formatShortLength as formatUnitShortLength,
  formatSpeed as formatUnitSpeed,
  formatTemperature as formatUnitTemperature,
  paceDisplayUnit,
  speedDisplayUnit,
  temperatureDisplayUnit,
  type UnitFormatOptions,
} from '@/lib/units'

// ── FieldKey ────────────────────────────────────────────────────────────────────

export type FieldKey =
  // Existing chart fields
  | 'altitude' | 'heartRate' | 'power' | 'cadence' | 'speed' | 'temperature'
  // New charts
  | 'groundContactTime' | 'verticalOscillation' | 'stepLength'
  | 'torqueEffectivenessL' | 'torqueEffectivenessR'
  | 'pedalSmoothnessL' | 'pedalSmoothnessR'
  | 'motorPower' | 'batteryLevel' | 'travelRange'
  | 'coreTemperature' | 'depth'
  // MTB
  | 'grit' | 'flow'
  // Garmin Stamina
  | 'potentialStamina' | 'staminaCurrent'
  // Environmental
  | 'pressure'

// ── Pace / Speed Formatter ──────────────────────────────────────────────────────

/** min/km pace for foot activities. speedMs=0 returns '—'. */
export function formatPace(speedMs: number): string {
  if (!speedMs || speedMs <= 0 || !isFinite(speedMs)) return '—'
  const secPerKm = 1000 / speedMs
  const min = Math.floor(secPerKm / 60)
  const sec = Math.floor(secPerKm % 60)
  return `${min}:${sec.toString().padStart(2, '0')} min/km`
}

/** min/100m pace for swimming. speedMs=0 returns '—'. */
export function formatPace100m(speedMs: number): string {
  if (!speedMs || speedMs <= 0 || !isFinite(speedMs)) return '—'
  const secPer100 = 100 / speedMs
  const min = Math.floor(secPer100 / 60)
  const sec = Math.floor(secPer100 % 60)
  return `${min}:${sec.toString().padStart(2, '0')} min/100m`
}

/** min/500m pace for rowing. speedMs=0 returns '—'. */
export function formatPace500m(speedMs: number): string {
  if (!speedMs || speedMs <= 0 || !isFinite(speedMs)) return '—'
  const secPer500 = 500 / speedMs
  const min = Math.floor(secPer500 / 60)
  const sec = Math.floor(secPer500 % 60)
  return `${min}:${sec.toString().padStart(2, '0')} min/500m`
}

/** km/h Speed. speedMs=0 → '—' */
export function formatSpeedKmh(speedMs: number): string {
  if (!speedMs || speedMs <= 0 || !isFinite(speedMs)) return '—'
  return `${(speedMs * 3.6).toFixed(1)} km/h`
}

// ── Speed format per sport ──────────────────────────────────────────────────────

type SpeedFormat = 'decimal' | 'pace' | 'pace100m' | 'pace500m'

function speedFormat(sportType: string): SpeedFormat {
  if (sportType === 'rowing') return 'pace500m'
  const g = getSegmentGroup(sportType)
  if (g === 'foot') return 'pace'
  if (g === 'swim') return 'pace100m'
  return 'decimal'
}

/** Default speed format for a sport: 'speed' or 'pace'. Used for toggle default. */
export function defaultSpeedMode(sportType: string): 'speed' | 'pace' {
  const fmt = speedFormat(sportType)
  return fmt === 'decimal' ? 'speed' : 'pace'
}

/** Format a speed value according to sport type and mode override. */
export function formatSpeedValue(
  speedMs: number | null | undefined,
  sportType: string,
  mode?: 'speed' | 'pace',
  unitOptions?: UnitFormatOptions,
): string {
  if (!speedMs || speedMs <= 0 || !isFinite(speedMs)) return '—'
  const fmt = speedFormat(sportType)
  const effectiveMode = mode ?? (fmt === 'decimal' ? 'speed' : 'pace')

  if (effectiveMode === 'speed') {
    return unitOptions ? formatUnitSpeed(speedMs, unitOptions) : formatSpeedKmh(speedMs)
  }

  // Pace mode — use sport-appropriate pace format
  if (sportType === 'rowing') return formatPace500m(speedMs)
  const g = getSegmentGroup(sportType)
  if (g === 'swim') return formatPace100m(speedMs)
  const secondsPerKm = 1000 / speedMs
  return unitOptions ? formatUnitPace(secondsPerKm, unitOptions) : formatPace(speedMs)
}

// ── Label Maps ──────────────────────────────────────────────────────────────────

interface FieldDef {
  labels: LocalizedString
  unit: string
}

type SegmentGroup = 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic'

const CADENCE_LABELS: Record<SegmentGroup, FieldDef> = {
  bike:    label('Trittfrequenz', 'Cadence', 'Cadence', 'Cadenza', 'Cadencia', 'U/min'),
  foot:    label('Schrittfrequenz', 'Step rate', 'Fréquence de pas', 'Frequenza passi', 'Frecuencia de pasos', 'spm'),
  swim:    label('Zugzahl', 'Stroke rate', 'Fréquence de nage', 'Frequenza bracciate', 'Frecuencia de brazadas', '/min'),
  paddle:  label('Schlagrate', 'Stroke rate', 'Fréquence de coups', 'Frequenza colpi', 'Frecuencia de paladas', '/min'),
  ski:     label('Schrittfrequenz', 'Step rate', 'Fréquence de pas', 'Frequenza passi', 'Frecuencia de pasos', 'spm'),
  generic: label('Kadenz', 'Cadence', 'Cadence', 'Cadenza', 'Cadencia', 'rpm'),
}

const SPEED_LABELS: Record<SegmentGroup, FieldDef> = {
  bike:    label('Geschwindigkeit', 'Speed', 'Vitesse', 'Velocità', 'Velocidad', 'km/h'),
  foot:    label('Pace', 'Pace', 'Allure', 'Passo', 'Ritmo', 'min/km'),
  swim:    label('Pace', 'Pace', 'Allure', 'Passo', 'Ritmo', 'min/100m'),
  paddle:  label('Geschwindigkeit', 'Speed', 'Vitesse', 'Velocità', 'Velocidad', 'km/h'),
  ski:     label('Geschwindigkeit', 'Speed', 'Vitesse', 'Velocità', 'Velocidad', 'km/h'),
  generic: label('Geschwindigkeit', 'Speed', 'Vitesse', 'Velocità', 'Velocidad', 'km/h'),
}

const POWER_LABELS: Record<SegmentGroup, FieldDef> = {
  bike:    label('Leistung', 'Power', 'Puissance', 'Potenza', 'Potencia', 'W'),
  foot:    label('Laufleistung', 'Running power', 'Puissance de course', 'Potenza di corsa', 'Potencia de carrera', 'W'),
  swim:    label('Leistung', 'Power', 'Puissance', 'Potenza', 'Potencia', 'W'),
  paddle:  label('Leistung', 'Power', 'Puissance', 'Potenza', 'Potencia', 'W'),
  ski:     label('Leistung', 'Power', 'Puissance', 'Potenza', 'Potencia', 'W'),
  generic: label('Leistung', 'Power', 'Puissance', 'Potenza', 'Potencia', 'W'),
}

const CYCLES_LABELS: Record<SegmentGroup, FieldDef | null> = {
  bike:    label('Pedalumdrehungen', 'Pedal revolutions', 'Tours de pédale', 'Rotazioni pedale', 'Revoluciones de pedal', ''),
  foot:    label('Schritte', 'Steps', 'Pas', 'Passi', 'Pasos', ''),
  swim:    label('Züge', 'Strokes', 'Mouvements', 'Bracciate', 'Brazadas', ''),
  paddle:  label('Schläge', 'Strokes', 'Coups', 'Colpi', 'Paladas', ''),
  ski:     label('Schritte', 'Steps', 'Pas', 'Passi', 'Pasos', ''),
  generic: null,
}

// Fixed labels (not segment-group dependent)
const FIXED_LABELS: Partial<Record<FieldKey, FieldDef>> = {
  altitude:             label('Höhe', 'Elevation', 'Altitude', 'Altitudine', 'Altitud', 'm'),
  heartRate:            label('Herzfrequenz', 'Heart rate', 'Fréquence cardiaque', 'Frequenza cardiaca', 'Frecuencia cardiaca', 'bpm'),
  temperature:          label('Temperatur', 'Temperature', 'Température', 'Temperatura', 'Temperatura', '°C'),
  groundContactTime:    label('Bodenkontaktzeit', 'Ground contact time', 'Temps de contact au sol', 'Tempo di contatto a terra', 'Tiempo de contacto con el suelo', 'ms'),
  verticalOscillation:  label('Vertikale Bewegung', 'Vertical oscillation', 'Oscillation verticale', 'Oscillazione verticale', 'Oscilación vertical', 'cm'),
  stepLength:           label('Schrittlänge', 'Step length', 'Longueur de pas', 'Lunghezza passo', 'Longitud de paso', 'm'),
  torqueEffectivenessL: label('Drehmoment-Eff. Links', 'Left torque effectiveness', 'Efficacité du couple gauche', 'Efficienza coppia sinistra', 'Efectividad de par izquierda', '%'),
  torqueEffectivenessR: label('Drehmoment-Eff. Rechts', 'Right torque effectiveness', 'Efficacité du couple droite', 'Efficienza coppia destra', 'Efectividad de par derecha', '%'),
  pedalSmoothnessL:     label('Tritt-Gleichm. Links', 'Left pedal smoothness', 'Fluidité pédalage gauche', 'Fluidità pedalata sinistra', 'Suavidad de pedaleo izquierda', '%'),
  pedalSmoothnessR:     label('Tritt-Gleichm. Rechts', 'Right pedal smoothness', 'Fluidité pédalage droite', 'Fluidità pedalata destra', 'Suavidad de pedaleo derecha', '%'),
  motorPower:           label('Motorleistung', 'Motor power', 'Puissance moteur', 'Potenza motore', 'Potencia del motor', 'W'),
  batteryLevel:         label('Akku-Level', 'Battery level', 'Niveau de batterie', 'Livello batteria', 'Nivel de batería', '%'),
  travelRange:          label('Reichweite', 'Range', 'Autonomie', 'Autonomia', 'Autonomía', 'km'),
  coreTemperature:      label('Kerntemperatur', 'Core temperature', 'Température corporelle', 'Temperatura interna', 'Temperatura central', '°C'),
  depth:                label('Tiefe', 'Depth', 'Profondeur', 'Profondità', 'Profundidad', 'm'),
  grit:                 label('MTB Grit', 'MTB Grit', 'MTB Grit', 'MTB Grit', 'MTB Grit', ''),
  flow:                 label('MTB Flow', 'MTB Flow', 'MTB Flow', 'MTB Flow', 'MTB Flow', ''),
  potentialStamina:     label('Stamina (Potential)', 'Stamina (potential)', 'Stamina (potentiel)', 'Stamina (potenziale)', 'Stamina (potencial)', '%'),
  staminaCurrent:       label('Stamina (Aktuell)', 'Stamina (current)', 'Stamina (actuelle)', 'Stamina (attuale)', 'Stamina (actual)', '%'),
  pressure:             label('Barometer', 'Barometer', 'Baromètre', 'Barometro', 'Barómetro', 'hPa'),
}

// ── Special Cases ────────────────────────────────────────────────────────────────

function label(de: string, en: string, fr: string, it: string, es: string, unit: string): FieldDef {
  return { labels: { de, en, fr, it, es }, unit }
}

function localizedLabel(def: FieldDef, language?: SupportedLanguage): string {
  return localizeString(def.labels, language)
}

function getSpeedLabel(sportType: string): FieldDef {
  if (sportType === 'rowing') return label('500-m-Pace', '500 m pace', 'Allure 500 m', 'Passo 500 m', 'Ritmo 500 m', 'min/500m')
  return SPEED_LABELS[getSegmentGroup(sportType)]
}

function getCadenceLabel(sportType: string): FieldDef {
  if (sportType === 'inline_skating') return label('Schrittfrequenz', 'Step rate', 'Fréquence de pas', 'Frequenza passi', 'Frecuencia de pasos', 'spm')
  return CADENCE_LABELS[getSegmentGroup(sportType)]
}

// ── Public API ──────────────────────────────────────────────────────────────────

/** Sport-specific label for a field. */
export function fieldLabel(field: FieldKey, sportType: string, language?: SupportedLanguage): string {
  const fixed = FIXED_LABELS[field]
  if (fixed) return localizedLabel(fixed, language)

  switch (field) {
    case 'cadence': return localizedLabel(getCadenceLabel(sportType), language)
    case 'speed':   return localizedLabel(getSpeedLabel(sportType), language)
    case 'power':   return localizedLabel(POWER_LABELS[getSegmentGroup(sportType)], language)
    default:        return field
  }
}

function speedUnit(sportType: string, unitOptions?: UnitFormatOptions): string {
  const unit = getSpeedLabel(sportType).unit
  if (!unitOptions) return unit
  if (unit === 'km/h') return speedDisplayUnit(unitOptions.units)
  if (unit === 'min/km') return paceDisplayUnit(unitOptions.units)
  return unit
}

function fixedFieldUnit(field: FieldKey, unit: string, unitOptions?: UnitFormatOptions): string {
  if (!unitOptions) return unit
  switch (field) {
    case 'altitude':
    case 'stepLength':
    case 'depth':
      return elevationInputUnit(unitOptions.units)
    case 'temperature':
    case 'coreTemperature':
      return temperatureDisplayUnit(unitOptions.units)
    case 'travelRange':
      return distanceInputUnit(unitOptions.units)
    default:
      return unit
  }
}

/** Sport-specific unit for a field. */
export function fieldUnit(field: FieldKey, sportType: string, unitOptions?: UnitFormatOptions): string {
  const fixed = FIXED_LABELS[field]
  if (fixed) return fixedFieldUnit(field, fixed.unit, unitOptions)

  switch (field) {
    case 'cadence': return getCadenceLabel(sportType).unit
    case 'speed':   return speedUnit(sportType, unitOptions)
    case 'power':   return POWER_LABELS[getSegmentGroup(sportType)].unit
    default:        return ''
  }
}

/** Format a value for the field and sport. Speed uses formatSpeedValue(). */
export function formatFieldValue(
  field: FieldKey,
  value: number | null | undefined,
  sportType: string,
  unitOptions?: UnitFormatOptions,
): string {
  if (value == null || isNaN(value)) return '—'

  switch (field) {
    case 'speed': return formatSpeedValue(value, sportType, undefined, unitOptions)
    case 'cadence': return `${Math.round(value)} ${fieldUnit('cadence', sportType)}`
    case 'power': return `${Math.round(value)} W`
    case 'heartRate': return `${Math.round(value)} bpm`
    case 'altitude': return unitOptions ? formatUnitElevation(value, unitOptions) : `${Math.round(value)} m`
    case 'temperature':
    case 'coreTemperature': return unitOptions ? formatUnitTemperature(value, unitOptions) : `${value.toFixed(1)} °C`
    case 'groundContactTime': return `${Math.round(value)} ms`
    case 'verticalOscillation': return `${(value / 10).toFixed(1)} cm`
    case 'stepLength': return unitOptions ? formatUnitShortLength(value / 1000, unitOptions) : `${(value / 1000).toFixed(2)} m`
    case 'torqueEffectivenessL':
    case 'torqueEffectivenessR':
    case 'pedalSmoothnessL':
    case 'pedalSmoothnessR':
    case 'batteryLevel': return `${value.toFixed(1)} %`
    case 'motorPower': return `${Math.round(value)} W`
    case 'travelRange': return unitOptions ? formatUnitDistance(value * 1000, unitOptions) : `${value.toFixed(1)} km`
    case 'depth': return unitOptions ? formatUnitShortLength(value, unitOptions) : `${value.toFixed(1)} m`
    case 'grit':
    case 'flow': return value.toFixed(2)
    case 'potentialStamina':
    case 'staminaCurrent': return `${Math.round(value)}`
    case 'pressure': return `${value.toFixed(1)} hPa`
    default: return String(value)
  }
}

/** Total-cycles label for StatsGrid. Returns null when not applicable. */
export function cyclesLabel(sportType: string, language?: SupportedLanguage): string | null {
  const def = CYCLES_LABELS[getSegmentGroup(sportType)]
  return def ? localizedLabel(def, language) : null
}

// ── Swim Stroke Labels ──────────────────────────────────────────────────────────

const SWIM_STROKE_LABELS: Record<string, LocalizedString> = {
  freestyle: { de: 'Kraul', en: 'Freestyle', fr: 'Nage libre', it: 'Stile libero', es: 'Crol' },
  backstroke: { de: 'Rücken', en: 'Backstroke', fr: 'Dos', it: 'Dorso', es: 'Espalda' },
  breaststroke: { de: 'Brust', en: 'Breaststroke', fr: 'Brasse', it: 'Rana', es: 'Braza' },
  butterfly: { de: 'Schmetterling', en: 'Butterfly', fr: 'Papillon', it: 'Farfalla', es: 'Mariposa' },
  drill: { de: 'Technik', en: 'Drill', fr: 'Exercice technique', it: 'Tecnica', es: 'Técnica' },
  mixed: { de: 'Gemischt', en: 'Mixed', fr: 'Mixte', it: 'Misto', es: 'Mixto' },
  im: { de: 'Lagen', en: 'Individual medley', fr: '4 nages', it: 'Misti', es: 'Estilos' },
  im_by_round: { de: 'Lagen (Runde)', en: 'Medley (round)', fr: '4 nages (tour)', it: 'Misti (giro)', es: 'Estilos (vuelta)' },
  rimo: { de: 'Lagen (umgekehrt)', en: 'Reverse medley', fr: '4 nages inversé', it: 'Misti inversi', es: 'Estilos inversos' },
}

/** Localized label for a swim stroke. Fallback: raw key. */
export function swimStrokeLabel(stroke?: string, language?: SupportedLanguage): string {
  if (!stroke) return '--'
  return localizeString(SWIM_STROKE_LABELS[stroke], language) || stroke
}

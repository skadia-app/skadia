// frontend/src/lib/sportFieldLabels.ts
//
// Sport-specific field labels, units, and formatters.
// Central source of truth — replaces all duplicated fmtPace/fmtSpeed functions.

import { getSegmentGroup } from '@/lib/sportTaxonomy'

// ── FieldKey ────────────────────────────────────────────────────────────────────

export type FieldKey =
  // Bestehende Chart-Felder
  | 'altitude' | 'heartRate' | 'power' | 'cadence' | 'speed' | 'temperature'
  // Neue Charts
  | 'groundContactTime' | 'verticalOscillation' | 'stepLength'
  | 'torqueEffectivenessL' | 'torqueEffectivenessR'
  | 'pedalSmoothnessL' | 'pedalSmoothnessR'
  | 'motorPower' | 'batteryLevel' | 'travelRange'
  | 'coreTemperature' | 'depth'

// ── Pace / Speed Formatter ──────────────────────────────────────────────────────

/** min/km Pace (Laufen, Wandern, Gehen). speedMs=0 → '—' */
export function formatPace(speedMs: number): string {
  if (!speedMs || speedMs <= 0 || !isFinite(speedMs)) return '—'
  const secPerKm = 1000 / speedMs
  const min = Math.floor(secPerKm / 60)
  const sec = Math.floor(secPerKm % 60)
  return `${min}:${sec.toString().padStart(2, '0')} /km`
}

/** min/100m Pace (Schwimmen). speedMs=0 → '—' */
export function formatPace100m(speedMs: number): string {
  if (!speedMs || speedMs <= 0 || !isFinite(speedMs)) return '—'
  const secPer100 = 100 / speedMs
  const min = Math.floor(secPer100 / 60)
  const sec = Math.floor(secPer100 % 60)
  return `${min}:${sec.toString().padStart(2, '0')} /100m`
}

/** min/500m Pace (Rudern). speedMs=0 → '—' */
export function formatPace500m(speedMs: number): string {
  if (!speedMs || speedMs <= 0 || !isFinite(speedMs)) return '—'
  const secPer500 = 500 / speedMs
  const min = Math.floor(secPer500 / 60)
  const sec = Math.floor(secPer500 % 60)
  return `${min}:${sec.toString().padStart(2, '0')} /500m`
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
export function formatSpeedValue(speedMs: number, sportType: string, mode?: 'speed' | 'pace'): string {
  if (!speedMs || speedMs <= 0 || !isFinite(speedMs)) return '—'
  const fmt = speedFormat(sportType)
  const effectiveMode = mode ?? (fmt === 'decimal' ? 'speed' : 'pace')

  if (effectiveMode === 'speed') return formatSpeedKmh(speedMs)

  // Pace mode — use sport-appropriate pace format
  if (sportType === 'rowing') return formatPace500m(speedMs)
  const g = getSegmentGroup(sportType)
  if (g === 'swim') return formatPace100m(speedMs)
  return formatPace(speedMs) // foot + all other pace modes
}

// ── Label Maps ──────────────────────────────────────────────────────────────────

interface FieldDef {
  label: string
  unit: string
}

type SegmentGroup = 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic'

const CADENCE_LABELS: Record<SegmentGroup, FieldDef> = {
  bike:    { label: 'Trittfrequenz', unit: 'U/min' },
  foot:    { label: 'Schrittfrequenz', unit: 'spm' },
  swim:    { label: 'Zugzahl', unit: '/min' },
  paddle:  { label: 'Schlagrate', unit: '/min' },
  ski:     { label: 'Schrittfrequenz', unit: 'spm' },
  generic: { label: 'Kadenz', unit: 'rpm' },
}

const SPEED_LABELS: Record<SegmentGroup, FieldDef> = {
  bike:    { label: 'Geschwindigkeit', unit: 'km/h' },
  foot:    { label: 'Pace', unit: 'min/km' },
  swim:    { label: 'Pace', unit: 'min/100m' },
  paddle:  { label: 'Geschwindigkeit', unit: 'km/h' },
  ski:     { label: 'Geschwindigkeit', unit: 'km/h' },
  generic: { label: 'Geschwindigkeit', unit: 'km/h' },
}

const POWER_LABELS: Record<SegmentGroup, FieldDef> = {
  bike:    { label: 'Leistung', unit: 'W' },
  foot:    { label: 'Laufleistung', unit: 'W' },
  swim:    { label: 'Leistung', unit: 'W' },
  paddle:  { label: 'Leistung', unit: 'W' },
  ski:     { label: 'Leistung', unit: 'W' },
  generic: { label: 'Leistung', unit: 'W' },
}

const CYCLES_LABELS: Record<SegmentGroup, FieldDef | null> = {
  bike:    { label: 'Pedalumdrehungen', unit: '' },
  foot:    { label: 'Schritte', unit: '' },
  swim:    { label: 'Züge', unit: '' },
  paddle:  { label: 'Schläge', unit: '' },
  ski:     { label: 'Schritte', unit: '' },
  generic: null,
}

// Fixed labels (not segment-group dependent)
const FIXED_LABELS: Partial<Record<FieldKey, FieldDef>> = {
  altitude:             { label: 'Höhe', unit: 'm' },
  heartRate:            { label: 'Herzfrequenz', unit: 'bpm' },
  temperature:          { label: 'Temperatur', unit: '°C' },
  groundContactTime:    { label: 'Bodenkontaktzeit', unit: 'ms' },
  verticalOscillation:  { label: 'Vertikale Bewegung', unit: 'cm' },
  stepLength:           { label: 'Schrittlänge', unit: 'm' },
  torqueEffectivenessL: { label: 'Drehmoment-Eff. Links', unit: '%' },
  torqueEffectivenessR: { label: 'Drehmoment-Eff. Rechts', unit: '%' },
  pedalSmoothnessL:     { label: 'Tritt-Gleichm. Links', unit: '%' },
  pedalSmoothnessR:     { label: 'Tritt-Gleichm. Rechts', unit: '%' },
  motorPower:           { label: 'Motorleistung', unit: 'W' },
  batteryLevel:         { label: 'Akku-Level', unit: '%' },
  travelRange:          { label: 'Reichweite', unit: 'km' },
  coreTemperature:      { label: 'Kerntemperatur', unit: '°C' },
  depth:                { label: 'Tiefe', unit: 'm' },
}

// ── Sonderfälle ─────────────────────────────────────────────────────────────────

function getSpeedLabel(sportType: string): FieldDef {
  if (sportType === 'rowing') return { label: '500-m-Pace', unit: 'min/500m' }
  return SPEED_LABELS[getSegmentGroup(sportType)]
}

function getCadenceLabel(sportType: string): FieldDef {
  if (sportType === 'inline_skating') return { label: 'Schrittfrequenz', unit: 'spm' }
  return CADENCE_LABELS[getSegmentGroup(sportType)]
}

// ── Exportierte API ─────────────────────────────────────────────────────────────

/** Sportspezifisches Label für ein Feld. */
export function fieldLabel(field: FieldKey, sportType: string): string {
  const fixed = FIXED_LABELS[field]
  if (fixed) return fixed.label

  switch (field) {
    case 'cadence': return getCadenceLabel(sportType).label
    case 'speed':   return getSpeedLabel(sportType).label
    case 'power':   return POWER_LABELS[getSegmentGroup(sportType)].label
    default:        return field
  }
}

/** Sportspezifische Einheit für ein Feld. */
export function fieldUnit(field: FieldKey, sportType: string): string {
  const fixed = FIXED_LABELS[field]
  if (fixed) return fixed.unit

  switch (field) {
    case 'cadence': return getCadenceLabel(sportType).unit
    case 'speed':   return getSpeedLabel(sportType).unit
    case 'power':   return POWER_LABELS[getSegmentGroup(sportType)].unit
    default:        return ''
  }
}

/** Formatiert einen Wert sportspezifisch. Für speed: verwendet formatSpeedValue(). */
export function formatFieldValue(field: FieldKey, value: number, sportType: string): string {
  if (value == null || isNaN(value)) return '—'

  switch (field) {
    case 'speed': return formatSpeedValue(value, sportType)
    case 'cadence': return `${Math.round(value)} ${fieldUnit('cadence', sportType)}`
    case 'power': return `${Math.round(value)} W`
    case 'heartRate': return `${Math.round(value)} bpm`
    case 'altitude': return `${Math.round(value)} m`
    case 'temperature':
    case 'coreTemperature': return `${value.toFixed(1)} °C`
    case 'groundContactTime': return `${Math.round(value)} ms`
    case 'verticalOscillation': return `${(value / 10).toFixed(1)} cm`
    case 'stepLength': return `${(value / 1000).toFixed(2)} m`
    case 'torqueEffectivenessL':
    case 'torqueEffectivenessR':
    case 'pedalSmoothnessL':
    case 'pedalSmoothnessR':
    case 'batteryLevel': return `${value.toFixed(1)} %`
    case 'motorPower': return `${Math.round(value)} W`
    case 'travelRange': return `${value.toFixed(1)} km`
    case 'depth': return `${value.toFixed(1)} m`
    default: return String(value)
  }
}

/** Total-Cycles Label für die StatsGrid-Anzeige. Gibt null zurück wenn nicht anwendbar (generic). */
export function cyclesLabel(sportType: string): string | null {
  return CYCLES_LABELS[getSegmentGroup(sportType)]?.label ?? null
}

// ── Swim Stroke Labels ──────────────────────────────────────────────────────────

const SWIM_STROKE_LABELS: Record<string, string> = {
  freestyle: 'Kraul',
  backstroke: 'Rücken',
  breaststroke: 'Brust',
  butterfly: 'Schmetterling',
  drill: 'Technik',
  mixed: 'Gemischt',
  im: 'Lagen',
  im_by_round: 'Lagen (Runde)',
  rimo: 'Lagen (umgekehrt)',
}

/** Deutsches Label für einen Schwimm-Stil. Fallback: raw key. */
export function swimStrokeLabel(stroke?: string): string {
  if (!stroke) return '--'
  return SWIM_STROKE_LABELS[stroke] ?? stroke
}

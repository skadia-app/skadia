import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchSurfaces } from '@/api/enrichment'
import type { ActivitySurface } from '@/api/types'

interface Props {
  activityId: string
}

// Surface grouping: merge similar OSM surface types into display groups
const SURFACE_GROUPS: Record<string, string> = {
  asphalt: 'Asphalt',
  concrete: 'Beton', concrete_lanes: 'Beton', concrete_plates: 'Beton',
  paved: 'Befestigt',
  paving_stones: 'Pflaster', sett: 'Pflaster', cobblestone: 'Pflaster',
  unhewn_cobblestone: 'Pflaster', bricks: 'Pflaster',
  compacted: 'Schotter', fine_gravel: 'Schotter', gravel: 'Schotter', pebblestone: 'Schotter',
  ground: 'Naturweg', earth: 'Naturweg', dirt: 'Naturweg', mud: 'Naturweg', clay: 'Naturweg',
  sand: 'Sand', shells: 'Sand',
  grass: 'Gras', grass_paver: 'Gras',
  wood: 'Holz', woodchips: 'Holz',
  rock: 'Fels',
  metal: 'Metall', metal_grid: 'Metall',
  snow: 'Schnee', ice: 'Schnee',
  tartan: 'Sportbelag', artificial_turf: 'Sportbelag', rubber: 'Sportbelag',
  unpaved: 'Unbefestigt',
  unknown: 'Unbekannt',
  offroad: 'Offroad',
}

const SURFACE_COLORS: Record<string, string> = {
  'Asphalt': '#374151',      // dunkelgrau
  'Beton': '#9ca3af',        // hellgrau
  'Befestigt': '#6b7280',    // mittelgrau
  'Pflaster': '#78716c',     // warmgrau/stein
  'Schotter': '#94a3b8',     // blaugrau (Kalkstein/Schiefer)
  'Naturweg': '#8b6c42',     // erdig-braun
  'Sand': '#dbb778',         // sandgelb
  'Gras': '#4d7c0f',         // natürliches grün
  'Holz': '#92400e',         // holzbraun
  'Fels': '#6b7280',         // felsgrau
  'Metall': '#71717a',       // zinkgrau
  'Schnee': '#e2e8f0',       // fast weiß
  'Sportbelag': '#dc2626',   // rot
  'Unbefestigt': '#a8885a',  // unspezifisch braun
  'Unbekannt': '#d1d5db',    // neutralgrau
  'Offroad': '#b91c1c',      // rot
}

// Labels are the group names themselves — no separate map needed.
// For fallback: unknown raw values show as-is.

const HIGHWAY_COLORS: Record<string, string> = {
  // Asphaltierte Straßen — Grautöne (dunkel → hell nach Bedeutung)
  motorway: '#1f2937',       // fast schwarz
  trunk: '#374151',          // sehr dunkel
  primary: '#c4a30a',        // gedämpftes gelb (Bundesstraßen-Schild)
  secondary: '#6b7280',      // mittelgrau
  tertiary: '#9ca3af',       // hellgrau
  unclassified: '#9ca3af',
  residential: '#d1d5db',    // sehr hell
  living_street: '#d1d5db',
  service: '#9ca3af',
  motorway_link: '#1f2937',
  trunk_link: '#374151',
  primary_link: '#c4a30a',
  secondary_link: '#6b7280',
  tertiary_link: '#9ca3af',
  // Unbefestigte Wege — Erdtöne
  track: '#c2956a',          // sandig-beige (Feldweg)
  path: '#92400e',           // braun (Wanderpfad)
  bridleway: '#854d0e',      // dunkler braun (Reitweg)
  // Rad-/Fußweg — Blautöne (Verkehrsschilder)
  footway: '#1d4ed8',        // dunkelblau (Fußgänger-Schild)
  cycleway: '#2563eb',       // blau (Radweg-Schild)
  pedestrian: '#3b82f6',     // heller blau (Fußgängerzone)
  // Sonstiges
  steps: '#78716c',          // steingrau
  via_ferrata: '#a21caf',    // lila (Klettersteig)
  offroad: '#b91c1c',        // rot
  unknown: '#d1d5db',
}

const HIGHWAY_LABELS: Record<string, string> = {
  primary: 'Bundesstraße',
  secondary: 'Landstraße',
  tertiary: 'Kreisstraße',
  unclassified: 'Gemeindestraße',
  residential: 'Wohnstraße',
  living_street: 'Spielstraße',
  service: 'Zufahrtsweg',
  track: 'Feldweg',
  path: 'Pfad',
  footway: 'Fußweg',
  cycleway: 'Radweg',
  bridleway: 'Reitweg',
  steps: 'Treppen',
  pedestrian: 'Fußgängerzone',
  motorway: 'Schnellstraße',
  trunk: 'Schnellstraße',
  offroad: 'Offroad',
  motorway_link: 'Schnellstraße',
  trunk_link: 'Schnellstraße',
  primary_link: 'Bundesstraße',
  secondary_link: 'Landstraße',
  tertiary_link: 'Kreisstraße',
  via_ferrata: 'Klettersteig',
  unknown: 'Unbekannt',
}

interface AggregatedSurface {
  surface_type: string
  length_m: number
  percentage: number
}

// Always show at least MIN_SHOWN surface types before collapsing to "Sonstige",
// regardless of their percentage. This prevents routes that are 98% asphalt
// from hiding all other surfaces (gravel, compacted, etc.) in a single
// "Sonstige 2%" bucket.
const MAX_DISPLAYED = 7
const MIN_SHOWN = 5

function aggregateSurfaces(raw: ActivitySurface[]): AggregatedSurface[] {
  // Group by display group (merging similar OSM surface types)
  const map = new Map<string, number>()
  for (const s of raw) {
    const group = SURFACE_GROUPS[s.surface_type] ?? s.surface_type
    map.set(group, (map.get(group) ?? 0) + s.length_m)
  }

  const totalLength = Array.from(map.values()).reduce((a, b) => a + b, 0)
  if (totalLength === 0) return []

  // Build sorted list (descending by length)
  const sorted: AggregatedSurface[] = Array.from(map.entries())
    .map(([surface_type, length_m]) => ({
      surface_type,
      length_m,
      percentage: (length_m / totalLength) * 100,
    }))
    .sort((a, b) => b.length_m - a.length_m)

  // Show at least MIN_SHOWN entries, up to MAX_DISPLAYED — remainder → "Sonstige"
  const showCount = Math.min(Math.max(sorted.length, MIN_SHOWN), MAX_DISPLAYED)
  let displayed = sorted.slice(0, showCount)
  const overflow = sorted.slice(showCount)

  const otherLength = overflow.reduce((a, s) => a + s.length_m, 0)
  if (otherLength > 0) {
    displayed = [
      ...displayed,
      {
        surface_type: '__other__',
        length_m: otherLength,
        percentage: (otherLength / totalLength) * 100,
      },
    ]
  }

  return displayed
}

function aggregateHighways(raw: ActivitySurface[]): AggregatedSurface[] {
  // Group by path_type, summing length_m
  const map = new Map<string, number>()
  for (const s of raw) {
    map.set(s.path_type, (map.get(s.path_type) ?? 0) + s.length_m)
  }

  const totalLength = Array.from(map.values()).reduce((a, b) => a + b, 0)
  if (totalLength === 0) return []

  const sorted: AggregatedSurface[] = Array.from(map.entries())
    .map(([surface_type, length_m]) => ({
      surface_type,
      length_m,
      percentage: (length_m / totalLength) * 100,
    }))
    .sort((a, b) => b.length_m - a.length_m)

  const showCount = Math.min(Math.max(sorted.length, MIN_SHOWN), MAX_DISPLAYED)
  let displayed = sorted.slice(0, showCount)
  const overflow = sorted.slice(showCount)

  const otherLength = overflow.reduce((a, s) => a + s.length_m, 0)
  if (otherLength > 0) {
    displayed = [
      ...displayed,
      {
        surface_type: '__other__',
        length_m: otherLength,
        percentage: (otherLength / totalLength) * 100,
      },
    ]
  }

  return displayed
}

function formatLength(m: number): string {
  return m >= 1000 ? `${(m / 1000).toFixed(1)} km` : `${Math.round(m)} m`
}

interface StackedBarProps {
  aggregated: AggregatedSurface[]
  colorMap: Record<string, string>
  labelMap?: Record<string, string>
  title: string
}

function StackedBar({ aggregated, colorMap, labelMap, title }: StackedBarProps) {
  function label(surface_type: string): string {
    if (surface_type === '__other__') return 'Sonstige'
    return labelMap?.[surface_type] ?? surface_type
  }

  function color(surface_type: string): string {
    if (surface_type === '__other__') return '#6b7280'
    return colorMap[surface_type] ?? '#9ca3af'
  }

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400">{title}</h3>
      {/* Stacked bar */}
      <div className="flex h-4 rounded-full overflow-hidden gap-0.5">
        {aggregated.map(s => (
          <div
            key={s.surface_type}
            style={{
              width: `${s.percentage}%`,
              backgroundColor: color(s.surface_type),
            }}
            title={`${label(s.surface_type)}: ${s.percentage.toFixed(0)}%`}
          />
        ))}
      </div>

      {/* Legend */}
      <div className="flex flex-wrap gap-x-4 gap-y-1.5 text-xs text-neutral-600 dark:text-neutral-400">
        {aggregated.map(s => (
          <span key={s.surface_type} className="flex items-center gap-1.5">
            <span
              className="w-2.5 h-2.5 rounded-full inline-block shrink-0"
              style={{ backgroundColor: color(s.surface_type) }}
            />
            <span className="text-neutral-800 dark:text-neutral-200">{label(s.surface_type)}</span>
            <span className="tabular-nums">{s.percentage.toFixed(0)}%</span>
            <span className="text-neutral-400 dark:text-neutral-500">({formatLength(s.length_m)})</span>
          </span>
        ))}
      </div>
    </div>
  )
}

export default function SurfacesSection({ activityId }: Props) {
  const [collapsed, setCollapsed] = useState(false)

  const { data: surfaces = [], isLoading, isError } = useQuery({
    queryKey: ['activity-surfaces', activityId],
    queryFn: () => fetchSurfaces(activityId),
  })

  if (isLoading) return null
  if (isError) return null
  if (surfaces.length === 0) return null

  // Split by category
  const surfaceRows = surfaces.filter(s => s.category === 'surface')
  const highwayRows = surfaces.filter(s => s.category === 'highway')

  // Legacy fallback: old data without category → show everything as a single surface bar
  const isLegacy = surfaceRows.length === 0 && highwayRows.length === 0

  const aggregatedSurfaces = isLegacy
    ? aggregateSurfaces(surfaces)
    : aggregateSurfaces(surfaceRows)
  const aggregatedHighways = isLegacy ? [] : aggregateHighways(highwayRows)

  const hasAnything = aggregatedSurfaces.length > 0 || aggregatedHighways.length > 0
  if (!hasAnything) return null

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 mb-3 group cursor-pointer"
      >
        <svg
          className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
          Wege und Oberflächen
        </h2>
      </button>

      {!collapsed && (
        <div className="space-y-5">
          {aggregatedHighways.length > 0 && (
            <StackedBar
              aggregated={aggregatedHighways}
              colorMap={HIGHWAY_COLORS}
              labelMap={HIGHWAY_LABELS}
              title="Wegearten"
            />
          )}
          {aggregatedSurfaces.length > 0 && (
            <StackedBar
              aggregated={aggregatedSurfaces}
              colorMap={SURFACE_COLORS}
              title={isLegacy ? 'Untergrund' : 'Oberflächen'}
            />
          )}
        </div>
      )}
    </div>
  )
}

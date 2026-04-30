import { useState, useCallback, useEffect, useMemo, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type {
  RouteWaypoint,
  RouteSegment,
  RouteSubSegment,
  PlannedRoute,
  ListRoutesParams,
  RouteSuggestion,
  CoordRange,
  PassedSummit,
  RouteElevationPoint,
} from '@/api/routes'
import {
  getCapabilities,
  listRoutes,
  getRoute,
  createRoute,
  updateRoute,
  computeInternalRoute,
  computeRoute,
  getElevationProfile,
  legacyToRouteProfile,
  type ElevationPoint,
  getProfileDefaults as fetchProfileDefaults,
} from '@/api/routes'
import { getElevationAtVerticesChunked } from '@/api/elevation-chunked'
import { needsMigration, migrateRouteCached } from './migrateRoute'
import type { ProfileConfig } from '@/lib/routingProfiles'
import { getProfileDefaults, cacheServerDefaults } from '@/lib/routingProfiles'
import { useToast } from '@/lib/toast'
import { catmullRom, polylineDistanceM } from '@/lib/catmull-rom'

// ─── Exported types ──────────────────────────────────────────────────────────

/** Identifies a segment by its from/to waypoint IDs. */
export type SegmentKey = { from: string; to: string }

/**
 * Profile-gated namespace returned by useRoutePlanner.
 *
 * - `{ status: 'loading' }` while the server default-fetch (or loadRoute async
 *   profile fetch) is still in-flight.
 * - `{ status: 'ready', ... }` once profileConfig is populated.
 *
 * TypeScript narrows automatically via the discriminant `status`, so consumers
 * can't accidentally access `.config` / `.setConfig` / `.recalculate` /
 * `.saveRoute` before the profile is hydrated.
 */
export type RoutePlannerProfileState =
  | { status: 'loading' }
  | {
      status: 'ready'
      config: ProfileConfig
      /** Convenience: `config.profile` string (e.g. "hiking") */
      name: string
      setConfig: (cfg: ProfileConfig) => void
      setAndRecalculate: (cfg: ProfileConfig) => Promise<void>
      recalculate: () => Promise<void>
      saveRoute: (overrides?: Partial<PlannedRoute>) => void
      saveRouteAsync: (overrides: Partial<PlannedRoute>) => Promise<PlannedRoute>
      isSaving: boolean
      saveError: Error | null
    }

// ─── Internal state ───────────────────────────────────────────────────────────

type RoutingEngine = 'badrouter' | 'brouter'

interface RoutePlannerState {
  waypoints: RouteWaypoint[]
  segments: RouteSegment[]
  profileConfig: ProfileConfig | null
  sportType: string
  subType: string | null
  isElectric: boolean
  isIndoor: boolean
  isVirtual: boolean
  isCommute: boolean
  isCalculating: boolean
  calculationError: string | null
  editingRouteId: string | null
  routingEngine: RoutingEngine
  brouterProfile: string
  // V6-Retrofit: aggregierter Stats-Shape über alle Segmente. null = keine Stats
  // verfügbar (BRouter-Engine, oder Segmente leer).
  routeStats: RouteStats | null
  // _statsCache: per-Segment-Cache mit key = `${from_waypoint}:${to_waypoint}`.
  // NICHT im Return-Object — rein intern. Muss im selben State-Objekt wie
  // segments[] leben, damit setState atomisch bleibt.
  _statsCache: Record<string, RouteStats>
  // Spec Rev 2.3 / §6.2: Zentraler Profil-State, gespeist aus 4 Quellen
  // (Segment-Profile, BRouter-Fallback, Lazy-Fill, geladene Route).
  effectiveProfile: RouteElevationPoint[] | null
  routeSchemaVersion: number | null
  // Lazy-Fill (Phase 5) — bereits hier vorbereitet (Spec §R7).
  lazyFillInProgress: boolean
  // Monoton steigender Counter für Stale-Check parallel laufender loadRoute-Calls.
  lazyFillToken: number
  lazyFillPendingSave: boolean
  // Per-segment fill tokens for offroad DEM fetches (Phase D.2).
  // Key = `from_waypoint:to_waypoint`, value = monotonically increasing counter.
  // Allocated inside setState updater (sequential guarantee, no race).
  fillTokens: Map<string, number>
  // Bug B (2026-04-24): Engine-switch must not recompute when a saved route
  // was loaded and the target engine has no pending profile/setting change.
  // - lastRenderedEngine: engine whose computation produced the current
  //   `segments`. null = not rendered yet (fresh hook, pre-load).
  // - dirtyEngines: per-engine flag set when its profile/settings changed
  //   since the last render. Cleared when that engine renders.
  lastRenderedEngine: RoutingEngine | null
  dirtyEngines: { badrouter: boolean; brouter: boolean }
}

const INITIAL_STATE: RoutePlannerState = {
  waypoints: [],
  segments: [],
  profileConfig: null,
  sportType: 'hiking',
  subType: null,
  isElectric: false,
  isIndoor: false,
  isVirtual: false,
  isCommute: false,
  isCalculating: false,
  calculationError: null,
  editingRouteId: null,
  routingEngine: 'badrouter',
  brouterProfile: 'hiking-mountain',
  routeStats: null,
  _statsCache: {},
  effectiveProfile: null,
  routeSchemaVersion: null,
  lazyFillInProgress: false,
  lazyFillToken: 0,
  lazyFillPendingSave: false,
  fillTokens: new Map(),
  lastRenderedEngine: null,
  dirtyEngines: { badrouter: false, brouter: false },
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// localStorage key for the last selected routing profile. Read on mount to
// hydrate defaults, written whenever the user switches profiles. Convention
// matches `skadia-map-viewport` / `skadia-map-defaults` (see lib/mapStyles.ts,
// pages/MapPage.tsx).
const ROUTING_PROFILE_STORAGE_KEY = 'skadia-routing-profile'
const DEFAULT_ROUTING_PROFILE = 'hiking'

function readStoredProfile(): string {
  try {
    const stored = localStorage.getItem(ROUTING_PROFILE_STORAGE_KEY)
    if (stored && stored.length > 0) return stored
  } catch {
    // localStorage unavailable (SSR / private mode / quota) — fall through.
  }
  return DEFAULT_ROUTING_PROFILE
}

function writeStoredProfile(name: string): void {
  try {
    localStorage.setItem(ROUTING_PROFILE_STORAGE_KEY, name)
  } catch {
    // Ignore — persistence is a UX polish, not critical.
  }
}

function clearStoredProfile(): void {
  try {
    localStorage.removeItem(ROUTING_PROFILE_STORAGE_KEY)
  } catch {
    // Ignore.
  }
}

function assignWaypointTypes(waypoints: RouteWaypoint[]): RouteWaypoint[] {
  if (waypoints.length === 0) return waypoints
  return waypoints.map((w, i) => ({
    ...w,
    type: i === 0 ? 'start' : i === waypoints.length - 1 ? 'end' : 'via',
  }))
}

function buildSegmentFromFeature(
  from: RouteWaypoint,
  to: RouteWaypoint,
  feature: GeoJSON.Feature | undefined,
  isFallback: boolean,
): RouteSegment {
  const props = feature?.properties ?? {}
  // BadRouter uses distance_m/ascent_m/descent_m, BRouter uses track-length/filtered ascend/descend
  const distance = Number(props['distance_m'] ?? props['track-length']) || 0
  // total_distance_m is approach-inclusive (set by BadRouter addApproach); absent on BRouter/offroad.
  const totalDistanceM = props['total_distance_m'] != null ? Number(props['total_distance_m']) : undefined
  const gain = Number(props['ascent_m'] ?? props['filtered ascend']) || 0
  const loss = Number(props['descent_m'] ?? props['filtered descend']) || 0

  const geometry = feature?.geometry as GeoJSON.LineString | undefined
  const approachFrom = props['approach_from'] as number[][] | undefined
  const approachTo = props['approach_to'] as number[][] | undefined
  const subSegments = (props['segments'] as RouteSubSegment[] | undefined) ?? []

  // Backend (services/api/internal/routing/result.go:188-197) emits
  // properties.elevation_profile as Array<{dist_m, ele_m}> per Feature.
  // Without extracting it here, buildCombinedProfileFromSegments returns null
  // and the BRouter-fallback fires even for BadRouter routes (Critic #2).
  const rawProfile = props['elevation_profile']
  let elevationProfile: RouteElevationPoint[] | undefined
  if (Array.isArray(rawProfile) && rawProfile.length > 0) {
    const filtered = rawProfile
      .filter(
        // ele_m may be null (DEM coverage gap — Spec §6 Class A). Pass null through
        // so the chart can render an honest gap rather than interpolating fake heights.
        (pt): pt is { dist_m: number; ele_m: number | null } =>
          pt != null &&
          typeof pt === 'object' &&
          typeof (pt as { dist_m?: unknown }).dist_m === 'number' &&
          ((pt as { ele_m?: unknown }).ele_m === null ||
            typeof (pt as { ele_m?: unknown }).ele_m === 'number'),
      )
      .map(pt => ({ dist_m: pt.dist_m, ele_m: pt.ele_m }))
    if (filtered.length > 0) elevationProfile = filtered
  }

  return {
    from_waypoint: from.id,
    to_waypoint: to.id,
    mode: isFallback ? 'offroad' : 'routed',
    geometry,
    control_points: isFallback ? [from.latlng, to.latlng] : [],
    smooth: true,
    distance_m: distance,
    total_distance_m: totalDistanceM,
    elevation_gain: gain,
    elevation_loss: loss,
    approach_from: approachFrom,
    approach_to: approachTo,
    sub_segments: subSegments,
    elevation_profile: elevationProfile,
  }
}

/** Rebuild geometry for an offroad segment from its control points. */
function rebuildOffroadGeometry(seg: RouteSegment): RouteSegment {
  const cps = seg.control_points ?? []
  const lineCoords = seg.smooth ? catmullRom(cps) : [...cps]
  // Convert to GeoJSON [lng, lat] for geometry
  const geoCoords = lineCoords.map(([lat, lng]) => [lng, lat] as [number, number])
  return {
    ...seg,
    geometry: geoCoords.length >= 2
      ? { type: 'LineString', coordinates: geoCoords }
      : undefined,
    distance_m: polylineDistanceM(lineCoords),
    elevation_estimated: true,
  }
}

// ─── RouteStats types + helpers ───────────────────────────────────────────────

// Zentraler Stats-Shape — wird aus /routing/route-internal (Quelle 1),
// loadSuggestion (Quelle 2) und recalculateAll (Quelle 3) befüllt, auf null
// gesetzt bei Segment-Reset (Quelle 4 — recalculateAll, clear, setRoutingEngine,
// loadWaypoints). Keys 1:1 identisch zu RouteSuggestion-Enrichment (Spec §4.2.4 E).
//
// CoordRange-Shape: {from: number, to: number} (to exclusive) — verifiziert
// in api/routes.ts:375-378. PassedSummit hat kein coord_index-Feld (Position
// via lat/lng, verifiziert api/routes.ts:380-388).
export type RouteStats = {
  forest_pct: number | null
  protected_pct: number | null
  forest_ranges: CoordRange[] | null
  protected_ranges: CoordRange[] | null
  sac_ranges: Record<string, CoordRange[]> | null
  mtb_ranges: Record<string, CoordRange[]> | null
  vf_ranges: Record<string, CoordRange[]> | null
  vf_unknown_ranges: CoordRange[] | null
  summits_passed: PassedSummit[] | null
  // coordinates: Referenz-Coord-Array gegen das alle Range-Indizes aufgelöst
  // werden. Im Multi-Segment-Fall: Concat aller Segment-Coords mit Dedup des
  // ersten Coords jedes Nachfolger-Segments (Spec §4.2.4 E).
  coordinates: number[][]
}

function extractRouteStatsFromFeature(feature: GeoJSON.Feature | undefined): RouteStats | null {
  const props = feature?.properties
  if (!props) return null
  const hasAnyKey =
    'forest_pct' in props || 'protected_pct' in props || 'sac_ranges' in props ||
    'mtb_ranges' in props || 'vf_ranges' in props || 'vf_unknown_ranges' in props ||
    'summits_passed' in props || 'forest_ranges' in props || 'protected_ranges' in props
  if (!hasAnyKey) return null
  const coords = ((feature?.geometry as GeoJSON.LineString | undefined)?.coordinates
    ?? []) as number[][]
  return {
    forest_pct: (props.forest_pct as number) ?? null,
    protected_pct: (props.protected_pct as number) ?? null,
    forest_ranges: (props.forest_ranges as RouteStats['forest_ranges']) ?? null,
    protected_ranges: (props.protected_ranges as RouteStats['protected_ranges']) ?? null,
    sac_ranges: (props.sac_ranges as RouteStats['sac_ranges']) ?? null,
    mtb_ranges: (props.mtb_ranges as RouteStats['mtb_ranges']) ?? null,
    vf_ranges: (props.vf_ranges as RouteStats['vf_ranges']) ?? null,
    vf_unknown_ranges: (props.vf_unknown_ranges as RouteStats['vf_unknown_ranges']) ?? null,
    summits_passed: (props.summits_passed as RouteStats['summits_passed']) ?? null,
    coordinates: coords,
  }
}

/**
 * Build combined elevation profile from per-segment RouteElevationPoint[] arrays.
 *
 * Distanz-Konvention (Spec §10 Q1 + §6.2):
 * - segment.elevation_profile[i].dist_m ist segment-lokal (startet bei 0 pro Segment).
 * - Helper addiert die Profil-letzte-DistM (NICHT seg.distance_m, weil das
 *   approach-inklusiv wäre und gegen das approach-exklusive Profil driftet —
 *   Spec §5.5).
 *
 * Achsen-Diskrepanz (Critic-Finding #9 + #17): Bei Segmenten ohne Profil wird
 * ersatzweise seg.distance_m als Offset verwendet. Für routed-Segmente ist das
 * approach-inklusiv, für offroad approach-exklusiv. Der resultierende Drift ist
 * typischerweise 10–100 m (abhängig von der Approach-Länge), also im Bereich eines
 * einzelnen Elevation-Profile-Sample-Intervalls. Akzeptierter Drift in Phase 1.
 *
 * Phase-4-Folgepunkt (Plan §10 Q1, Hover-Sync): Beheben via per-Segment
 * profile_dist_m-Feld, das die tatsächliche routed Distanz exklusive Approach
 * liefert. Wird in TestBuildCombinedProfileFromSegments_NoProfile_DistanceDrift
 * (noch zu ergänzen) quantitativ abgedeckt.
 *
 * Junction-Dedup: Erstes Profil-Sample pro Folge-Segment wird übersprungen,
 * weil es den letzten Punkt des Vorgänger-Segments dupliziert.
 *
 * Exported for unit tests only.
 */
// Perf (Wave 3, Fix 4): Cost-Analyse — O(W) Map-Build + O(S log S) Sort +
// O(N) Doppelloop über Segments×Profile-Punkte. Ein Push + ein Object pro
// Profilpunkt, keine Allocations in der Inner Loop. Bei 100km / 10 Segmenten /
// 5000 Profilpunkten ≈ <2ms gemessen — keine Architektur-Änderung sinnvoll.
export function buildCombinedProfileFromSegments(
  segments: RouteSegment[],
  waypoints: RouteWaypoint[],
): RouteElevationPoint[] | null {
  // Defensive sort: segments may be temporarily out of waypoint order during
  // recalculation (mirrors combinedCoordinates logic).
  const wpOrder = new Map(waypoints.map((wp, i) => [wp.id, i]))
  const sorted = [...segments].sort(
    (a, b) =>
      (wpOrder.get(a.from_waypoint) ?? Infinity) -
      (wpOrder.get(b.from_waypoint) ?? Infinity),
  )
  const out: RouteElevationPoint[] = []
  let distAccum = 0
  let hadAny = false
  for (const seg of sorted) {
    const p = seg.elevation_profile
    if (!p || p.length === 0) {
      // No profile: still advance distAccum by segment distance for axis continuity.
      distAccum += seg.distance_m ?? 0
      continue
    }
    const startIdx = hadAny ? 1 : 0
    for (let i = startIdx; i < p.length; i++) {
      out.push({ dist_m: distAccum + p[i].dist_m, ele_m: p[i].ele_m })
    }
    // Advance by the profile's last dist_m (approach-exclusive, Spec §5.5).
    distAccum += p[p.length - 1].dist_m
    hadAny = true
  }
  return out.length > 0 ? out : null
}

/** Exported for unit tests only — do not use in production code outside this file. */
export function mergeSegmentStatsForState(
  segments: RouteSegment[],
  cache: Record<string, RouteStats>,
): RouteStats | null {
  if (segments.length === 0) return null

  // Early-out: kein Segment hat Cache-Eintrag → null (BRouter-Gesamtroute)
  const anyStats = segments.some(s => cache[`${s.from_waypoint}:${s.to_waypoint}`])
  if (!anyStats) return null

  // Distance-gewichtete Prozent-Werte (forest_pct, protected_pct).
  let totalDist = 0
  let forestWeighted = 0
  let protectedWeighted = 0

  // Range-Maps mit Coord-Offset-Shift pro Segment.
  const sacMerged: Record<string, CoordRange[]> = {}
  const mtbMerged: Record<string, CoordRange[]> = {}
  const vfMerged: Record<string, CoordRange[]> = {}
  const vfUnknownMerged: CoordRange[] = []
  const forestRangesMerged: CoordRange[] = []
  const protectedRangesMerged: CoordRange[] = []
  const summitsMerged: PassedSummit[] = []
  const coordinatesMerged: number[][] = []

  /** Shift flat CoordRange[] by coordOffset and push into dst. */
  const shiftFlatRanges = (src: CoordRange[] | null, dst: CoordRange[]) => {
    if (!src) return
    for (const r of src) {
      dst.push({ from: r.from + coordOffset, to: r.to + coordOffset })
    }
  }

  let coordOffset = 0
  for (let i = 0; i < segments.length; i++) {
    const seg = segments[i]
    const stats = cache[`${seg.from_waypoint}:${seg.to_waypoint}`]
    const segCoords = (seg.geometry?.coordinates ?? []) as number[][]
    const segLen = segCoords.length

    // Coords mergen mit Dedup: ab Segment 2 ersten Coord überspringen.
    if (i === 0) {
      for (const c of segCoords) coordinatesMerged.push(c)
    } else {
      for (let j = 1; j < segLen; j++) coordinatesMerged.push(segCoords[j])
    }

    // Finding #7 fix: always add seg.distance_m to totalDist so that segments
    // without stats (e.g. BRouter-fallover leg) count as 0 % rather than being
    // excluded from the denominator. This keeps forest_pct / protected_pct
    // proportional to the full route distance, not just the BadRouter portion.
    totalDist += seg.distance_m
    if (stats) {
      if (stats.forest_pct != null) forestWeighted += stats.forest_pct * seg.distance_m
      if (stats.protected_pct != null) protectedWeighted += stats.protected_pct * seg.distance_m

      const shiftRangesInto = (
        src: Record<string, CoordRange[]> | null,
        dst: Record<string, CoordRange[]>,
      ) => {
        if (!src) return
        for (const [key, ranges] of Object.entries(src)) {
          if (!dst[key]) dst[key] = []
          for (const r of ranges) {
            dst[key].push({ from: r.from + coordOffset, to: r.to + coordOffset })
          }
        }
      }
      shiftRangesInto(stats.sac_ranges, sacMerged)
      shiftRangesInto(stats.mtb_ranges, mtbMerged)
      shiftRangesInto(stats.vf_ranges, vfMerged)
      if (stats.vf_unknown_ranges) {
        for (const r of stats.vf_unknown_ranges) {
          vfUnknownMerged.push({ from: r.from + coordOffset, to: r.to + coordOffset })
        }
      }
      shiftFlatRanges(stats.forest_ranges, forestRangesMerged)
      shiftFlatRanges(stats.protected_ranges, protectedRangesMerged)
      if (stats.summits_passed) {
        // Summits dedupliziert: kann an Segment-Grenzen doppelt auftreten.
        // Finding #8 fix: osm_id=0 (DB-Importbug) → Coord-Fallback-Key statt osm_id,
        // sonst werden alle osm_id=0-Gipfel fälschlich als Duplikate behandelt.
        const dedupKey = (sm: { osm_id: number; lat: number; lng: number }) =>
          sm.osm_id > 0 ? String(sm.osm_id) : `${sm.lat.toFixed(5)},${sm.lng.toFixed(5)}`
        const seenKeys = new Set(summitsMerged.map(dedupKey))
        for (const sm of stats.summits_passed) {
          const key = dedupKey(sm)
          if (!seenKeys.has(key)) {
            seenKeys.add(key)
            summitsMerged.push(sm)
          }
        }
      }
    }

    // Coord-Offset fortschalten. Für i==0: Offset wächst um segLen.
    // Für i>0: Offset wächst um segLen-1 (erster Coord wurde geskippt).
    coordOffset += i === 0 ? segLen : Math.max(segLen - 1, 0)
  }

  return {
    forest_pct: totalDist > 0 ? forestWeighted / totalDist : null,
    protected_pct: totalDist > 0 ? protectedWeighted / totalDist : null,
    forest_ranges: forestRangesMerged.length > 0 ? forestRangesMerged : null,
    protected_ranges: protectedRangesMerged.length > 0 ? protectedRangesMerged : null,
    sac_ranges: Object.keys(sacMerged).length > 0 ? sacMerged : null,
    mtb_ranges: Object.keys(mtbMerged).length > 0 ? mtbMerged : null,
    vf_ranges: Object.keys(vfMerged).length > 0 ? vfMerged : null,
    vf_unknown_ranges: vfUnknownMerged.length > 0 ? vfUnknownMerged : null,
    summits_passed: summitsMerged.length > 0 ? summitsMerged : null,
    coordinates: coordinatesMerged,
  }
}

// ─── RouteStats persistence helpers ───────────────────────────────────────────

/**
 * Serialize RouteStats into a JSON-safe object for the enrichment JSONB payload.
 * The `coordinates` field is NOT persisted (it is a runtime index into the
 * combined segment geometry and is rebuilt on load).
 */
function serializeRouteStats(stats: RouteStats): Record<string, unknown> {
  return {
    forest_pct:         stats.forest_pct,
    protected_pct:      stats.protected_pct,
    forest_ranges:      stats.forest_ranges,
    protected_ranges:   stats.protected_ranges,
    sac_ranges:         stats.sac_ranges,
    mtb_ranges:         stats.mtb_ranges,
    vf_ranges:          stats.vf_ranges,
    vf_unknown_ranges:  stats.vf_unknown_ranges,
    summits_passed:     stats.summits_passed,
  }
}

/**
 * Deserialize a stored enrichment JSONB payload back into RouteStats.
 * Returns null if the payload is absent or has an unrecognised version.
 * `coordinates` is set to [] — it is rebuilt from segment geometries at runtime.
 *
 * Used in T5.4 (loadRoute hydration).
 */
export function deserializeRouteStats(
  enrichment: Record<string, unknown> | null | undefined,
): RouteStats | null {
  if (!enrichment || enrichment.enrichment_version !== 1) return null
  return {
    forest_pct:        enrichment.forest_pct as number | null,
    protected_pct:     enrichment.protected_pct as number | null,
    forest_ranges:     enrichment.forest_ranges as RouteStats['forest_ranges'],
    protected_ranges:  enrichment.protected_ranges as RouteStats['protected_ranges'],
    sac_ranges:        enrichment.sac_ranges as RouteStats['sac_ranges'],
    mtb_ranges:        enrichment.mtb_ranges as RouteStats['mtb_ranges'],
    vf_ranges:         enrichment.vf_ranges as RouteStats['vf_ranges'],
    vf_unknown_ranges: enrichment.vf_unknown_ranges as RouteStats['vf_unknown_ranges'],
    summits_passed:    enrichment.summits_passed as RouteStats['summits_passed'],
    coordinates:       [],
  }
}

// ─── Offroad DEM fill helper ──────────────────────────────────────────────────

/** Haversine distance in meters between two [lon, lat] points. */
function haversineDistanceM(a: [number, number], b: [number, number]): number {
  const R = 6371000
  const dLat = (b[1] - a[1]) * Math.PI / 180
  const dLon = (b[0] - a[0]) * Math.PI / 180
  const sinDLat = Math.sin(dLat / 2)
  const sinDLon = Math.sin(dLon / 2)
  const h =
    sinDLat * sinDLat +
    Math.cos(a[1] * Math.PI / 180) * Math.cos(b[1] * Math.PI / 180) * sinDLon * sinDLon
  return 2 * R * Math.asin(Math.sqrt(h))
}

/** Insert intermediate vertices along each polyline edge so the maximum gap
 *  between consecutive vertices is ≤ stepM. Endpoints are preserved exactly.
 *  No-op when the input has < 2 coordinates. */
function densifyLineString(
  line: GeoJSON.LineString,
  stepM: number,
): GeoJSON.LineString {
  const coords = line.coordinates as [number, number][]
  if (coords.length < 2 || stepM <= 0) return line

  const out: [number, number][] = [coords[0]]
  for (let i = 1; i < coords.length; i++) {
    const a = coords[i - 1]
    const b = coords[i]
    const segM = haversineDistanceM(a, b)
    if (segM > stepM) {
      const subdivisions = Math.ceil(segM / stepM)
      for (let k = 1; k < subdivisions; k++) {
        const t = k / subdivisions
        out.push([a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t])
      }
    }
    out.push(b)
  }
  return { type: 'LineString', coordinates: out }
}

const OFFROAD_DENSIFY_STEP_M = 25

/**
 * Pure async helper: densifies an offroad segment's polyline to ≤25m vertex
 * spacing, samples DEM elevation at every (densified) vertex, and returns the
 * segment with both the dense geometry AND elevation_profile populated. The
 * dense geometry is what the map and the hover-coord-map consume so the
 * profile↔coord 1:1 invariant holds and hover tracks the user's offroad line
 * smoothly instead of jumping between the user's 2-3 click points.
 *
 * Returns the segment unchanged when:
 * - mode !== 'offroad' (no-op for routed segments)
 * - geometry is missing (segment not yet built)
 *
 * The addToast callback is forwarded to getElevationAtVerticesChunked so the
 * DEM-coverage warning (Spec §6 Class B) fires at most once per session.
 * Pass null to suppress (e.g. tests without a toast provider).
 */
async function fillOffroadSegmentProfile(
  seg: RouteSegment,
  addToast: Parameters<typeof getElevationAtVerticesChunked>[1],
): Promise<RouteSegment> {
  if (seg.mode !== 'offroad' || !seg.geometry) return seg
  const denseGeometry = densifyLineString(seg.geometry, OFFROAD_DENSIFY_STEP_M)
  const profile = await getElevationAtVerticesChunked(denseGeometry, addToast)
  // ele_m may be null (DEM coverage gap — Phase A.0 nullable); pass through as-is.
  return { ...seg, geometry: denseGeometry, elevation_profile: profile }
}

// ─── Hook ─────────────────────────────────────────────────────────────────────

export function useRoutePlanner() {
  const [state, setState] = useState<RoutePlannerState>(INITIAL_STATE)
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const loadedRouteRef = useRef<PlannedRoute | null>(null)

  // Bug B (2026-04-25): tracks which source produced the current
  // state.effectiveProfile ('segments' = full per-segment aggregate,
  // 'dem-fallback' = 200-sample DEM resampling, 'none' = no profile yet).
  // Used by the effectiveProfile-sync useEffect to reject regressions where
  // a shorter dem-fallback would overwrite a longer segments-based profile.
  const _prevSourceRef = useRef<'segments' | 'dem-fallback' | 'none'>('none')

  // Ref to fillOffroadAndApply so _calculateSegment (stable [] deps) can call it
  // without needing it as a dep (avoids recreating the calculate callback on
  // every addToast identity change).
  const _fillOffroadRef = useRef<((seg: RouteSegment) => Promise<void>) | null>(null)

  // Per-segment fill-token counters. Allocated synchronously (ref, not state)
  // so that token assignment never writes to outer variables inside a setState
  // updater — which React StrictMode prohibits (updaters may run twice).
  // The allocated value is then mirrored into state so the race-guard in the
  // completion updater can compare against s.fillTokens without an extra ref read.
  const _fillTokenCountersRef = useRef<Map<string, number>>(new Map())

  // Always-current state snapshot for callbacks that need synchronous reads
  // (updateControlPoint / insertControlPoint / removeControlPoint) without
  // relying on React's batching timing for the setState-updater-capture pattern.
  const _stateRef = useRef(state)
  _stateRef.current = state

  // ── Scale-Pin + Hover state (SAC / MTB / Via Ferrata) ────────────────────────
  // scalePin: persistente Auswahl (Click auf Badge, bleibt bis erneuter Click
  // oder Reset). scaleHover: transient (Badge-Hover, clearet bei mouseleave).
  // scaleEffective = scalePin ?? scaleHover — ein gepinneter Badge gewinnt,
  // sonst wird der gehoverte Badge als Preview angezeigt. Consumers (MapPage,
  // ScaleLegend) lesen scaleEffective statt scalePin, damit Hover einen
  // Map-Preview inkl. Legende liefert ohne dass der User klicken muss.
  const [scalePin, setScalePin] = useState<'sac' | 'mtb' | 'vf' | null>(null)
  const [scaleHover, setScaleHover] = useState<'sac' | 'mtb' | 'vf' | null>(null)

  const togglePin = useCallback((kind: 'sac' | 'mtb' | 'vf') => {
    setScalePin(prev => prev === kind ? null : kind)
  }, [])

  const resetScalePin = useCallback(() => {
    setScalePin(null)
    setScaleHover(null)
  }, [])

  const scaleEffective = scalePin ?? scaleHover

  // ── Mount-fetch: hydrate hiking defaults from server ─────────────────────────

  useEffect(() => {
    if (state.profileConfig !== null) return

    const storedProfile = readStoredProfile()

    const hydrate = (profileName: string): Promise<void> =>
      fetchProfileDefaults(profileName).then(resp => {
        cacheServerDefaults(profileName, resp)
        let didPopulate = false
        setState(s => {
          // Race-guard: another flow (e.g. loadRoute for deeplink) may have
          // populated profileConfig between this useEffect firing and the
          // fetch resolving. Don't overwrite.
          if (s.profileConfig !== null) return s
          const fresh = getProfileDefaults(profileName)
          if (!fresh) return s
          didPopulate = true
          return { ...s, profileConfig: fresh }
        })
        // Finding #10: if waypoints were added before profileConfig was ready,
        // they have no segments yet — recalculate now that we have a profile.
        // Use a microtask so setState above has committed before we read state.
        if (didPopulate) {
          void Promise.resolve().then(() => {
            // recalculateAll reads state via its own setState-updater, so it
            // always sees the freshly committed profileConfig.
            recalculateAll()
          })
        }
      })

    hydrate(storedProfile)
      .catch(() => {
        // Stored profile missing/renamed server-side — clear the stale entry
        // and retry with the hardcoded default. Only then surface a toast.
        if (storedProfile === DEFAULT_ROUTING_PROFILE) {
          throw new Error('default profile fetch failed')
        }
        clearStoredProfile()
        return hydrate(DEFAULT_ROUTING_PROFILE)
      })
      .catch(() => {
        addToast({
          type: 'error',
          title: 'Profil-Defaults konnten nicht geladen werden',
          message: 'Bitte Seite neu laden.',
          duration: 0,
        })
      })
    // Deps intentionally empty: this is a one-shot mount hydration. addToast is
    // stable (Context ref), and fetchProfileDefaults / cacheServerDefaults /
    // getProfileDefaults are module-level imports. Re-firing on every render
    // would cause a fetch storm.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── Capabilities ────────────────────────────────────────────────────────────

  const capabilitiesQuery = useQuery({
    queryKey: ['routing-capabilities'],
    queryFn: getCapabilities,
    staleTime: 30 * 1000, // 30s — matches backend cache TTL
    retry: 1,
  })

  // Set default engine based on capabilities: if routing=false, fall back to brouter
  useEffect(() => {
    if (capabilitiesQuery.data && !capabilitiesQuery.data.routing) {
      setState(s => {
        if (s.routingEngine === 'badrouter') {
          return { ...s, routingEngine: 'brouter' }
        }
        return s
      })
    }
  }, [capabilitiesQuery.data])

  // ── Route list ──────────────────────────────────────────────────────────────

  function useRouteList(params?: ListRoutesParams) {
    return useQuery({
      queryKey: ['routes', params],
      queryFn: () => listRoutes(params),
    })
  }

  // ── Route load ──────────────────────────────────────────────────────────────

  function useRouteDetail(id: string | null) {
    return useQuery({
      queryKey: ['route', id],
      queryFn: () => getRoute(id!),
      enabled: id !== null,
    })
  }

  // ── Segment calculation ─────────────────────────────────────────────────────

  // Internal helper — wrapped in useCallback so callers (recalculateAll, etc.) get a stable reference
  // MUST be declared before addWaypoint/moveWaypoint which reference it
  const _calculateSegment = useCallback(async (
    from: RouteWaypoint,
    to: RouteWaypoint,
    profileConfig: ProfileConfig,
    engine: RoutingEngine = 'badrouter',
    brouterProf: string = 'hiking-mountain',
  ): Promise<void> => {
    setState(s => ({ ...s, isCalculating: true, calculationError: null }))
    try {
      const result =
        engine === 'brouter'
          ? await computeRoute([from.latlng, to.latlng], brouterProf)
          : await computeInternalRoute(from.latlng, to.latlng, profileConfig)
      const feature = result.features?.[0]
      const props = feature?.properties ?? {}
      if (props.status === 'no_route') {
        setState(s => ({
          ...s,
          isCalculating: false,
          calculationError: 'no_route',
        }))
        return
      }
      const isFallback = feature?.properties?.fallback === true

      const segment = buildSegmentFromFeature(from, to, feature, isFallback)

      // V6-Retrofit Quelle 1: Stats aus Response extrahieren. extractRouteStatsFromFeature
      // hat einen hasAnyKey-Guard und returnt null, wenn die Response keine Enrichment-
      // Keys enthält (BRouter-Proxy aktuell, Spec §3 Non-Goal + §4.1.5b). Kein Engine-Gate
      // hier — die Funktion ist self-guarding. Hinweis: der hasAnyKey-Check sucht nach
      // hart-kodierten snake_case-Keys (forest_pct, sac_ranges etc.); falls BRouter je Stats
      // emittieren soll, müsste extractRouteStatsFromFeature an das konkrete Key-Schema
      // angepasst werden — "robust gegen zukünftige Backend-Änderungen" gilt nur solange
      // die Key-Namen konsistent bleiben.
      const statsForSegment = extractRouteStatsFromFeature(feature)
      const segmentKey = `${from.id}:${to.id}`

      setState(s => {
        // Replace existing segment between these two waypoints, or append
        const idx = s.segments.findIndex(
          seg =>
            seg.from_waypoint === from.id && seg.to_waypoint === to.id,
        )
        const rawSegments =
          idx >= 0
            ? s.segments.map((seg, i) => (i === idx ? segment : seg))
            : [...s.segments, segment]

        // Sort by waypoint order to guard against concurrent fetch race conditions
        // (addWaypoint fires multiple _calculateSegment calls in parallel; the
        // fastest response wins the append race and can land out of order).
        const wpOrder = new Map(s.waypoints.map((wp, i) => [wp.id, i]))
        const newSegments = rawSegments
          .slice()
          .sort((a, b) => (wpOrder.get(a.from_waypoint) ?? Infinity) - (wpOrder.get(b.from_waypoint) ?? Infinity))

        // _statsCache atomar mit-updaten
        const newCache = { ...s._statsCache }
        if (statsForSegment) {
          newCache[segmentKey] = statsForSegment
        } else {
          delete newCache[segmentKey]
        }

        // Bug A fix (2026-04-25): Stats-Erhaltung bei geladenen Routen.
        // RouteSegment-Type (api/routes.ts:19-34) persistiert KEINE Per-Segment-
        // Stats — das aggregierte routeStats kommt aus route.enrichment JSONB.
        // Wenn der User nach loadRoute einen Wegpunkt verschiebt/hinzufügt, hat
        // _statsCache nur den NEU berechneten Eintrag, die anderen Segmente sind
        // im Cache nicht vertreten. mergeSegmentStatsForState würde dann nur die
        // Stats des neuen Segments zurückgeben → Gipfel/Forest/SAC anderer
        // Segmente verschwinden aus dem Side-Panel.
        //
        // Schutzregel: wenn (a) ein deserialisiertes baseStats existiert (s.routeStats != null)
        // UND (b) der Cache NICHT für alle Segmente Einträge hat → das alte
        // routeStats bleibt erhalten (best-effort: lieber leicht stale Aggregat
        // als komplett verlorenes Enrichment). Sobald der User recalculateAll()
        // triggert oder alle Segmente einzeln neu berechnet werden, ist der
        // Cache vollständig und der Merge übernimmt wieder.
        const allSegmentsInCache = newSegments.every(
          seg => newCache[`${seg.from_waypoint}:${seg.to_waypoint}`],
        )
        const mergedStats = mergeSegmentStatsForState(newSegments, newCache)
        const nextRouteStats = allSegmentsInCache || s.routeStats === null
          ? mergedStats
          : s.routeStats

        return {
          ...s,
          segments: newSegments,
          _statsCache: newCache,
          routeStats: nextRouteStats,
          isCalculating: false,
          // Bug B: rendered geometry now reflects this engine — mark it as
          // the rendered engine and clear its dirty-flag.
          lastRenderedEngine: engine,
          dirtyEngines: { ...s.dirtyEngines, [engine]: false },
        }
      })

      // Phase D.2 — Trigger 1: kick off DEM fill for fallback (offroad) segments.
      // The segment geometry was just stored in state; fill runs concurrently.
      if (isFallback && segment.geometry) {
        void Promise.resolve().then(() => {
          _fillOffroadRef.current?.(segment)
        })
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Routenberechnung fehlgeschlagen'
      setState(s => ({
        ...s,
        isCalculating: false,
        calculationError: message,
      }))
    }
  // setState from useState is stable
  }, [])

  // ── Offroad DEM fill (Phase D.2) ─────────────────────────────────────────────
  //
  // Per-segment token counter: fillTokens Map<segKey, counter> prevents stale
  // in-flight results from landing when the same segment was re-edited.
  //
  // Token allocation is done via _fillTokenCountersRef (synchronous, outside
  // setState) so that no side-effect writes to an outer variable inside a pure
  // setState updater. The allocated value is then mirrored into state so that
  // the race-guard in the completion updater can read s.fillTokens normally.

  const fillOffroadAndApply = useCallback(async (seg: RouteSegment) => {
    const segKey = `${seg.from_waypoint}:${seg.to_waypoint}`

    // Allocate token from the ref — synchronous, StrictMode-safe.
    const allocatedToken = (_fillTokenCountersRef.current.get(segKey) ?? 0) + 1
    _fillTokenCountersRef.current.set(segKey, allocatedToken)

    // Mirror into state so the completion updater can use s.fillTokens for the
    // race-guard check (pure updater — no outer-variable writes).
    setState(s => {
      const fillTokens = new Map(s.fillTokens)
      fillTokens.set(segKey, allocatedToken)
      return { ...s, fillTokens }
    })

    const filled = await fillOffroadSegmentProfile(seg, addToast)

    setState(s => {
      // Race-Guard: discard if a newer fill for the same segment was started.
      if (s.fillTokens.get(segKey) !== allocatedToken) return s
      // Segment-Identity-Check: segment may have been replaced (e.g. user removed
      // and re-added a waypoint while the fetch was in-flight).
      const idx = s.segments.findIndex(
        x => x.from_waypoint === seg.from_waypoint && x.to_waypoint === seg.to_waypoint,
      )
      if (idx === -1) return s
      const segments = [...s.segments]
      segments[idx] = filled
      return { ...s, segments }
    })
  }, [addToast])

  // Keep the ref in sync so _calculateSegment can call it without a dep.
  _fillOffroadRef.current = fillOffroadAndApply

  // ── Waypoint management ─────────────────────────────────────────────────────

  const addWaypoint = useCallback(
    async (
      latlng: [number, number],
      poi?: { id: number; name: string; type: string },
    ) => {
      const newWp: RouteWaypoint = {
        id: crypto.randomUUID(),
        latlng,
        type: 'end', // will be reassigned below
        poi_id: poi?.id,
        poi_name: poi?.name,
        poi_type: poi?.type,
      }

      // Read current state synchronously via setState updater, then trigger
      // the segment calculation inside the updater itself (guaranteed to see
      // the latest state, even with React 19 concurrent batching).
      setState(s => {
        const updated = assignWaypointTypes([...s.waypoints, newWp])
        if (updated.length >= 2 && s.profileConfig) {
          const prevWp = updated[updated.length - 2]
          const config = s.profileConfig
          // Fire segment calculation outside the render cycle via microtask
          void Promise.resolve().then(() =>
            _calculateSegment(prevWp, newWp, config, s.routingEngine, s.brouterProfile),
          )
        }
        return { ...s, waypoints: updated }
      })
    },
    [_calculateSegment],
  )

  const removeWaypoint = useCallback((wpId: string) => {
    setState(s => {
      const newWaypoints = assignWaypointTypes(
        s.waypoints.filter(w => w.id !== wpId),
      )
      // Remove segments that involved this waypoint
      const newSegments = s.segments.filter(
        seg => seg.from_waypoint !== wpId && seg.to_waypoint !== wpId,
      )
      // V6-Retrofit Quelle 4: invalidate cached stats — stale after waypoint removal.
      // Next recalculate() will repopulate via Quelle 1.
      return { ...s, waypoints: newWaypoints, segments: newSegments, routeStats: null, _statsCache: {} }
    })
    // Note: adjacent segment recalculation is triggered by the caller component
    // when it calls recalculateSegmentsAfterRemoval() below.
  }, [])

  const moveWaypoint = useCallback(
    async (wpId: string, newLatlng: [number, number]) => {
      setState(s => {
        const newWaypoints = s.waypoints.map(w =>
          w.id === wpId ? { ...w, latlng: newLatlng } : w,
        )
        const idx = newWaypoints.findIndex(w => w.id === wpId)
        if (idx !== -1) {
          const pairs: Array<[RouteWaypoint, RouteWaypoint]> = []
          if (idx > 0) pairs.push([newWaypoints[idx - 1], newWaypoints[idx]])
          if (idx < newWaypoints.length - 1) pairs.push([newWaypoints[idx], newWaypoints[idx + 1]])
          // Fire calculation as microtask (React 19 safe)
          if (s.profileConfig) {
            const config = s.profileConfig
            void Promise.resolve().then(async () => {
              for (const [from, to] of pairs) {
                await _calculateSegment(from, to, config, s.routingEngine, s.brouterProfile)
              }
            })
          }
        }
        return { ...s, waypoints: newWaypoints }
      })
    },
    [_calculateSegment],
  )

  const reorderWaypoints = useCallback((newOrder: RouteWaypoint[]) => {
    setState(s => {
      const retyped = assignWaypointTypes(newOrder)
      // V6-Retrofit Quelle 4: segments + stats invalid after reorder — full recalc needed.
      return { ...s, waypoints: retyped, segments: [], routeStats: null, _statsCache: {} }
    })
  }, [])

  /** Insert a via waypoint after `afterId`, removing only the affected segment. */
  const insertViaWaypoint = useCallback((afterId: string, newWp: RouteWaypoint) => {
    setState(s => {
      const idx = s.waypoints.findIndex(w => w.id === afterId)
      if (idx < 0 || idx >= s.waypoints.length - 1) return s
      const nextWp = s.waypoints[idx + 1]
      const newWaypoints = assignWaypointTypes([
        ...s.waypoints.slice(0, idx + 1),
        newWp,
        ...s.waypoints.slice(idx + 1),
      ])
      // Remove only the segment between afterId and nextWp (it will be replaced by two new ones)
      const newSegments = s.segments.filter(
        seg => !(seg.from_waypoint === afterId && seg.to_waypoint === nextWp.id),
      )
      // V6-Retrofit Quelle 4: evict stats cache — affected segment is gone,
      // two new segments will be calculated and will repopulate via Quelle 1.
      const newCache = { ...s._statsCache }
      delete newCache[`${afterId}:${nextWp.id}`]
      return { ...s, waypoints: newWaypoints, segments: newSegments, routeStats: null, _statsCache: newCache }
    })
  }, [])

  const calculateSegment = useCallback(
    (from: RouteWaypoint, to: RouteWaypoint) => {
      if (!state.profileConfig) return Promise.resolve()
      return _calculateSegment(from, to, state.profileConfig, state.routingEngine, state.brouterProfile)
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [state.profileConfig, state.routingEngine, state.brouterProfile],
  )

  const recalculateAll = useCallback(async () => {
    resetScalePin()
    let currentWaypoints: RouteWaypoint[] = []
    let currentProfileConfig: ProfileConfig | null = null
    let currentEngine: RoutingEngine = 'badrouter'
    let currentBrouterProfile = 'hiking-mountain'
    setState(s => {
      currentWaypoints = s.waypoints
      currentProfileConfig = s.profileConfig
      currentEngine = s.routingEngine
      currentBrouterProfile = s.brouterProfile
      // Finding #9 fix (Option b): do NOT clear existing segments here.
      // Keeps DB-loaded segments visible while re-routing is in-flight, preventing
      // the visible flash (route → blank → route). _calculateSegment replaces each
      // segment in-place via findIndex+map (Quelle 1 path).
      // Stats cache IS cleared: will be repopulated per-segment by _calculateSegment.
      // Bug #4 fix: mark the active engine dirty BEFORE kicking off the recompute,
      // so the invariant "dirty=true iff this engine has uncomputed pending changes"
      // holds during the entire in-flight window. _calculateSegment clears it on completion.
      return {
        ...s,
        isCalculating: true,
        calculationError: null,
        routeStats: null,
        _statsCache: {},
        dirtyEngines: { ...s.dirtyEngines, [s.routingEngine]: true },
      }
    })
    if (!currentProfileConfig) {
      setState(s => ({ ...s, isCalculating: false }))
      return
    }
    for (let i = 0; i < currentWaypoints.length - 1; i++) {
      await _calculateSegment(currentWaypoints[i], currentWaypoints[i + 1], currentProfileConfig, currentEngine, currentBrouterProfile)
    }
    setState(s => ({ ...s, isCalculating: false }))
  }, [_calculateSegment])

  // ── Offroad toggle ──────────────────────────────────────────────────────────

  const toggleOffroad = useCallback((key: SegmentKey) => {
    type RecalcArgs = { from: RouteWaypoint; to: RouteWaypoint; config: ProfileConfig; engine: RoutingEngine; profile: string }
    let needsRecalc: RecalcArgs | null = null

    setState(s => {
      const segments = [...s.segments]
      const idx = segments.findIndex(
        seg => seg.from_waypoint === key.from && seg.to_waypoint === key.to,
      )
      if (idx < 0) return s
      const seg = { ...segments[idx] }
      if (seg.mode === 'routed') {
        seg.mode = 'offroad'
        const fromWp = s.waypoints.find(w => w.id === seg.from_waypoint)
        const toWp = s.waypoints.find(w => w.id === seg.to_waypoint)
        seg.control_points =
          fromWp && toWp ? [fromWp.latlng, toWp.latlng] : seg.control_points
        seg.geometry = undefined
        segments[idx] = rebuildOffroadGeometry(seg)
      } else {
        seg.mode = 'routed'
        seg.control_points = []
        segments[idx] = seg
        // Capture values for side effect outside setState
        const fromWp = s.waypoints.find(w => w.id === seg.from_waypoint)
        const toWp = s.waypoints.find(w => w.id === seg.to_waypoint)
        if (fromWp && toWp && s.profileConfig) {
          needsRecalc = { from: fromWp, to: toWp, config: s.profileConfig, engine: s.routingEngine, profile: s.brouterProfile }
        }
      }
      return { ...s, segments }
    })

    // Side-effect OUTSIDE setState
    if (needsRecalc) {
      const { from, to, config, engine, profile } = needsRecalc as RecalcArgs
      void _calculateSegment(from, to, config, engine, profile)
    }
  }, [_calculateSegment])

  // ── Control point CRUD ──────────────────────────────────────────────────────

  const updateControlPoint = useCallback((key: SegmentKey, cpIndex: number, latlng: [number, number]) => {
    // Read current state synchronously from ref to avoid relying on React's batching
    // timing for the setState-updater-capture pattern (which is unreliable in React 19
    // when multiple setState calls are batched in the same tick).
    const s = _stateRef.current
    const idx = s.segments.findIndex(
      seg => seg.from_waypoint === key.from && seg.to_waypoint === key.to,
    )
    if (idx < 0) return
    const seg = { ...s.segments[idx] }
    const cps = [...(seg.control_points ?? [])]
    if (cpIndex < 0 || cpIndex >= cps.length) return
    cps[cpIndex] = latlng
    seg.control_points = cps
    const updatedSeg = rebuildOffroadGeometry(seg)
    setState(prev => {
      // Re-validate inside updater: the segment must still exist at the same position.
      const i = prev.segments.findIndex(
        sg => sg.from_waypoint === key.from && sg.to_waypoint === key.to,
      )
      if (i < 0) return prev
      const segments = [...prev.segments]
      segments[i] = updatedSeg
      return { ...prev, segments }
    })
    // Phase D.2 — Trigger 2: DEM fill after control point drag (geometry changed).
    void Promise.resolve().then(() => { _fillOffroadRef.current?.(updatedSeg) })
  }, [])

  const insertControlPoint = useCallback((key: SegmentKey, afterCpIndex: number, latlng: [number, number]) => {
    const s = _stateRef.current
    const idx = s.segments.findIndex(
      seg => seg.from_waypoint === key.from && seg.to_waypoint === key.to,
    )
    if (idx < 0) return
    const seg = { ...s.segments[idx] }
    const cps = [...(seg.control_points ?? [])]
    cps.splice(afterCpIndex + 1, 0, latlng)
    seg.control_points = cps
    const updatedSeg = rebuildOffroadGeometry(seg)
    setState(prev => {
      const i = prev.segments.findIndex(
        sg => sg.from_waypoint === key.from && sg.to_waypoint === key.to,
      )
      if (i < 0) return prev
      const segments = [...prev.segments]
      segments[i] = updatedSeg
      return { ...prev, segments }
    })
    // Phase D.2 — Trigger 2: DEM fill after control point insert.
    void Promise.resolve().then(() => { _fillOffroadRef.current?.(updatedSeg) })
  }, [])

  const removeControlPoint = useCallback((key: SegmentKey, cpIndex: number) => {
    const s = _stateRef.current
    const idx = s.segments.findIndex(
      seg => seg.from_waypoint === key.from && seg.to_waypoint === key.to,
    )
    if (idx < 0) return
    const seg = { ...s.segments[idx] }
    const cps = [...(seg.control_points ?? [])]
    if (cpIndex < 0 || cpIndex >= cps.length) return
    cps.splice(cpIndex, 1)
    seg.control_points = cps
    const updatedSeg = rebuildOffroadGeometry(seg)
    setState(prev => {
      const i = prev.segments.findIndex(
        sg => sg.from_waypoint === key.from && sg.to_waypoint === key.to,
      )
      if (i < 0) return prev
      const segments = [...prev.segments]
      segments[i] = updatedSeg
      return { ...prev, segments }
    })
    // Phase D.2 — Trigger 2: DEM fill after control point removal.
    void Promise.resolve().then(() => { _fillOffroadRef.current?.(updatedSeg) })
  }, [])

  // ── Profile ─────────────────────────────────────────────────────────────────

  const setProfile = useCallback((profileConfig: ProfileConfig) => {
    setState(s => ({ ...s, profileConfig }))
    writeStoredProfile(profileConfig.profile)
  }, [])

  const setProfileAndRecalculate = useCallback(
    async (profileConfig: ProfileConfig) => {
      let currentWaypoints: RouteWaypoint[] = []
      let currentEngine: RoutingEngine = 'badrouter'
      let currentBrouterProfile = 'hiking-mountain'
      setState(s => {
        currentWaypoints = s.waypoints
        currentEngine = s.routingEngine
        currentBrouterProfile = s.brouterProfile
        // Bug #4 fix: mark the active engine dirty BEFORE kicking off recompute so
        // the invariant "dirty=true iff this engine has uncomputed pending changes"
        // holds during the in-flight window. _calculateSegment clears it on completion.
        return {
          ...s,
          profileConfig,
          segments: [],
          isCalculating: true,
          calculationError: null,
          dirtyEngines: { ...s.dirtyEngines, [s.routingEngine]: true },
        }
      })
      writeStoredProfile(profileConfig.profile)
      for (let i = 0; i < currentWaypoints.length - 1; i++) {
        await _calculateSegment(currentWaypoints[i], currentWaypoints[i + 1], profileConfig, currentEngine, currentBrouterProfile)
      }
      setState(s => ({ ...s, isCalculating: false }))
    },
    [_calculateSegment],
  )

  const setSportType = useCallback((sportType: string, subType?: string | null) => {
    setState(s => ({ ...s, sportType, subType: subType ?? null }))
  }, [])

  const setRouteModifiers = useCallback((modifiers: {
    isElectric?: boolean
    isIndoor?: boolean
    isVirtual?: boolean
    isCommute?: boolean
  }) => {
    setState(s => ({
      ...s,
      isElectric: modifiers.isElectric ?? s.isElectric,
      isIndoor: modifiers.isIndoor ?? s.isIndoor,
      isVirtual: modifiers.isVirtual ?? s.isVirtual,
      isCommute: modifiers.isCommute ?? s.isCommute,
    }))
  }, [])

  // ── Load route into planner ─────────────────────────────────────────────────

  const loadRoute = useCallback((route: PlannedRoute) => {
    // Spec Rev 2.3 §7.4 / F3: Saved routes load instantly from DB
    // (waypoints + segments + profile + enrichment). NEVER recalculateAll().
    resetScalePin()
    loadedRouteRef.current = route

    // route.profile is a string — try cache first, then fetch async
    const cached = getProfileDefaults(route.profile)

    // Critic #3 (BLOCKER): deserializeRouteStats sets coordinates: [].
    // MapPage memo-guard (routeStats.coordinates.length >= 2) gates the
    // SAC/MTB/VF range overlays + scale-color base — without rebuilding
    // coordinates from segment geometry here, those overlays never render
    // for loaded routes. Mirrors the merge logic in mergeSegmentStatsForState
    // (junction-dedup: skip first coord of each segment after the first).
    const baseStats = deserializeRouteStats(route.enrichment ?? null)
    let routeStats = baseStats
    if (baseStats) {
      const wpOrder = new Map(route.waypoints.map((wp, i) => [wp.id, i]))
      const sortedSegs = [...route.segments].sort(
        (a, b) =>
          (wpOrder.get(a.from_waypoint) ?? Infinity) -
          (wpOrder.get(b.from_waypoint) ?? Infinity),
      )
      const coords: number[][] = []
      for (let i = 0; i < sortedSegs.length; i++) {
        const segCoords = (sortedSegs[i].geometry?.coordinates ?? []) as number[][]
        if (i === 0) {
          for (const c of segCoords) coords.push(c)
        } else {
          for (let j = 1; j < segCoords.length; j++) coords.push(segCoords[j])
        }
      }
      routeStats = { ...baseStats, coordinates: coords }
    }

    // Bug B (2026-04-25): mark the deserialized profile as "segments"-grade
    // so the effectiveProfile-sync useEffect's truncation-guard knows the
    // current prev is full-resolution and rejects shorter dem-fallback
    // candidates that may briefly appear during user edits.
    if (route.elevation_profile != null && route.elevation_profile.length > 0) {
      _prevSourceRef.current = 'segments'
    }

    setState(s => ({
      ...s,
      waypoints: route.waypoints,
      segments: route.segments,
      // profileConfig: use cached if present; otherwise keep current and fetch below.
      profileConfig: cached ?? s.profileConfig,
      sportType: route.sport_type,
      subType: route.sub_type ?? null,
      isElectric: route.is_electric ?? false,
      isIndoor: route.is_indoor ?? false,
      isVirtual: route.is_virtual ?? false,
      isCommute: route.is_commute ?? false,
      isCalculating: false,
      calculationError: null,
      editingRouteId: route.id,
      // Spec §6.2 / §7.4: hydrate effectiveProfile + routeStats from DB.
      effectiveProfile: route.elevation_profile ?? null,
      routeSchemaVersion: route.route_schema_version ?? 0,
      routeStats,
      _statsCache: {},
      lazyFillInProgress: false,
      lazyFillPendingSave: false,
      // Bug B (2026-04-24): treat the loaded geometry as already rendered by
      // the persisted engine (or current selection for legacy routes without
      // routing_engine). Also seed routingEngine so the engine-toggle UI
      // reflects the engine that actually produced the saved geometry.
      // Finding #10 (2026-04-24): seed from route.routing_engine, not s.routingEngine.
      routingEngine: (route.routing_engine ?? s.routingEngine) as typeof s.routingEngine,
      lastRenderedEngine: (route.routing_engine ?? s.routingEngine) as typeof s.routingEngine,
      dirtyEngines: { badrouter: false, brouter: false },
    }))

    if (!cached) {
      // Profile defaults were not in cache — fetch them (does NOT trigger
      // recalculateAll; Spec §7.4 forbids it on load).
      fetchProfileDefaults(route.profile)
        .then(resp => {
          cacheServerDefaults(route.profile, resp)
          setState(s => {
            // Race-guard: another loadRoute may have been called while this
            // fetch was in-flight. Only apply if we're still editing THIS route.
            if (s.editingRouteId !== route.id) return s
            const fresh = getProfileDefaults(route.profile)
            if (!fresh) return s
            return { ...s, profileConfig: fresh }
          })
        })
        .catch(() => {
          addToast({
            type: 'error',
            title: 'Profil-Defaults konnten nicht geladen werden',
            message: `Profil "${route.profile}" konnte nicht geladen werden.`,
            duration: 0,
          })
        })
    }

    // ── Pre-T0 Migration (Phase E / Spec §5.3-5.5) ────────────────────────────
    // Only migrate when the route needs it — T0 steady-state routes are
    // served with route_schema_version=1 and all segments having profiles,
    // so needsMigration returns false and we skip this entire path.
    // Note: segments + lazyFillInProgress=false are already set by the setState
    // above, so the no-migration path is a plain return.
    if (!needsMigration(route)) return

    // Snapshot the segments-array reference BEFORE the await.
    // Any user mutation (moveWaypoint/add/delete → _calculateSegment replaces
    // the segments array) will break the === check in the post-await callback.
    const segmentsSnapshot = route.segments

    // Token-counter prevents a concurrent loadRoute(R2) from overwriting an
    // in-flight loadRoute(R1) migration result.
    let expectedToken = 0
    setState(s => {
      const newToken = (s.lazyFillToken ?? 0) + 1
      expectedToken = newToken
      return { ...s, lazyFillInProgress: true, lazyFillToken: newToken }
    })

    void (async () => {
      let migratedSegments: RouteSegment[]
      try {
        migratedSegments = await migrateRouteCached(route)
      } catch {
        // DEM endpoint unavailable or migration error: silent graceful degradation.
        // Toast is shown so the user knows the profile may be incomplete.
        addToast({
          type: 'error',
          title: 'Höhenprofil-Migration fehlgeschlagen',
          message: 'Höhenprofil bleibt unvollständig — bitte Seite neu laden.',
          duration: 0,
        })
        setState(s => {
          if (s.lazyFillToken !== expectedToken || s.segments !== segmentsSnapshot) return s
          return { ...s, lazyFillInProgress: false }
        })
        return
      }

      setState(s => {
        // Race-guard: user triggered a segment mutation or a newer loadRoute
        // incremented the token while migration was in-flight → discard.
        if (s.lazyFillToken !== expectedToken || s.segments !== segmentsSnapshot) {
          return { ...s, lazyFillInProgress: false }
        }
        // Migration is strictly read-only (Spec §5.4): do NOT set
        // lazyFillPendingSave=true. The next explicit user save will persist
        // the filled profiles and set route_schema_version=1 server-side.
        return {
          ...s,
          segments: migratedSegments,
          lazyFillInProgress: false,
        }
      })
    })()
  }, [addToast])

  // Load waypoints into the planner without creating/linking a saved route.
  const loadWaypoints = useCallback((waypoints: RouteWaypoint[]) => {
    loadedRouteRef.current = null
    setState(s => ({
      ...s,
      waypoints,
      segments: [],
      isCalculating: false,
      calculationError: null,
      editingRouteId: null,
      routeStats: null,
      _statsCache: {},
      routeSchemaVersion: null,
    }))
  }, [])

  // Load a route suggestion directly as a segment — no re-routing.
  // Takes the full geojson LineString geometry from the backend suggestion and
  // stores it as a single 'routed' segment with backend-provided metrics.
  // This prevents the previous bug where only 3 waypoints were extracted and
  // the segment router re-calculated a completely different route.
  const loadSuggestion = useCallback((s: RouteSuggestion) => {
    // I3-Invariant: MUSS als erste Anweisung bleiben — "Übernehmen"-Flow
    // im Comparison-Panel (onSelect-Handler des RouteComparisonPanel) ist von diesem Reset abhängig.
    resetScalePin()
    loadedRouteRef.current = null
    const firstFeature = s.geojson?.features?.[0]
    const coords = firstFeature?.geometry?.coordinates as number[][] | undefined
    if (!coords || coords.length < 2) {
      setState(st => ({
        ...st,
        calculationError: 'Suggestion-Geometrie fehlt oder ungültig',
      }))
      return
    }

    const startWp: RouteWaypoint = {
      id: crypto.randomUUID(),
      latlng: [coords[0][1], coords[0][0]],
      type: 'start',
    }
    const endWp: RouteWaypoint = {
      id: crypto.randomUUID(),
      latlng: [coords[coords.length - 1][1], coords[coords.length - 1][0]],
      type: 'end',
    }

    // Critic #15 (MAJOR): extract per-segment elevation_profile from the
    // feature's properties so that buildCombinedProfileFromSegments returns
    // a non-null profile and the useEffect-sync does NOT trigger an unnecessary
    // /elevation/profile fallback (the data is already in this response).
    // Mirrors buildSegmentFromFeature parsing logic.
    const featureProps = (firstFeature?.properties ?? {}) as Record<string, unknown>
    const rawSegProfile = featureProps['elevation_profile']
    let segElevationProfile: RouteElevationPoint[] | undefined
    if (Array.isArray(rawSegProfile) && rawSegProfile.length > 0) {
      const filtered = rawSegProfile
        .filter(
          // ele_m may be null (DEM coverage gap — Spec §6 Class A). Pass null through
          // so the chart can render an honest gap rather than interpolating fake heights.
          (pt): pt is { dist_m: number; ele_m: number | null } =>
            pt != null &&
            typeof pt === 'object' &&
            typeof (pt as { dist_m?: unknown }).dist_m === 'number' &&
            ((pt as { ele_m?: unknown }).ele_m === null ||
              typeof (pt as { ele_m?: unknown }).ele_m === 'number'),
        )
        .map(pt => ({ dist_m: pt.dist_m, ele_m: pt.ele_m }))
      if (filtered.length > 0) segElevationProfile = filtered
    }

    const segment: RouteSegment = {
      from_waypoint: startWp.id,
      to_waypoint: endWp.id,
      mode: 'routed',
      geometry: {
        type: 'LineString',
        coordinates: coords,
      },
      control_points: [],
      smooth: false,
      distance_m: s.distance_m,
      elevation_gain: s.ascent_m,
      elevation_loss: s.descent_m,
      elevation_profile: segElevationProfile,
    }

    // V6-Retrofit Quelle 2: Stats aus Suggestion direkt übernehmen + synthetisch cachen.
    const suggestionCoords = (s.geojson?.features?.[0]?.geometry?.coordinates
      ?? []) as number[][]
    const statsFromSuggestion: RouteStats = {
      forest_pct: s.forest_pct ?? null,
      protected_pct: s.protected_pct ?? null,
      forest_ranges: s.forest_ranges ?? null,
      protected_ranges: s.protected_ranges ?? null,
      sac_ranges: s.sac_ranges ?? null,
      mtb_ranges: s.mtb_ranges ?? null,
      vf_ranges: s.vf_ranges ?? null,
      vf_unknown_ranges: s.vf_unknown_ranges ?? null,
      summits_passed: s.summits_passed ?? null,
      coordinates: suggestionCoords,
    }
    const synthKey = `${startWp.id}:${endWp.id}`

    setState(st => ({
      ...st,
      waypoints: [startWp, endWp],
      segments: [segment],
      _statsCache: { [synthKey]: statsFromSuggestion },
      routeStats: statsFromSuggestion, // kein Shift nötig — einzelnes Segment, coordOffset=0
      editingRouteId: null,
      routeSchemaVersion: null,
      calculationError: null,
      isCalculating: false,
      // Critic #4 (MAJOR): hydrate effectiveProfile from suggestion top-level.
      // Top-level is authoritative (Spec §6.4), per-feature is for per-segment
      // wiring (already done via segElevationProfile above).
      effectiveProfile: s.elevation_profile ?? null,
    }))
    // Bug B (2026-04-25): suggestion delivers a full top-level profile —
    // mark as 'segments'-grade for the truncation-guard.
    if (s.elevation_profile != null && s.elevation_profile.length > 0) {
      _prevSourceRef.current = 'segments'
    }
  }, [])

  // ── Clear ───────────────────────────────────────────────────────────────────

  const clear = useCallback(() => {
    loadedRouteRef.current = null
    setState(s => ({
      ...INITIAL_STATE,
      profileConfig: s.profileConfig,
      sportType: s.sportType,
      subType: s.subType,
      isElectric: s.isElectric,
      isIndoor: s.isIndoor,
      isVirtual: s.isVirtual,
      isCommute: s.isCommute,
      routingEngine: s.routingEngine,
      brouterProfile: s.brouterProfile,
    }))
  }, [])

  const clearError = useCallback(() => {
    setState(s => ({ ...s, calculationError: null }))
  }, [])

  // ── Routing engine ──────────────────────────────────────────────────────────

  const setRoutingEngine = useCallback(async (engine: RoutingEngine) => {
    // Bug B (2026-04-24): switching engines on a LOADED saved route must not
    // wipe its persisted geometry — display-only swap unless the target
    // engine has pending profile/setting changes since the last render.
    // FRESH planning (editingRouteId === null) keeps the original behaviour:
    // recompute so the user can compare engines on a route they're building.
    //
    // Fix #5 (React 19 StrictMode): the recompute decision is made here from
    // the `state` closure variable BEFORE entering setState. This avoids the
    // StrictMode double-invocation hazard where the reducer runs twice and the
    // outer `needsRecompute`/`recomputeArgs` variables can be left with stale
    // values from the first invocation.
    if (state.routingEngine === engine) return

    const isLoadedRoute = state.editingRouteId !== null
    const hasSegments = state.segments.length > 0
    const targetEngineDirty = state.dirtyEngines[engine]
    const needsRecompute = !(isLoadedRoute && hasSegments && !targetEngineDirty)
    const recomputeArgs = needsRecompute && state.profileConfig
      ? { wps: state.waypoints, cfg: state.profileConfig, profile: state.brouterProfile }
      : null

    if (!needsRecompute) {
      // Display-only: keep saved geometry visible, just swap the engine flag.
      setState(s => ({ ...s, routingEngine: engine }))
      return
    }

    setState(s => ({
      ...s,
      routingEngine: engine,
      segments: [],
      isCalculating: true,
      calculationError: null,
      routeStats: null,
      _statsCache: {},
    }))

    if (!recomputeArgs) {
      setState(s => ({ ...s, isCalculating: false }))
      return
    }
    // Pass the new engine value explicitly — do NOT read from state here,
    // as React batches setState updates and s.routingEngine would still be stale.
    for (let i = 0; i < recomputeArgs.wps.length - 1; i++) {
      await _calculateSegment(recomputeArgs.wps[i], recomputeArgs.wps[i + 1], recomputeArgs.cfg, engine, recomputeArgs.profile)
    }
    setState(s => ({
      ...s,
      isCalculating: false,
      lastRenderedEngine: engine,
      dirtyEngines: { ...s.dirtyEngines, [engine]: false },
    }))
  }, [state, _calculateSegment])

  const setBrouterProfile = useCallback(async (profile: string) => {
    // Fix #5 (React 19 StrictMode): compute the recompute decision from the
    // `state` closure variable BEFORE entering setState to avoid the double-
    // invocation hazard where mutating outer-scope variables inside a reducer
    // leaves stale values after the second invocation.
    const isActiveBrouter = state.routingEngine === 'brouter'
    const canRecompute = isActiveBrouter && state.waypoints.length >= 2 && state.profileConfig !== null
    const recomputeArgs = canRecompute && state.profileConfig
      ? { wps: state.waypoints, cfg: state.profileConfig }
      : null

    if (canRecompute) {
      setState(s => ({
        ...s,
        brouterProfile: profile,
        segments: [],
        isCalculating: true,
        calculationError: null,
        routeStats: null,
        _statsCache: {},
        dirtyEngines: { ...s.dirtyEngines, brouter: true },
      }))
    } else {
      // Inactive engine OR not enough waypoints: just flag dirty.
      setState(s => ({
        ...s,
        brouterProfile: profile,
        dirtyEngines: { ...s.dirtyEngines, brouter: true },
      }))
    }

    if (!recomputeArgs) return
    for (let i = 0; i < recomputeArgs.wps.length - 1; i++) {
      await _calculateSegment(recomputeArgs.wps[i], recomputeArgs.wps[i + 1], recomputeArgs.cfg, 'brouter', profile)
    }
    setState(s => ({
      ...s,
      isCalculating: false,
      lastRenderedEngine: 'brouter',
      dirtyEngines: { ...s.dirtyEngines, brouter: false },
    }))
  }, [state, _calculateSegment])

  /**
   * Bug B (2026-04-24): Public API to flag an engine as having pending
   * profile/setting changes. UI panels that mutate engine-specific settings
   * (e.g. BadRouter slider panel, BRouter profile selector) call this so
   * a subsequent engine-switch back to that engine triggers a recompute
   * instead of a stale display-only swap.
   *
   * No-op when engine === currently active engine AND profile/settings are
   * already in lockstep with the rendered geometry — callers may invoke
   * defensively without checking.
   */
  // ── Save mutation ────────────────────────────────────────────────────────────

  // Captured synchronously in mutationFn so onSuccess can tell update vs. create.
  const wasUpdateRef = useRef(false)
  // Captured synchronously in mutationFn so onSuccess can detect a concurrent
  // loadRoute() that replaced editingRouteId while the PATCH was in-flight.
  const savingRouteIdRef = useRef<string | null>(null)

  const saveRouteMutation = useMutation({
    mutationFn: (overrides: Partial<PlannedRoute>) => {
      if (!state.profileConfig) {
        addToast({
          type: 'error',
          title: 'Route kann noch nicht gespeichert werden',
          message: 'Profil-Defaults sind noch nicht geladen.',
          duration: 0,
        })
        return Promise.reject(new Error('profileConfig not loaded'))
      }
      // Build combined LineString geometry from all segments. Backend requires
      // geometry_geojson (non-empty GeoJSON) — without it, ST_GeomFromGeoJSON('')
      // fails with SQLSTATE XX000 "unexpected end of data".
      // Sort by waypoint order as a safety net (state.segments is normally kept
      // sorted by the _calculateSegment sort, but old/corrupted saved routes may
      // load with a wrong order before recalculation completes).
      const wpOrder = new Map(state.waypoints.map((wp, i) => [wp.id, i]))
      const sortedSegments = [...state.segments].sort(
        (a, b) => (wpOrder.get(a.from_waypoint) ?? Infinity) - (wpOrder.get(b.from_waypoint) ?? Infinity),
      )
      const combinedCoords = sortedSegments.flatMap(seg => seg.geometry?.coordinates ?? [])
      if (combinedCoords.length < 2) {
        addToast({
          type: 'error',
          title: 'Route kann nicht gespeichert werden',
          message: 'Die Route hat keine Geometrie. Bitte zuerst Wegpunkte und eine Strecke erzeugen.',
          duration: 0,
        })
        return Promise.reject(new Error('route has no geometry'))
      }
      const geometryGeoJSON = JSON.stringify({
        type: 'LineString',
        coordinates: combinedCoords,
      })

      const profilePayload: RouteElevationPoint[] | null =
        state.effectiveProfile && state.effectiveProfile.length > 0
          ? state.effectiveProfile
          : null

      const enrichmentPayload: Record<string, unknown> | null = state.routeStats
        ? { enrichment_version: 1, ...serializeRouteStats(state.routeStats) }
        : null

      const payload: Partial<PlannedRoute> = {
        waypoints: state.waypoints,
        segments: state.segments,
        geometry_geojson: geometryGeoJSON,
        profile: state.profileConfig.profile,
        sport_type: state.sportType,
        sub_type: state.subType,
        is_electric: state.isElectric,
        is_indoor: state.isIndoor,
        is_virtual: state.isVirtual,
        is_commute: state.isCommute,
        total_distance_m: totals.distance_m,
        total_elevation_gain: totals.elevation_gain,
        total_elevation_loss: totals.elevation_loss,
        elevation_profile: profilePayload,
        enrichment: enrichmentPayload,
        routing_engine: state.routingEngine,
        ...overrides,
      }
      wasUpdateRef.current = state.editingRouteId !== null
      savingRouteIdRef.current = state.editingRouteId
      if (state.editingRouteId !== null) {
        return updateRoute(state.editingRouteId, payload)
      }
      return createRoute(payload)
    },
    onSuccess: saved => {
      // Cache invalidation always fires, regardless of any race.
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      queryClient.invalidateQueries({ queryKey: ['route', saved.id] })
      setState(s => {
        // Guard against concurrent loadRoute(): read editingRouteId from the
        // *current* state snapshot (not the stale closure) so we catch any
        // route swap that happened while the PATCH was in-flight.
        // If we were updating route X and state now shows a different route, skip
        // the editingRouteId / lazyFill updates — they belong to the new route.
        const idDrifted = wasUpdateRef.current && s.editingRouteId !== savingRouteIdRef.current
        if (idDrifted) {
          return s
        }
        return {
          ...s,
          editingRouteId: saved.id,
          routeSchemaVersion: saved.route_schema_version ?? 1,
          lazyFillPendingSave: false,  // Spec §7.2: reset after successful save
          // Bump token to invalidate any in-flight lazy-fill whose resolve would
          // otherwise re-set lazyFillPendingSave=true after we cleared it here.
          // The lazy-fill setState-callback checks s.lazyFillToken === expectedToken
          // and no-ops on mismatch, so it silently discards the stale update.
          lazyFillToken: (s.lazyFillToken ?? 0) + 1,
        }
      })
      if (wasUpdateRef.current) {
        addToast({
          type: 'success',
          title: 'Route aktualisiert',
          message: saved.name || undefined,
        })
      }
    },
  })

  const forceMigrateAndSave = useCallback(async (): Promise<PlannedRoute | null> => {
    const loadedRoute = loadedRouteRef.current
    if (!loadedRoute) return null

    const routeForMigration: PlannedRoute = {
      ...loadedRoute,
      waypoints: state.waypoints,
      segments: state.segments,
      elevation_profile: state.effectiveProfile,
    }
    const migratedSegments = await migrateRouteCached(routeForMigration)
    const migratedProfile = buildCombinedProfileFromSegments(migratedSegments, state.waypoints)

    setState(s => ({
      ...s,
      segments: migratedSegments,
      effectiveProfile: migratedProfile,
      routeSchemaVersion: 1,
      lazyFillInProgress: false,
    }))

    const saved = await saveRouteMutation.mutateAsync({
      segments: migratedSegments,
      elevation_profile: migratedProfile,
      route_schema_version: 1,
    })
    loadedRouteRef.current = saved
    return saved
  }, [saveRouteMutation, state.effectiveProfile, state.segments, state.waypoints])

  // ── Elevation profile (Spec §6.2 / Rev 2.3) ─────────────────────────────────
  //
  // Quelle 1 (Primary): pro-Segment elevation_profile — vom Backend in
  //   RouteSegment.elevation_profile geliefert (Phase 2/3.1) bzw. später
  //   von Lazy-Fill nachgeladen (Phase 5).
  // Quelle 2 (Fallback): POST /elevation/profile gegen die kombinierte
  //   Geometrie — nur wenn KEIN Segment ein Profil trägt.
  // Die effectiveProfile-State wird via useEffect aus diesen Quellen synchronisiert
  // und ist die single-source-of-truth für Konsumenten + Save-Pfad.

  // Build combined coordinate list from all segment geometries, sorted by
  // waypoint order (state.segments is normally sorted, but defensive sort here
  // covers the brief window between loadRoute and recalculateAll completing).
  //
  // Bug D fix (2026-04-25): vorher wurden combinedCoordinates, profileFromSegments
  // und _totalDistMForProfile bei JEDEM Render neu berechnet. Beim schnellen
  // Setzen mehrerer Wegpunkte erzeugte das ständig neue Array-Identitäten
  // → neuer queryKey für elevationProfileQuery → Refetch + useEffect-Trigger
  // → Render-Storm und UI-Ruckler. useMemo bündelt die Ableitungen mit den
  // einzigen wirklichen Inputs (segments + waypoints).
  const { combinedCoordinates, profileFromSegments, _totalDistMForProfile } = useMemo(() => {
    const wpOrder = new Map(state.waypoints.map((wp, i) => [wp.id, i]))
    const sorted = [...state.segments].sort(
      (a, b) =>
        (wpOrder.get(a.from_waypoint) ?? Infinity) -
        (wpOrder.get(b.from_waypoint) ?? Infinity),
    )
    const coords = sorted.flatMap(seg => seg.geometry?.coordinates ?? [])
    const totalDist = state.segments.reduce((acc, seg) => acc + seg.distance_m, 0)
    const profile = buildCombinedProfileFromSegments(state.segments, state.waypoints)
    return {
      combinedCoordinates: coords,
      profileFromSegments: profile,
      _totalDistMForProfile: totalDist,
    }
  }, [state.segments, state.waypoints])

  // Fallback: POST /elevation/profile only when no segment carries a profile.
  const needsBRouterFallback: boolean =
    profileFromSegments === null && combinedCoordinates.length >= 2

  const combinedGeometry: GeoJSON.LineString | null = useMemo(
    () =>
      needsBRouterFallback
        ? { type: 'LineString', coordinates: combinedCoordinates }
        : null,
    [needsBRouterFallback, combinedCoordinates],
  )

  const elevationProfileQuery = useQuery<{ profile: ElevationPoint[] }>({
    queryKey: [
      'route-elevation-profile',
      state.editingRouteId,
      combinedCoordinates.length,
      // Include first+last coordinate to detect waypoint moves with same point count
      combinedCoordinates[0]?.toString(),
      combinedCoordinates[combinedCoordinates.length - 1]?.toString(),
    ],
    queryFn: () => getElevationProfile(combinedGeometry!, { numPoints: 200 }),
    enabled: needsBRouterFallback,
    staleTime: 60 * 1000,
  })

  // Sync effectiveProfile from the two sources. Shallow-equality guard prevents
  // re-render loops: buildCombinedProfileFromSegments returns a new array each
  // render, so reference comparison alone would always trigger setState.
  //
  // Bug B fix (2026-04-25): Source-Tracking gegen Höhenprofil-Trunkierung.
  // Vorher schützte der Guard nur bei `editingRouteId !== null` (geladene Route).
  // Bei frischer Route (editingRouteId=null) konnte ein Add-Waypoint mit kurz
  // unvollständigem profileFromSegments (neues Segment ohne elevation_profile-
  // Feld → distAccum advanced, aber keine Punkte) das DEM-Fallback (200-Sample-
  // Resampling) als Sieger küren und das vorhandene Vollprofil überschreiben.
  // Neue Regel: wenn `prev` aus 'segments' kam und `next` aus 'dem-fallback'
  // kommt UND `prev.length > next.length` → next verwerfen. Engine-agnostisch.
  const nextSource: 'segments' | 'dem-fallback' | 'none' =
    profileFromSegments !== null
      ? 'segments'
      : elevationProfileQuery.data
        ? 'dem-fallback'
        : 'none'
  useEffect(() => {
    const next: RouteElevationPoint[] | null =
      profileFromSegments
      ?? (elevationProfileQuery.data
            ? legacyToRouteProfile(elevationProfileQuery.data.profile, _totalDistMForProfile)
            : null)
    setState(s => {
      const prev = s.effectiveProfile
      if (prev === next) return s
      if (prev === null && next === null) return s
      // Perf (Wave 3, Fix 3): Equality-Check ist bewusst O(1) — Length-Match
      // plus first/last dist_m+ele_m. Bei 10k-Punkt-Profilen reicht das, weil
      // jede Profil-Modifikation entweder die Länge oder den Endpunkt verschiebt
      // (Waypoint-Move ändert distAccum am Segment-Übergang). Vollscan würde
      // 40k Vergleiche pro setState kosten und keinen praktischen Nutzen bringen.
      if (
        prev && next && prev.length === next.length &&
        prev[0]?.dist_m === next[0]?.dist_m &&
        prev[0]?.ele_m === next[0]?.ele_m &&
        prev[prev.length - 1]?.dist_m === next[next.length - 1]?.dist_m &&
        prev[prev.length - 1]?.ele_m === next[next.length - 1]?.ele_m
      ) {
        return s
      }
      // Critic #6 (MAJOR, belt-and-suspenders): when editing a loaded route
      // and a non-null full-resolution profile is already present (from
      // route.elevation_profile or a completed lazy-fill), do NOT overwrite
      // it with a shorter candidate (e.g. a 200-sample DEM resampling).
      // The lazy-fill code-path is unaffected: it sets effectiveProfile via
      // its own setState while prev is still null, so this guard is bypassed.
      if (
        s.editingRouteId !== null &&
        prev !== null &&
        next !== null &&
        prev.length > next.length
      ) {
        return s
      }
      // Bug B fix (2026-04-25): engine-agnostischer Trunkierungsschutz.
      // Wenn das aktuelle Profil aus dem (vollständigen) Segment-Aggregat kam
      // und der neue Kandidat aus dem DEM-Fallback (200-Sample-Resampling) ist
      // UND kürzer, dann ist das eine Regression — typisches Symptom: ein neu
      // hinzugefügtes Segment hat kurz noch kein elevation_profile, sodass
      // profileFromSegments null wird und das DEM-Fallback gewinnt. Der vorige
      // editingRouteId-Guard fing den Fall nur für geladene Routen ab.
      // _prevSourceRef tracked die Quelle des aktuellen prev-Profils.
      if (
        prev !== null &&
        next !== null &&
        _prevSourceRef.current === 'segments' &&
        nextSource === 'dem-fallback' &&
        prev.length > next.length
      ) {
        return s
      }
      _prevSourceRef.current = nextSource
      return { ...s, effectiveProfile: next }
    })
    // profileFromSegments is recomputed each render (new identity), but the
    // shallow-equality guard above no-ops when content is unchanged.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    state.segments,
    state.waypoints,
    elevationProfileQuery.data,
    _totalDistMForProfile,
  ])

  // ── Totals ──────────────────────────────────────────────────────────────────

  const totals = {
    // Use approach-inclusive total_distance_m when available (BadRouter); fall back to
    // network-only distance_m for BRouter and offroad segments (Spec §5.5 / Finding #17).
    distance_m: state.segments.reduce((acc, seg) => acc + (seg.total_distance_m ?? seg.distance_m), 0),
    elevation_gain: state.segments.reduce(
      (acc, seg) => acc + seg.elevation_gain,
      0,
    ),
    elevation_loss: state.segments.reduce(
      (acc, seg) => acc + seg.elevation_loss,
      0,
    ),
  }

  // ── Derived flags ────────────────────────────────────────────────────────────

  const canSave =
    state.waypoints.length >= 2 &&
    !state.isCalculating &&
    !saveRouteMutation.isPending

  const isDirty =
    state.waypoints.length > 0

  // ── Profile-gated namespace ──────────────────────────────────────────────────

  const profileState: RoutePlannerProfileState = state.profileConfig === null
    ? { status: 'loading' as const }
    : {
        status: 'ready' as const,
        config: state.profileConfig,
        name: state.profileConfig.profile,
        setConfig: setProfile,
        setAndRecalculate: setProfileAndRecalculate,
        recalculate: recalculateAll,
        saveRoute: (overrides?: Partial<PlannedRoute>) => saveRouteMutation.mutate(overrides ?? {}),
        saveRouteAsync: saveRouteMutation.mutateAsync,
        isSaving: saveRouteMutation.isPending,
        saveError: saveRouteMutation.error,
      }

  return {
    // ── Always-available state ─────────────────────────────────────────────
    waypoints: state.waypoints,
    segments: state.segments,
    sportType: state.sportType,
    subType: state.subType,
    isElectric: state.isElectric,
    isIndoor: state.isIndoor,
    isVirtual: state.isVirtual,
    isCommute: state.isCommute,
    isCalculating: state.isCalculating,
    calculationError: state.calculationError,
    editingRouteId: state.editingRouteId,
    routeSchemaVersion: state.routeSchemaVersion,
    routingEngine: state.routingEngine,
    brouterProfile: state.brouterProfile,
    totals,
    canSave,
    isDirty,

    // ── Waypoint-Management (null-tolerant) ───────────────────────────────
    addWaypoint,
    removeWaypoint,
    moveWaypoint,
    reorderWaypoints,
    insertViaWaypoint,

    // ── Segment actions (null-tolerant) ───────────────────────────────────
    calculateSegment,          // no-op when profileConfig null
    toggleOffroad,
    updateControlPoint,
    insertControlPoint,
    removeControlPoint,

    // ── Sport / Engine ────────────────────────────────────────────────────
    setSportType,
    setRouteModifiers,
    setRoutingEngine,
    setBrouterProfile,

    // ── Route lifecycle ───────────────────────────────────────────────────
    loadRoute,
    loadWaypoints,
    loadSuggestion,
    clear,
    clearError,

    // ── React Query: capabilities ─────────────────────────────────────────
    capabilities: capabilitiesQuery.data ?? null,
    capabilitiesLoading: capabilitiesQuery.isLoading,

    // ── Elevation profile (Spec §6.2 / Rev 2.3) ──────────────────────────
    // Single source of truth: effectiveProfile state (see useEffect above).
    // Type: RouteElevationPoint[] — { dist_m, ele_m } in meters.
    elevationProfile: state.effectiveProfile,
    elevationProfileLoading: needsBRouterFallback && elevationProfileQuery.isLoading,

    // ── Profile-gated namespace ───────────────────────────────────────────
    profile: profileState,

    // ── Factory queries (call in component) ───────────────────────────────
    useRouteList,
    useRouteDetail,

    // ── Scale-Pin + Hover (SAC/MTB/VF) ────────────────────────────────────
    scalePin,
    scaleHover,
    scaleEffective,
    togglePin,
    setScaleHover,
    resetScalePin,

    // ── V6-Retrofit: aggregierte Route-Stats ──────────────────────────────
    // null wenn BRouter-Engine, kein Segment, oder noch keine Stats berechnet.
    routeStats: state.routeStats,

    // ── Lazy-Fill DEM (Spec §7.4) ─────────────────────────────────────────
    lazyFillPendingSave: state.lazyFillPendingSave,
    lazyFillInProgress: state.lazyFillInProgress,
    forceMigrateAndSave,
  }
}

import { useState, useCallback, useEffect } from 'react'
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
  type ElevationPoint,
  getProfileDefaults as fetchProfileDefaults,
} from '@/api/routes'
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
}

const INITIAL_STATE: RoutePlannerState = {
  waypoints: [],
  segments: [],
  profileConfig: null,
  sportType: 'hiking',
  isCalculating: false,
  calculationError: null,
  editingRouteId: null,
  routingEngine: 'badrouter',
  brouterProfile: 'hiking-mountain',
  routeStats: null,
  _statsCache: {},
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
  const gain = Number(props['ascent_m'] ?? props['filtered ascend']) || 0
  const loss = Number(props['descent_m'] ?? props['filtered descend']) || 0

  const geometry = feature?.geometry as GeoJSON.LineString | undefined
  const approachFrom = props['approach_from'] as number[][] | undefined
  const approachTo = props['approach_to'] as number[][] | undefined
  const subSegments = (props['segments'] as RouteSubSegment[] | undefined) ?? []

  return {
    from_waypoint: from.id,
    to_waypoint: to.id,
    mode: isFallback ? 'offroad' : 'routed',
    geometry,
    control_points: isFallback ? [from.latlng, to.latlng] : [],
    smooth: true,
    distance_m: distance,
    elevation_gain: gain,
    elevation_loss: loss,
    approach_from: approachFrom,
    approach_to: approachTo,
    sub_segments: subSegments,
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

// ─── Hook ─────────────────────────────────────────────────────────────────────

export function useRoutePlanner() {
  const [state, setState] = useState<RoutePlannerState>(INITIAL_STATE)
  const queryClient = useQueryClient()
  const { addToast } = useToast()

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
        const newSegments =
          idx >= 0
            ? s.segments.map((seg, i) => (i === idx ? segment : seg))
            : [...s.segments, segment]

        // _statsCache atomar mit-updaten
        const newCache = { ...s._statsCache }
        if (statsForSegment) {
          newCache[segmentKey] = statsForSegment
        } else {
          delete newCache[segmentKey]
        }

        return {
          ...s,
          segments: newSegments,
          _statsCache: newCache,
          routeStats: mergeSegmentStatsForState(newSegments, newCache),
          isCalculating: false,
        }
      })
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
      return {
        ...s,
        isCalculating: true,
        calculationError: null,
        routeStats: null,
        _statsCache: {},
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
    setState(s => {
      const segments = [...s.segments]
      const idx = segments.findIndex(
        seg => seg.from_waypoint === key.from && seg.to_waypoint === key.to,
      )
      if (idx < 0) return s
      const seg = { ...segments[idx] }
      const cps = [...(seg.control_points ?? [])]
      if (cpIndex < 0 || cpIndex >= cps.length) return s
      cps[cpIndex] = latlng
      seg.control_points = cps
      segments[idx] = rebuildOffroadGeometry(seg)
      return { ...s, segments }
    })
  }, [])

  const insertControlPoint = useCallback((key: SegmentKey, afterCpIndex: number, latlng: [number, number]) => {
    setState(s => {
      const segments = [...s.segments]
      const idx = segments.findIndex(
        seg => seg.from_waypoint === key.from && seg.to_waypoint === key.to,
      )
      if (idx < 0) return s
      const seg = { ...segments[idx] }
      const cps = [...(seg.control_points ?? [])]
      cps.splice(afterCpIndex + 1, 0, latlng)
      seg.control_points = cps
      segments[idx] = rebuildOffroadGeometry(seg)
      return { ...s, segments }
    })
  }, [])

  const removeControlPoint = useCallback((key: SegmentKey, cpIndex: number) => {
    setState(s => {
      const segments = [...s.segments]
      const idx = segments.findIndex(
        seg => seg.from_waypoint === key.from && seg.to_waypoint === key.to,
      )
      if (idx < 0) return s
      const seg = { ...segments[idx] }
      const cps = [...(seg.control_points ?? [])]
      if (cpIndex < 0 || cpIndex >= cps.length) return s
      cps.splice(cpIndex, 1)
      seg.control_points = cps
      segments[idx] = rebuildOffroadGeometry(seg)
      return { ...s, segments }
    })
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
        return { ...s, profileConfig, segments: [], isCalculating: true, calculationError: null }
      })
      writeStoredProfile(profileConfig.profile)
      for (let i = 0; i < currentWaypoints.length - 1; i++) {
        await _calculateSegment(currentWaypoints[i], currentWaypoints[i + 1], profileConfig, currentEngine, currentBrouterProfile)
      }
      setState(s => ({ ...s, isCalculating: false }))
    },
    [_calculateSegment],
  )

  const setSportType = useCallback((sportType: string) => {
    setState(s => ({ ...s, sportType }))
  }, [])

  // ── Load route into planner ─────────────────────────────────────────────────

  const loadRoute = useCallback((route: PlannedRoute) => {
    // route.profile is a string — try cache first, then fetch async
    const cached = getProfileDefaults(route.profile)
    if (cached) {
      resetScalePin()
      setState(s => ({
        ...s,
        waypoints: route.waypoints,
        // L-11: DB-Segmente bleiben als Sub-Frame-Zwischenzustand stehen.
        // recalculateAll() setzt sie auf [] und befüllt routeStats neu via Quelle 1.
        // Intentional — falls recalculateAll scheitert stehen die DB-Segmente als Fallback.
        segments: route.segments,
        profileConfig: cached,
        sportType: route.sport_type,
        isCalculating: false,
        calculationError: null,
        editingRouteId: route.id,
        routeStats: null,      // V6-Retrofit: reset vor Re-Routing
        _statsCache: {},
      }))
      // V6-Retrofit Quelle 3: Re-Routing triggert _calculateSegment-Schleife,
      // die via Quelle 1 routeStats befüllt. ~500 ms Latenz, akzeptiert (Spec §3 Non-Goal).
      void recalculateAll()
    } else {
      resetScalePin()
      // Load waypoints/segments immediately, fetch profile in background
      setState(s => ({
        ...s,
        waypoints: route.waypoints,
        segments: route.segments,
        sportType: route.sport_type,
        isCalculating: false,
        calculationError: null,
        editingRouteId: route.id,
        routeStats: null,
        _statsCache: {},
      }))
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
          // NACH profileConfig-Set: Re-Routing triggern
          void recalculateAll()
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
  }, [recalculateAll])

  // Load waypoints into the planner without creating/linking a saved route.
  const loadWaypoints = useCallback((waypoints: RouteWaypoint[]) => {
    setState(s => ({
      ...s,
      waypoints,
      segments: [],
      isCalculating: false,
      calculationError: null,
      editingRouteId: null,
      routeStats: null,
      _statsCache: {},
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
    const coords = s.geojson?.features?.[0]?.geometry?.coordinates as number[][] | undefined
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
      calculationError: null,
      isCalculating: false,
    }))
  }, [])

  // ── Clear ───────────────────────────────────────────────────────────────────

  const clear = useCallback(() => {
    setState(s => ({
      ...INITIAL_STATE,
      profileConfig: s.profileConfig,
      sportType: s.sportType,
      routingEngine: s.routingEngine,
      brouterProfile: s.brouterProfile,
    }))
  }, [])

  const clearError = useCallback(() => {
    setState(s => ({ ...s, calculationError: null }))
  }, [])

  // ── Routing engine ──────────────────────────────────────────────────────────

  const setRoutingEngine = useCallback(async (engine: RoutingEngine) => {
    let currentWaypoints: RouteWaypoint[] = []
    let currentBrouterProfile = 'hiking-mountain'
    let currentProfileConfig: ProfileConfig | null = null
    setState(s => {
      if (s.routingEngine === engine) return s
      currentWaypoints = s.waypoints
      currentBrouterProfile = s.brouterProfile
      currentProfileConfig = s.profileConfig
      return {
        ...s,
        routingEngine: engine,
        segments: [],
        isCalculating: true,
        calculationError: null,
        routeStats: null,
        _statsCache: {},
      }
    })
    if (!currentProfileConfig) {
      setState(s => ({ ...s, isCalculating: false }))
      return
    }
    // Pass the new engine value explicitly — do NOT read from state here,
    // as React batches setState updates and s.routingEngine would still be stale.
    for (let i = 0; i < currentWaypoints.length - 1; i++) {
      await _calculateSegment(currentWaypoints[i], currentWaypoints[i + 1], currentProfileConfig, engine, currentBrouterProfile)
    }
    setState(s => ({ ...s, isCalculating: false }))
  }, [_calculateSegment])

  const setBrouterProfile = useCallback((profile: string) => {
    setState(s => ({ ...s, brouterProfile: profile }))
  }, [])

  // ── Save mutation ────────────────────────────────────────────────────────────

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
      const combinedCoords = state.segments.flatMap(seg => seg.geometry?.coordinates ?? [])
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

      const payload: Partial<PlannedRoute> = {
        waypoints: state.waypoints,
        segments: state.segments,
        geometry_geojson: geometryGeoJSON,
        profile: state.profileConfig.profile,
        sport_type: state.sportType,
        total_distance_m: totals.distance_m,
        total_elevation_gain: totals.elevation_gain,
        total_elevation_loss: totals.elevation_loss,
        ...overrides,
      }
      if (state.editingRouteId !== null) {
        return updateRoute(state.editingRouteId, payload)
      }
      return createRoute(payload)
    },
    onSuccess: saved => {
      setState(s => ({ ...s, editingRouteId: saved.id }))
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      queryClient.invalidateQueries({ queryKey: ['route', saved.id] })
    },
  })

  // ── Elevation profile ───────────────────────────────────────────────────────

  // Build combined coordinate list from all segment geometries
  const combinedCoordinates = state.segments
    .flatMap(seg => seg.geometry?.coordinates ?? [])

  // Check whether the geometry already carries elevation (3rd coordinate).
  // BadRouter returns [lon, lat, ele]; BRouter returns [lon, lat] only.
  const hasGeometryElevation =
    combinedCoordinates.length >= 2 &&
    combinedCoordinates[0].length >= 3

  // If BadRouter-style elevation is present, derive the profile directly from
  // coordinates — no extra API call needed.
  const profileFromGeometry: ElevationPoint[] | null = hasGeometryElevation
    ? (() => {
        const coords = combinedCoordinates
        // Haversine distance in meters between two [lon, lat] points
        const haversineM = (a: number[], b: number[]): number => {
          const R = 6371000
          const dLat = (b[1] - a[1]) * Math.PI / 180
          const dLon = (b[0] - a[0]) * Math.PI / 180
          const sinDLat = Math.sin(dLat / 2)
          const sinDLon = Math.sin(dLon / 2)
          const h = sinDLat * sinDLat +
            Math.cos(a[1] * Math.PI / 180) * Math.cos(b[1] * Math.PI / 180) * sinDLon * sinDLon
          return 2 * R * Math.asin(Math.sqrt(h))
        }

        const segLengths: number[] = []
        for (let i = 1; i < coords.length; i++) {
          segLengths.push(haversineM(coords[i - 1], coords[i]))
        }
        const totalLen = segLengths.reduce((a, b) => a + b, 0)
        if (totalLen === 0) return null

        const points: ElevationPoint[] = [{ fraction: 0, elevation: coords[0][2] ?? null }]
        let cumLen = 0
        for (let i = 0; i < segLengths.length; i++) {
          cumLen += segLengths[i]
          points.push({
            fraction: cumLen / totalLen,
            elevation: coords[i + 1][2] ?? null,
          })
        }
        return points
      })()
    : null

  // Fallback: POST /elevation/profile (for BRouter routes without 3D geometry)
  const combinedGeometry: GeoJSON.LineString | null =
    !hasGeometryElevation && combinedCoordinates.length >= 2
      ? { type: 'LineString', coordinates: combinedCoordinates }
      : null

  const elevationProfileQuery = useQuery<{ profile: ElevationPoint[] }>({
    queryKey: [
      'route-elevation-profile',
      state.editingRouteId,
      combinedCoordinates.length,
      // Include first+last coordinate to detect waypoint moves with same point count
      combinedCoordinates[0]?.toString(),
      combinedCoordinates[combinedCoordinates.length - 1]?.toString(),
    ],
    queryFn: () => getElevationProfile(combinedGeometry!, 200),
    enabled: combinedGeometry !== null,
    staleTime: 60 * 1000,
  })

  // ── Totals ──────────────────────────────────────────────────────────────────

  const totals = {
    distance_m: state.segments.reduce((acc, seg) => acc + seg.distance_m, 0),
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
    isCalculating: state.isCalculating,
    calculationError: state.calculationError,
    editingRouteId: state.editingRouteId,
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

    // ── React Query: elevation profile ────────────────────────────────────
    // Prefer geometry-embedded elevation (BadRouter), fall back to API query (BRouter)
    elevationProfile: profileFromGeometry ?? elevationProfileQuery.data?.profile ?? null,
    elevationProfileLoading: !profileFromGeometry && elevationProfileQuery.isLoading,

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
  }
}

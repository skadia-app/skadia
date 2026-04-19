import { useState, useCallback, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type {
  RouteWaypoint,
  RouteSegment,
  RouteSubSegment,
  PlannedRoute,
  ListRoutesParams,
  RouteSuggestion,
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
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

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

// ─── Hook ─────────────────────────────────────────────────────────────────────

export function useRoutePlanner() {
  const [state, setState] = useState<RoutePlannerState>(INITIAL_STATE)
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  // ── Mount-fetch: hydrate hiking defaults from server ─────────────────────────

  useEffect(() => {
    if (state.profileConfig !== null) return
    fetchProfileDefaults('hiking')
      .then(resp => {
        cacheServerDefaults('hiking', resp)
        let didPopulate = false
        setState(s => {
          // Race-guard: another flow (e.g. loadRoute for deeplink) may have
          // populated profileConfig between this useEffect firing and the
          // fetch resolving. Don't overwrite.
          if (s.profileConfig !== null) return s
          const fresh = getProfileDefaults('hiking')
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
        return { ...s, segments: newSegments, isCalculating: false }
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
      return { ...s, waypoints: newWaypoints, segments: newSegments }
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
      return { ...s, waypoints: retyped, segments: [] } // segments are invalid after reorder
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
      return { ...s, waypoints: newWaypoints, segments: newSegments }
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
    let currentWaypoints: RouteWaypoint[] = []
    let currentProfileConfig: ProfileConfig | null = null
    let currentEngine: RoutingEngine = 'badrouter'
    let currentBrouterProfile = 'hiking-mountain'
    setState(s => {
      currentWaypoints = s.waypoints
      currentProfileConfig = s.profileConfig
      currentEngine = s.routingEngine
      currentBrouterProfile = s.brouterProfile
      return { ...s, segments: [], isCalculating: true, calculationError: null }
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
      setState(s => ({
        ...s,
        waypoints: route.waypoints,
        segments: route.segments,
        profileConfig: cached,
        sportType: route.sport_type,
        isCalculating: false,
        calculationError: null,
        editingRouteId: route.id,
      }))
    } else {
      // Load waypoints/segments immediately, fetch profile in background
      setState(s => ({
        ...s,
        waypoints: route.waypoints,
        segments: route.segments,
        sportType: route.sport_type,
        isCalculating: false,
        calculationError: null,
        editingRouteId: route.id,
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
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Load waypoints into the planner without creating/linking a saved route.
  const loadWaypoints = useCallback((waypoints: RouteWaypoint[]) => {
    setState(s => ({
      ...s,
      waypoints,
      segments: [],
      isCalculating: false,
      calculationError: null,
      editingRouteId: null,
    }))
  }, [])

  // Load a route suggestion directly as a segment — no re-routing.
  // Takes the full geojson LineString geometry from the backend suggestion and
  // stores it as a single 'routed' segment with backend-provided metrics.
  // This prevents the previous bug where only 3 waypoints were extracted and
  // the segment router re-calculated a completely different route.
  const loadSuggestion = useCallback((s: RouteSuggestion) => {
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

    setState(st => ({
      ...st,
      waypoints: [startWp, endWp],
      segments: [segment],
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
      return { ...s, routingEngine: engine, segments: [], isCalculating: true, calculationError: null }
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
  }
}

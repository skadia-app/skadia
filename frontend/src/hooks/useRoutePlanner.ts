import { useState, useCallback, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type {
  RouteWaypoint,
  RouteSegment,
  PlannedRoute,
  ListRoutesParams,
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
} from '@/api/routes'
import type { ProfileConfig } from '@/lib/routingProfiles'
import { getProfileDefaults } from '@/lib/routingProfiles'
import { catmullRom, polylineDistanceM } from '@/lib/catmull-rom'

// ─── Exported types ──────────────────────────────────────────────────────────

/** Identifies a segment by its from/to waypoint IDs. */
export type SegmentKey = { from: string; to: string }

// ─── Internal state ───────────────────────────────────────────────────────────

type RoutingEngine = 'badrouter' | 'brouter'

interface RoutePlannerState {
  waypoints: RouteWaypoint[]
  segments: RouteSegment[]
  profileConfig: ProfileConfig
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
  profileConfig: getProfileDefaults('hiking'),
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

      let prevWp: RouteWaypoint | undefined
      let profileConfig: ProfileConfig | undefined
      let routingEngine: RoutingEngine | undefined
      let brouterProfile: string | undefined

      setState(s => {
        const updated = assignWaypointTypes([...s.waypoints, newWp])
        // Capture values for side effect outside setState
        if (updated.length >= 2) {
          prevWp = updated[updated.length - 2]
          profileConfig = s.profileConfig
          routingEngine = s.routingEngine
          brouterProfile = s.brouterProfile
        }
        return { ...s, waypoints: updated }
      })

      // Calculate segment between previous last waypoint and the new one
      // Side effect runs outside setState (Bug 4 fix)
      if (prevWp && profileConfig && routingEngine && brouterProfile) {
        void _calculateSegment(prevWp, newWp, profileConfig, routingEngine, brouterProfile)
      }
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
      let segmentsToCalc: Array<[RouteWaypoint, RouteWaypoint]> = []
      let profileConfig: ProfileConfig | undefined
      let routingEngine: RoutingEngine | undefined
      let brouterProfile: string | undefined

      setState(s => {
        const newWaypoints = s.waypoints.map(w =>
          w.id === wpId ? { ...w, latlng: newLatlng } : w,
        )
        // Capture adjacent segment pairs for side effect outside setState (Bug 4 fix)
        const idx = newWaypoints.findIndex(w => w.id === wpId)
        if (idx !== -1) {
          segmentsToCalc = []
          if (idx > 0) segmentsToCalc.push([newWaypoints[idx - 1], newWaypoints[idx]])
          if (idx < newWaypoints.length - 1) segmentsToCalc.push([newWaypoints[idx], newWaypoints[idx + 1]])
          profileConfig = s.profileConfig
          routingEngine = s.routingEngine
          brouterProfile = s.brouterProfile
        }
        return { ...s, waypoints: newWaypoints }
      })

      // Side effect runs outside setState (Bug 4 fix)
      if (profileConfig && routingEngine && brouterProfile) {
        for (const [from, to] of segmentsToCalc) {
          await _calculateSegment(from, to, profileConfig, routingEngine, brouterProfile)
        }
      }
    },
    [_calculateSegment],
  )

  const reorderWaypoints = useCallback((newOrder: RouteWaypoint[]) => {
    setState(s => {
      const retyped = assignWaypointTypes(newOrder)
      return { ...s, waypoints: retyped, segments: [] } // segments are invalid after reorder
    })
  }, [])

  const calculateSegment = useCallback(
    (from: RouteWaypoint, to: RouteWaypoint) => {
      return _calculateSegment(from, to, state.profileConfig, state.routingEngine, state.brouterProfile)
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [state.profileConfig, state.routingEngine, state.brouterProfile],
  )

  const recalculateAll = useCallback(async () => {
    let currentWaypoints: RouteWaypoint[] = []
    let currentProfileConfig: ProfileConfig = getProfileDefaults('hiking')
    let currentEngine: RoutingEngine = 'badrouter'
    let currentBrouterProfile = 'hiking-mountain'
    setState(s => {
      currentWaypoints = s.waypoints
      currentProfileConfig = s.profileConfig
      currentEngine = s.routingEngine
      currentBrouterProfile = s.brouterProfile
      return { ...s, segments: [], isCalculating: true, calculationError: null }
    })
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
        if (fromWp && toWp) {
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
    // route.profile is a string (BRouter legacy) — derive ProfileConfig from it
    const profileConfig = getProfileDefaults(route.profile)
    setState(s => ({
      ...s,
      waypoints: route.waypoints,
      segments: route.segments,
      profileConfig,
      sportType: route.sport_type,
      isCalculating: false,
      calculationError: null,
      editingRouteId: route.id,
    }))
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

  const setRoutingEngine = useCallback((engine: RoutingEngine) => {
    setState(s => {
      if (s.routingEngine === engine) return s
      return { ...s, routingEngine: engine }
    })
    // recalculateAll reads state via setState updater, so it sees the updated
    // routingEngine immediately — no setTimeout needed
    void recalculateAll()
  }, [recalculateAll])

  const setBrouterProfile = useCallback((profile: string) => {
    setState(s => ({ ...s, brouterProfile: profile }))
  }, [])

  // ── Save mutation ────────────────────────────────────────────────────────────

  const saveRouteMutation = useMutation({
    mutationFn: (overrides: Partial<PlannedRoute>) => {
      const payload: Partial<PlannedRoute> = {
        waypoints: state.waypoints,
        segments: state.segments,
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

  return {
    // State
    waypoints: state.waypoints,
    segments: state.segments,
    profileConfig: state.profileConfig,
    profile: state.profileConfig.profile,   // convenience string alias
    sportType: state.sportType,
    isCalculating: state.isCalculating,
    calculationError: state.calculationError,
    editingRouteId: state.editingRouteId,
    routingEngine: state.routingEngine,
    brouterProfile: state.brouterProfile,
    totals,
    canSave,
    isDirty,

    // Waypoint actions
    addWaypoint,
    removeWaypoint,
    moveWaypoint,
    reorderWaypoints,

    // Segment actions
    calculateSegment,
    recalculateAll,
    toggleOffroad,
    updateControlPoint,
    insertControlPoint,
    removeControlPoint,

    // Profile/sport
    setProfile,
    setProfileAndRecalculate,
    setSportType,

    // Routing engine
    setRoutingEngine,
    setBrouterProfile,

    // Route lifecycle
    loadRoute,
    loadWaypoints,
    clear,
    clearError,

    // React Query: save
    saveRoute: saveRouteMutation.mutate,
    saveRouteAsync: saveRouteMutation.mutateAsync,
    isSaving: saveRouteMutation.isPending,
    saveError: saveRouteMutation.error,

    // React Query: capabilities
    capabilities: capabilitiesQuery.data ?? null,
    capabilitiesLoading: capabilitiesQuery.isLoading,

    // React Query: elevation profile
    // Prefer geometry-embedded elevation (BadRouter), fall back to API query (BRouter)
    elevationProfile: profileFromGeometry ?? elevationProfileQuery.data?.profile ?? null,
    elevationProfileLoading: !profileFromGeometry && elevationProfileQuery.isLoading,

    // Factory queries (call in component)
    useRouteList,
    useRouteDetail,
  }
}

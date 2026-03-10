import { useState, useCallback } from 'react'
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
  computeRoute,
  getElevationProfile,
  type ElevationPoint,
} from '@/api/routes'

// ─── Internal state ───────────────────────────────────────────────────────────

interface RoutePlannerState {
  waypoints: RouteWaypoint[]
  segments: RouteSegment[]
  profile: string
  sportType: string
  isCalculating: boolean
  calculationError: string | null
  editingRouteId: string | null
}

const INITIAL_STATE: RoutePlannerState = {
  waypoints: [],
  segments: [],
  profile: 'hiking-sac2',
  sportType: 'hiking',
  isCalculating: false,
  calculationError: null,
  editingRouteId: null,
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
  return {
    from_waypoint: from.id,
    to_waypoint: to.id,
    mode: isFallback ? 'offroad' : 'routed',
    geometry: feature?.geometry as GeoJSON.LineString | undefined,
    control_points: isFallback ? [from.latlng, to.latlng] : [],
    smooth: false,
    distance_m: (feature?.properties?.['track-length'] as number) ?? 0,
    elevation_gain: (feature?.properties?.['filtered ascend'] as number) ?? 0,
    elevation_loss: (feature?.properties?.['filtered descend'] as number) ?? 0,
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
    staleTime: 5 * 60 * 1000, // 5 min — changes only on server restart
  })

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

      setState(s => {
        const updated = assignWaypointTypes([...s.waypoints, newWp])
        return { ...s, waypoints: updated }
      })

      // Calculate segment between previous last waypoint and the new one
      setState(s => {
        if (s.waypoints.length < 2) return s
        const prevWp = s.waypoints[s.waypoints.length - 2]
        // Trigger segment calculation async (state setter is synchronous guard only)
        void _calculateSegment(prevWp, newWp, s.profile)
        return s
      })
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
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
      setState(s => ({
        ...s,
        waypoints: s.waypoints.map(w =>
          w.id === wpId ? { ...w, latlng: newLatlng } : w,
        ),
      }))
      // Recalculate adjacent segments
      setState(s => {
        const idx = s.waypoints.findIndex(w => w.id === wpId)
        if (idx === -1) return s
        if (idx > 0) {
          void _calculateSegment(s.waypoints[idx - 1], s.waypoints[idx], s.profile)
        }
        if (idx < s.waypoints.length - 1) {
          void _calculateSegment(s.waypoints[idx], s.waypoints[idx + 1], s.profile)
        }
        return s
      })
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  const reorderWaypoints = useCallback((newOrder: RouteWaypoint[]) => {
    setState(s => {
      const retyped = assignWaypointTypes(newOrder)
      return { ...s, waypoints: retyped, segments: [] } // segments are invalid after reorder
    })
  }, [])

  // ── Segment calculation ─────────────────────────────────────────────────────

  // Internal helper — not directly returned (use calculateSegment wrapper)
  async function _calculateSegment(
    from: RouteWaypoint,
    to: RouteWaypoint,
    profile: string,
  ): Promise<void> {
    setState(s => ({ ...s, isCalculating: true, calculationError: null }))
    try {
      const result = await computeRoute([from.latlng, to.latlng], profile)
      const feature = result.features[0]
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
  }

  const calculateSegment = useCallback(
    (from: RouteWaypoint, to: RouteWaypoint) => {
      return _calculateSegment(from, to, state.profile)
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [state.profile],
  )

  const recalculateAll = useCallback(async () => {
    let currentWaypoints: RouteWaypoint[] = []
    let currentProfile = ''
    setState(s => {
      currentWaypoints = s.waypoints
      currentProfile = s.profile
      return { ...s, segments: [], isCalculating: true, calculationError: null }
    })
    for (let i = 0; i < currentWaypoints.length - 1; i++) {
      await _calculateSegment(currentWaypoints[i], currentWaypoints[i + 1], currentProfile)
    }
    setState(s => ({ ...s, isCalculating: false }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [_calculateSegment])

  // ── Offroad toggle ──────────────────────────────────────────────────────────

  const toggleOffroad = useCallback((segmentIndex: number) => {
    setState(s => {
      const segments = [...s.segments]
      const seg = { ...segments[segmentIndex] }
      if (seg.mode === 'routed') {
        seg.mode = 'offroad'
        const fromWp = s.waypoints.find(w => w.id === seg.from_waypoint)
        const toWp = s.waypoints.find(w => w.id === seg.to_waypoint)
        seg.control_points =
          fromWp && toWp ? [fromWp.latlng, toWp.latlng] : seg.control_points
        seg.geometry = undefined
      } else {
        seg.mode = 'routed'
        seg.control_points = []
        // Trigger recalculation asynchronously
        const fromWp = s.waypoints.find(w => w.id === seg.from_waypoint)
        const toWp = s.waypoints.find(w => w.id === seg.to_waypoint)
        if (fromWp && toWp) {
          void _calculateSegment(fromWp, toWp, s.profile)
        }
      }
      segments[segmentIndex] = seg
      return { ...s, segments }
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── Profile ─────────────────────────────────────────────────────────────────

  const setProfile = useCallback((profile: string) => {
    setState(s => ({ ...s, profile }))
  }, [])

  const setProfileAndRecalculate = useCallback(
    async (profile: string) => {
      let currentWaypoints: RouteWaypoint[] = []
      setState(s => {
        currentWaypoints = s.waypoints
        return { ...s, profile, segments: [], isCalculating: true, calculationError: null }
      })
      for (let i = 0; i < currentWaypoints.length - 1; i++) {
        await _calculateSegment(currentWaypoints[i], currentWaypoints[i + 1], profile)
      }
      setState(s => ({ ...s, isCalculating: false }))
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [_calculateSegment],
  )

  const setSportType = useCallback((sportType: string) => {
    setState(s => ({ ...s, sportType }))
  }, [])

  // ── Load route into planner ─────────────────────────────────────────────────

  const loadRoute = useCallback((route: PlannedRoute) => {
    setState({
      waypoints: route.waypoints,
      segments: route.segments,
      profile: route.profile,
      sportType: route.sport_type,
      isCalculating: false,
      calculationError: null,
      editingRouteId: route.id,
    })
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
      profile: s.profile,
      sportType: s.sportType,
    }))
  }, [])

  // ── Save mutation ────────────────────────────────────────────────────────────

  const saveRouteMutation = useMutation({
    mutationFn: (overrides: Partial<PlannedRoute>) => {
      const payload: Partial<PlannedRoute> = {
        waypoints: state.waypoints,
        segments: state.segments,
        profile: state.profile,
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

  // ── Elevation profile query ─────────────────────────────────────────────────

  // Build combined geometry from all segments for elevation profile
  const combinedCoordinates = state.segments
    .flatMap(seg => seg.geometry?.coordinates ?? [])

  const combinedGeometry: GeoJSON.LineString | null =
    combinedCoordinates.length >= 2
      ? { type: 'LineString', coordinates: combinedCoordinates }
      : null

  const elevationProfileQuery = useQuery<{ profile: ElevationPoint[] }>({
    queryKey: [
      'route-elevation-profile',
      state.editingRouteId,
      combinedCoordinates.length,
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
    profile: state.profile,
    sportType: state.sportType,
    isCalculating: state.isCalculating,
    calculationError: state.calculationError,
    editingRouteId: state.editingRouteId,
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

    // Profile/sport
    setProfile,
    setProfileAndRecalculate,
    setSportType,

    // Route lifecycle
    loadRoute,
    loadWaypoints,
    clear,

    // React Query: save
    saveRoute: saveRouteMutation.mutate,
    saveRouteAsync: saveRouteMutation.mutateAsync,
    isSaving: saveRouteMutation.isPending,
    saveError: saveRouteMutation.error,

    // React Query: capabilities
    capabilities: capabilitiesQuery.data ?? null,
    capabilitiesLoading: capabilitiesQuery.isLoading,

    // React Query: elevation profile
    elevationProfile: elevationProfileQuery.data?.profile ?? null,
    elevationProfileLoading: elevationProfileQuery.isLoading,

    // Factory queries (call in component)
    useRouteList,
    useRouteDetail,
  }
}

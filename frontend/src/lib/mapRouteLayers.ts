// Perf (Wave 3, Fix 5): re-enforce route-layer z-order.
//
// Overlays (waters/forests/summits/visited-pois/pois/segments) call `addLayer`
// without `beforeId`, which puts them at the top of the MapLibre stack above
// the route and the A/B waypoints. This helper moves every route-related layer
// back to the top in bottom-to-top order. It is idempotent and tolerant of
// missing layers during first-mount races.
//
// Deliberate order:
//   Casing < Routed < Offroad < Approach < Hit-Area < Calculating-Glow <
//   Calculating < CP-Circle < VF-Unknown < Scale-Base < Range-Highlight <
//   Summit-Markers < Highlight-Overlay (yellow) < WP-Circle < WP-Label
// Waypoint markers and labels stay at the very top (symbol layers A/B), with
// lines and polygon overlays below them. These IDs duplicate constants from
// useRouteEditor.ts and MapPage.tsx intentionally: the strings are stable, and
// importing them here would introduce a cycle between MapPage/useRouteEditor
// and this shared helper.
import type maplibregl from 'maplibre-gl'

const ROUTE_LAYERS_BOTTOM_TO_TOP = [
  'route-planner-line-casing',
  'route-planner-line',
  'route-planner-offroad',
  'route-planner-approach',
  'route-planner-line-hit',
  'route-planner-calculating-glow',
  'route-planner-calculating',
  'route-planner-control-points',
  'route-vf-unknown-layer',
  'route-scale-base-layer',
  'route-highlight-layer',
  'route-summit-markers-layer',
  'route-planner-highlight',
  'route-planner-waypoints',
  'route-planner-waypoints-label',
] as const

export function enforceRouteLayersOnTop(map: maplibregl.Map): void {
  for (const id of ROUTE_LAYERS_BOTTOM_TO_TOP) {
    if (map.getLayer(id)) {
      try {
        map.moveLayer(id) // no beforeId → moves to top of stack
      } catch {
        // tolerate concurrent removal/style swap
      }
    }
  }
}

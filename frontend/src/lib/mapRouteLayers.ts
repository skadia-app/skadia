// Perf (Wave 3, Fix 5): Z-Order Re-Enforce für Route-Layer.
//
// Overlays (waters/forests/summits/visited-pois/pois/segments) rufen `addLayer`
// ohne `beforeId` und landen damit oben im MapLibre-Stack — über der Route
// sowie den A/B-Wegpunkten. Diese Helper-Funktion zieht alle Route-relevanten
// Layer (in Bottom→Top-Order) wieder ans Stack-Ende. Idempotent + tolerant
// gegen fehlende Layer (race auf erstem Mount).
//
// Reihenfolge bewusst gewählt:
//   Casing < Routed < Offroad < Approach < Hit-Area < Calculating-Glow <
//   Calculating < CP-Circle < VF-Unknown < Scale-Base < Range-Highlight <
//   Summit-Markers < Highlight-Overlay (yellow) < WP-Circle < WP-Label
// → Wegpunkt-Marker + Labels stehen ganz oben (Symbol-Layer A/B), unter ihnen
//   die Linien/Polygonal-Overlays. Die IDs sind doppelte Wahrheiten zu
//   useRouteEditor.ts + MapPage.tsx (dortige const-Anker), aber bewusst hier
//   inline statt aus useRouteEditor zu importieren — die Strings sind stabil
//   und ein Re-Export würde nur den Cycle-Import zwischen MapPage / useRouteEditor
//   und einem geteilten Lib provozieren.
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

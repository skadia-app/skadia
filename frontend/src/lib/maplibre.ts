import * as maplibregl from 'maplibre-gl'
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url'

// MapLibre GL JS v6 loads its Web Worker externally instead of using an inlined
// Blob worker. Register Vite's emitted worker asset URL once before any Map is
// constructed; check presence via `in` before property access so partial Vitest
// module mocks of 'maplibre-gl' (whose Proxy throws on missing exports) do not fail.
if (
  'setWorkerUrl' in maplibregl &&
  typeof maplibregl.setWorkerUrl === 'function' &&
  workerUrl
) {
  maplibregl.setWorkerUrl(workerUrl)
}

export * from 'maplibre-gl'

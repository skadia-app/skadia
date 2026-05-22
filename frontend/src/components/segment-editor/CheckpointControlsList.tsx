import { ArrowDown, ArrowUp, X } from 'lucide-react'
import type { CheckpointControl } from '@/api/segments'
import ZoneEditor, { type ZoneData } from '@/components/ZoneEditor'

export function CheckpointControlsList(props: {
  controls: CheckpointControl[]
  onRemove: (index: number) => void
  onMove: (index: number, direction: -1 | 1) => void
  onUpdate: (index: number, patch: Partial<CheckpointControl>) => void
}) {
  if (props.controls.length === 0) return null
  return (
    <div className="border-t border-neutral-200 bg-white p-3 text-sm dark:border-neutral-800 dark:bg-neutral-900">
      <div className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
        Checkpoints
      </div>
      <div className="space-y-1.5">
        {props.controls.map((control, index) => {
          const isMultipolygon = control.geometry_shape === 'multipolygon'
          return (
            <div key={checkpointControlKey(control, index)} className="rounded-md border border-neutral-200 dark:border-neutral-700">
              <div className="flex items-center gap-2 px-2 py-1.5">
                <span className="flex h-6 min-w-6 items-center justify-center rounded-full bg-orange-100 text-xs font-semibold text-orange-800 dark:bg-orange-950 dark:text-orange-200">
                  {index === 0 ? 'S' : index === props.controls.length - 1 ? 'Z' : index + 1}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-neutral-800 dark:text-neutral-100">
                    {index === 0 ? 'Start' : index === props.controls.length - 1 ? 'Ziel' : `Checkpoint ${index + 1}`}
                  </div>
                  <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[11px] text-neutral-500 dark:text-neutral-400">
                    <select
                      value={isMultipolygon || control.geometry_shape === 'circle' || control.geometry_shape === 'polygon' ? control.geometry_shape : 'point'}
                      disabled={isMultipolygon}
                      onChange={(e) => {
                        if (isMultipolygon) return
                        const next = e.target.value
                        if (next === 'circle') {
                          const radius = control.area_radius_m ?? control.hit_radius_m ?? 30
                          props.onUpdate(index, {
                            primitive: 'area',
                            geometry_shape: 'circle',
                            geometry: pointGeometryForControl(control),
                            hit_radius_m: undefined,
                            area_radius_m: radius,
                          })
                        } else if (next === 'polygon') {
                          props.onUpdate(index, {
                            primitive: 'area',
                            geometry_shape: 'polygon',
                            hit_radius_m: undefined,
                            area_radius_m: undefined,
                            geometry: polygonAroundControl(control),
                          })
                        } else {
                          const radius = control.hit_radius_m ?? control.area_radius_m ?? 30
                          props.onUpdate(index, {
                            primitive: 'point',
                            geometry_shape: 'point',
                            geometry: pointGeometryForControl(control),
                            hit_radius_m: radius,
                            area_radius_m: undefined,
                          })
                        }
                      }}
                      className="rounded border border-neutral-200 bg-white px-1 py-0.5 disabled:text-neutral-500 dark:border-neutral-700 dark:bg-neutral-900"
                      aria-label="Checkpoint-Form"
                    >
                      <option value="point">Punkt</option>
                      <option value="circle">Kreis</option>
                      <option value="polygon">Polygon</option>
                      <option value="multipolygon">MultiPolygon</option>
                    </select>
                    {control.geometry_shape !== 'polygon' && !isMultipolygon && (
                      <label className="inline-flex items-center gap-1">
                        Radius
                        <input
                          type="number"
                          min={1}
                          max={5000}
                          step={1}
                          value={Math.round(control.geometry_shape === 'circle' ? control.area_radius_m ?? 30 : control.hit_radius_m ?? 30)}
                          onChange={(e) => {
                            const radius = Math.min(5000, Math.max(1, Number(e.target.value) || 1))
                            props.onUpdate(index, control.geometry_shape === 'circle'
                              ? { area_radius_m: radius }
                              : { hit_radius_m: radius })
                          }}
                          className="w-16 rounded border border-neutral-200 bg-white px-1 py-0.5 dark:border-neutral-700 dark:bg-neutral-900"
                          aria-label="Checkpoint-Radius in Metern"
                        />
                        m
                      </label>
                    )}
                    {isMultipolygon && (
                      <span>Mehrfachfläche nur lesbar. Einzelne Flächen können hier nicht bearbeitet werden.</span>
                    )}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => props.onMove(index, -1)}
                  disabled={index === 0}
                  className="rounded p-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900 disabled:opacity-30 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-white"
                  aria-label="Checkpoint nach oben"
                >
                  <ArrowUp className="h-4 w-4" aria-hidden="true" />
                </button>
                <button
                  type="button"
                  onClick={() => props.onMove(index, 1)}
                  disabled={index === props.controls.length - 1}
                  className="rounded p-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900 disabled:opacity-30 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-white"
                  aria-label="Checkpoint nach unten"
                >
                  <ArrowDown className="h-4 w-4" aria-hidden="true" />
                </button>
                <button
                  type="button"
                  onClick={() => props.onRemove(index)}
                  className="rounded p-1 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-white"
                  aria-label="Checkpoint entfernen"
                >
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              </div>
              {control.geometry_shape === 'polygon' && (
                <div className="border-t border-neutral-200 p-2 dark:border-neutral-700">
                  <ZoneEditor
                    key={zoneEditorKey(control)}
                    value={zoneFromControl(control)}
                    hideModeSwitcher
                    enableHoles
                    minRadiusM={1}
                    className="h-72"
                    onChange={(zone) => {
                      if (zone.mode === 'polygon' && zone.polygon) {
                        props.onUpdate(index, {
                          primitive: 'area',
                          geometry_shape: 'polygon',
                          geometry: zone.polygon,
                          hit_radius_m: undefined,
                          area_radius_m: undefined,
                        })
                      }
                    }}
                  />
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function zoneFromControl(control: CheckpointControl): ZoneData {
  const geometry = control.geometry as GeoJSON.Geometry | undefined
  if (geometry?.type === 'Polygon') return { mode: 'polygon', polygon: geometry as GeoJSON.Polygon }
  return { mode: 'polygon', polygon: polygonAroundControl(control) }
}

function polygonAroundControl(control: CheckpointControl): GeoJSON.Polygon {
  const point = controlPoint(control) ?? [10, 47]
  const radiusM = control.area_radius_m ?? control.hit_radius_m ?? 50
  const latDelta = radiusM / 111_320
  const lngDelta = radiusM / (111_320 * Math.max(Math.cos(point[1] * Math.PI / 180), 0.01))
  return {
    type: 'Polygon',
    coordinates: [[
      [point[0] - lngDelta, point[1] - latDelta],
      [point[0] + lngDelta, point[1] - latDelta],
      [point[0] + lngDelta, point[1] + latDelta],
      [point[0] - lngDelta, point[1] + latDelta],
      [point[0] - lngDelta, point[1] - latDelta],
    ]],
  }
}

function controlPoint(control: CheckpointControl): [number, number] | null {
  const geometry = control.geometry as { type?: string; coordinates?: unknown } | undefined
  if (geometry?.type === 'Point' && Array.isArray(geometry.coordinates) && geometry.coordinates.length >= 2) {
    const [lng, lat] = geometry.coordinates
    if (typeof lng === 'number' && typeof lat === 'number') return [lng, lat]
  }
  if (geometry?.coordinates) {
    const coords: [number, number][] = []
    const collect = (value: unknown): void => {
      if (!Array.isArray(value)) return
      if (value.length >= 2 && typeof value[0] === 'number' && typeof value[1] === 'number') {
        coords.push([value[0], value[1]])
        return
      }
      value.forEach(collect)
    }
    collect(geometry.coordinates)
    if (coords.length > 0) {
      const [lng, lat] = coords.reduce(([lngSum, latSum], coord) => [lngSum + coord[0], latSum + coord[1]], [0, 0])
      return [lng / coords.length, lat / coords.length]
    }
  }
  return null
}

function pointGeometryForControl(control: CheckpointControl): GeoJSON.Point {
  return { type: 'Point', coordinates: controlPoint(control) ?? [10, 47] }
}

function checkpointControlKey(control: CheckpointControl, index: number): string {
  if (control.id) return control.id
  return `${control.geometry_shape}:${safeGeometryKey(control.geometry)}:${index}`
}

function zoneEditorKey(control: CheckpointControl): string {
  return `${control.id ?? 'new'}:${safeGeometryKey(control.geometry)}`
}

function safeGeometryKey(geometry: unknown): string {
  try {
    return JSON.stringify(geometry) ?? ''
  } catch {
    return ''
  }
}

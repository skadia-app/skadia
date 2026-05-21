import { ArrowDown, ArrowUp, X } from 'lucide-react'
import type { CheckpointControl } from '@/api/segments'

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
        {props.controls.map((control, index) => (
          <div
            key={`${control.sequence_index}-${index}`}
            className="flex items-center gap-2 rounded-md border border-neutral-200 px-2 py-1.5 dark:border-neutral-700"
          >
            <span className="flex h-6 min-w-6 items-center justify-center rounded-full bg-orange-100 text-xs font-semibold text-orange-800 dark:bg-orange-950 dark:text-orange-200">
              {index === 0 ? 'S' : index === props.controls.length - 1 ? 'Z' : index + 1}
            </span>
            <div className="min-w-0 flex-1">
              <div className="truncate text-neutral-800 dark:text-neutral-100">
                {index === 0 ? 'Start' : index === props.controls.length - 1 ? 'Ziel' : `Checkpoint ${index + 1}`}
              </div>
              <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[11px] text-neutral-500 dark:text-neutral-400">
                <select
                  value={control.geometry_shape === 'circle' ? 'circle' : 'point'}
                  onChange={(e) => {
                    const next = e.target.value
                    if (next === 'circle') {
                      const radius = control.area_radius_m ?? control.hit_radius_m ?? 30
                      props.onUpdate(index, {
                        primitive: 'area',
                        geometry_shape: 'circle',
                        hit_radius_m: undefined,
                        area_radius_m: radius,
                      })
                    } else {
                      const radius = control.hit_radius_m ?? control.area_radius_m ?? 30
                      props.onUpdate(index, {
                        primitive: 'point',
                        geometry_shape: 'point',
                        hit_radius_m: radius,
                        area_radius_m: undefined,
                      })
                    }
                  }}
                  className="rounded border border-neutral-200 bg-white px-1 py-0.5 dark:border-neutral-700 dark:bg-neutral-900"
                  aria-label="Checkpoint-Form"
                >
                  <option value="point">Punkt</option>
                  <option value="circle">Kreis</option>
                </select>
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
        ))}
      </div>
    </div>
  )
}

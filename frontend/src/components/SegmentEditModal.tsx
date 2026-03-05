import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { updateSegment, type Segment } from '@/api/segments'

interface Props {
  segment: Segment
  onClose: () => void
}

const SAC_SCALES = ['T1', 'T2', 'T3', 'T4', 'T5', 'T6']
const MTB_SCALES = ['S0', 'S1', 'S2', 'S3', 'S4', 'S5']

export default function SegmentEditModal({ segment, onClose }: Props) {
  const qc = useQueryClient()
  const [name, setName] = useState(segment.name)
  const [description, setDescription] = useState(segment.description ?? '')
  const [diffColor, setDiffColor] = useState((segment.segment_type_meta?.difficulty_color as string) ?? '')
  const [sacScale, setSacScale] = useState((segment.segment_type_meta?.sac_scale as string) ?? '')
  const [mtbScale, setMtbScale] = useState((segment.segment_type_meta?.mtb_scale as string) ?? '')
  const [newVersion, setNewVersion] = useState(false)

  const mutation = useMutation({
    mutationFn: () => {
      const meta: Record<string, unknown> = { ...segment.segment_type_meta }
      if (segment.segment_type === 'trail') {
        if (diffColor) meta.difficulty_color = diffColor; else delete meta.difficulty_color
        if (sacScale) meta.sac_scale = sacScale; else delete meta.sac_scale
        if (mtbScale) meta.mtb_scale = mtbScale; else delete meta.mtb_scale
      }
      return updateSegment(segment.id, {
        name: name.trim(),
        description: description.trim() || undefined,
        segment_type_meta: Object.keys(meta).length > 0 ? meta : undefined,
        new_version: newVersion || undefined,
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['segments'] })
      onClose()
    },
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div
        className="bg-neutral-900 rounded-xl border border-neutral-800 p-6 w-full max-w-md space-y-4"
        onClick={e => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold text-neutral-100">Segment bearbeiten</h2>

        <input
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder="Name *"
          className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100"
        />

        <textarea
          value={description}
          onChange={e => setDescription(e.target.value)}
          placeholder="Beschreibung (optional)"
          rows={2}
          className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100"
        />

        {segment.segment_type === 'trail' && (
          <div className="space-y-2">
            <select
              value={diffColor}
              onChange={e => setDiffColor(e.target.value)}
              className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100"
            >
              <option value="">Farbe — keine</option>
              <option value="blue">Blau</option>
              <option value="red">Rot</option>
              <option value="black">Schwarz</option>
            </select>
            <select
              value={sacScale}
              onChange={e => setSacScale(e.target.value)}
              className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100"
            >
              <option value="">SAC-Skala — keine</option>
              {SAC_SCALES.map(s => <option key={s} value={s}>{s}</option>)}
            </select>
            <select
              value={mtbScale}
              onChange={e => setMtbScale(e.target.value)}
              className="w-full border border-neutral-700 rounded-lg px-3 py-2 text-sm bg-neutral-800 text-neutral-100"
            >
              <option value="">MTB-Skala — keine</option>
              {MTB_SCALES.map(s => <option key={s} value={s}>{s}</option>)}
            </select>
          </div>
        )}

        <label className="flex items-start gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={newVersion}
            onChange={e => setNewVersion(e.target.checked)}
            className="mt-0.5 rounded border-neutral-600"
          />
          <span className="text-sm text-neutral-300">
            Neue Version erstellen
            <span className="block text-xs text-neutral-500 mt-0.5">
              Nur aktivieren wenn sich die Route grundlegend ändert.
              Bestehende Bestzeiten bleiben der alten Version zugeordnet.
            </span>
          </span>
        </label>

        {mutation.isError && (
          <p className="text-sm text-red-400">Fehler beim Speichern.</p>
        )}

        <div className="flex gap-2 justify-end">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-sm border border-neutral-700 rounded-lg text-neutral-300 hover:bg-neutral-800"
          >
            Abbrechen
          </button>
          <button
            onClick={() => mutation.mutate()}
            disabled={!name.trim() || mutation.isPending}
            className="px-4 py-1.5 text-sm bg-accent-DEFAULT rounded-lg text-white hover:bg-accent-600 disabled:opacity-50"
          >
            {mutation.isPending ? 'Speichern...' : 'Speichern'}
          </button>
        </div>
      </div>
    </div>
  )
}

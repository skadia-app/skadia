import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { updateSegment, publishSegment, type Segment, type UpdateSegmentPayload } from '@/api/segments'
import { ModalShell } from '@/components/ui/ModalShell'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'

interface Props {
  segment: Segment
  onClose: () => void
}

const SAC_SCALES = [
  { value: 'T1', label: 'T1 — Wandern' },
  { value: 'T2', label: 'T2 — Bergwandern' },
  { value: 'T3', label: 'T3 — Anspruchsvolles Bergwandern' },
  { value: 'T4', label: 'T4 — Alpinwandern' },
  { value: 'T5', label: 'T5 — Anspruchsvolles Alpinwandern' },
  { value: 'T6', label: 'T6 — Schwieriges Alpinwandern' },
]
const MTB_SCALES = [
  { value: 'S0', label: 'S0 — Leicht' },
  { value: 'S1', label: 'S1 — Leicht technisch' },
  { value: 'S2', label: 'S2 — Technisch' },
  { value: 'S3', label: 'S3 — Schwer technisch' },
  { value: 'S4', label: 'S4 — Sehr schwer' },
  { value: 'S5', label: 'S5 — Extrem' },
]

export default function SegmentEditModal({ segment, onClose }: Props) {
  const qc = useQueryClient()
  const [name, setName] = useState(segment.name)
  const [description, setDescription] = useState(segment.description ?? '')
  const [diffColor, setDiffColor] = useState((segment.segment_type_meta?.difficulty_color as string) ?? '')
  const [sacScale, setSacScale] = useState((segment.segment_type_meta?.sac_scale as string) ?? '')
  const [mtbScale, setMtbScale] = useState((segment.segment_type_meta?.mtb_scale as string) ?? '')
  const [newVersion, setNewVersion] = useState(false)
  const initialClimbType = segment.climb_type ?? 'standard'
  const [isMountain, setIsMountain] = useState(initialClimbType === 'bergauf' || initialClimbType === 'bergab')
  const [climbDirection, setClimbDirection] = useState<'bergauf' | 'bergab'>(
    initialClimbType === 'bergab' ? 'bergab' : 'bergauf'
  )
  const [visibility, setVisibility] = useState<'public' | 'private'>(
    (segment.visibility as 'public' | 'private') ?? 'public'
  )
  const { openModal } = useModal()
  const { addToast } = useToast()

  function getClimbType(): string {
    if (segment.segment_type === 'lap') return 'standard'
    if (isMountain) return climbDirection
    return 'standard'
  }

  const mutation = useMutation({
    mutationFn: () => {
      const meta: Record<string, unknown> = { ...segment.segment_type_meta }
      if (segment.segment_type === 'trail') {
        if (diffColor) meta.difficulty_color = diffColor; else delete meta.difficulty_color
        if (sacScale) meta.sac_scale = sacScale; else delete meta.sac_scale
        if (mtbScale) meta.mtb_scale = mtbScale; else delete meta.mtb_scale
      }
      const payload: UpdateSegmentPayload = {
        name: name.trim(),
        description: description.trim() || undefined,
        segment_type_meta: Object.keys(meta).length > 0 ? meta : undefined,
        new_version: newVersion || undefined,
        climb_type: getClimbType(),
      }
      // Only pass visibility for public→private (private→public uses the publish endpoint).
      if (segment.visibility === 'public' && visibility === 'private') {
        payload.visibility = 'private'
      }
      return updateSegment(segment.id, payload)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['segments'] })
      qc.invalidateQueries({ queryKey: ['segment', segment.id] })
      onClose()
    },
  })

  const publishMut = useMutation({
    mutationFn: () => publishSegment(segment.id),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Segment veröffentlicht', message: 'Ein Recompute wurde gestartet.' })
      qc.invalidateQueries({ queryKey: ['segments'] })
      qc.invalidateQueries({ queryKey: ['segment', segment.id] })
      onClose()
    },
    onError: () => {
      addToast({ type: 'error', title: 'Veröffentlichung fehlgeschlagen', duration: 0 })
    },
  })

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md" className="bg-neutral-900 rounded-xl border border-neutral-800 space-y-4">
        <h2 className="text-lg font-semibold text-neutral-100">Segment bearbeiten</h2>

        <input
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder="Name *"
          className={AUTH_INPUT_CLASS}
        />

        <textarea
          value={description}
          onChange={e => setDescription(e.target.value)}
          placeholder="Beschreibung (optional)"
          rows={2}
          className={`${AUTH_INPUT_CLASS} resize-none`}
        />

        {segment.segment_type === 'trail' && (
          <div className="space-y-2">
            <select
              value={diffColor}
              onChange={e => setDiffColor(e.target.value)}
              className={AUTH_INPUT_CLASS}
            >
              <option value="">Farbe — keine</option>
              <option value="blue">Blau (leicht)</option>
              <option value="red">Rot (mittel)</option>
              <option value="black">Schwarz (schwer)</option>
            </select>
            <select
              value={sacScale}
              onChange={e => setSacScale(e.target.value)}
              className={AUTH_INPUT_CLASS}
            >
              <option value="">SAC-Skala — keine</option>
              {SAC_SCALES.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
            </select>
            <select
              value={mtbScale}
              onChange={e => setMtbScale(e.target.value)}
              className={AUTH_INPUT_CLASS}
            >
              <option value="">MTB-Skala — keine</option>
              {MTB_SCALES.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
            </select>
          </div>
        )}

        {/* Berg-Checkbox (nicht bei Lap-Segmenten) */}
        {segment.segment_type !== 'lap' && (
          <div className="space-y-2">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={isMountain}
                onChange={e => setIsMountain(e.target.checked)}
                className="rounded border-neutral-600"
              />
              <span className="text-sm text-neutral-300">Berg (Auf-/Abstieg unterscheiden)</span>
            </label>

            {isMountain && (
              <div className="ml-6 flex gap-4">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="radio"
                    name="edit-climb-dir"
                    checked={climbDirection === 'bergauf'}
                    onChange={() => setClimbDirection('bergauf')}
                    className="border-neutral-600"
                  />
                  <span className="text-sm text-neutral-300">Bergauf</span>
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="radio"
                    name="edit-climb-dir"
                    checked={climbDirection === 'bergab'}
                    onChange={() => setClimbDirection('bergab')}
                    className="border-neutral-600"
                  />
                  <span className="text-sm text-neutral-300">Bergab</span>
                </label>
              </div>
            )}
          </div>
        )}

        {/* Visibility toggle — not for lift segments */}
        {segment.segment_type !== 'lift' && (
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setVisibility('public')}
                className={`flex-1 py-1.5 text-sm rounded-lg border cursor-pointer transition-colors ${
                  visibility === 'public'
                    ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                    : 'border-neutral-700 text-neutral-300 hover:bg-neutral-800'
                }`}
              >
                Öffentlich
              </button>
              <button
                type="button"
                onClick={() => setVisibility('private')}
                className={`flex-1 py-1.5 text-sm rounded-lg border cursor-pointer transition-colors ${
                  visibility === 'private'
                    ? 'bg-neutral-700 text-neutral-100 border-neutral-600'
                    : 'border-neutral-700 text-neutral-300 hover:bg-neutral-800'
                }`}
              >
                Privat
              </button>
            </div>
            {segment.visibility === 'private' && visibility === 'public' && (
              <p className="text-xs text-amber-400">
                Veröffentlichen: alle User können das Segment sehen. Ein Recompute-Job startet.
              </p>
            )}
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
            className="cursor-pointer px-3 py-1.5 text-sm border border-neutral-700 rounded-lg text-neutral-300 hover:bg-neutral-800"
          >
            Abbrechen
          </button>
          <button
            onClick={() => {
              if (segment.visibility === 'private' && visibility === 'public') {
                openModal({
                  title: 'Segment veröffentlichen?',
                  message: 'Alle Nutzer können das Segment danach sehen. Ein Recompute-Job wird gestartet.',
                  buttons: [
                    { label: 'Abbrechen', variant: 'default', onClick: () => {} },
                    { label: 'Veröffentlichen', variant: 'destructive', onClick: () => publishMut.mutate() },
                  ],
                })
              } else {
                mutation.mutate()
              }
            }}
            disabled={!name.trim() || mutation.isPending || publishMut.isPending}
            className="cursor-pointer px-4 py-1.5 text-sm bg-accent-DEFAULT rounded-lg text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {mutation.isPending || publishMut.isPending ? 'Speichern...' : 'Speichern'}
          </button>
        </div>
    </ModalShell>
  )
}

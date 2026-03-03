import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { createSegment, type CreateSegmentPayload } from '@/api/segments'

const SPORT_GROUPS = [
  { value: 'bike', label: 'Rad' },
  { value: 'foot', label: 'Zu Fuss' },
  { value: 'swim', label: 'Schwimmen' },
  { value: 'ski', label: 'Ski' },
  { value: 'paddle', label: 'Paddeln' },
  { value: 'generic', label: 'Allgemein' },
]

const SAC_SCALES = [
  { value: 'T1', label: 'T1 Wanderweg' },
  { value: 'T2', label: 'T2 Bergwanderweg' },
  { value: 'T3', label: 'T3 Anspruchsvoller Bergwanderweg' },
  { value: 'T4', label: 'T4 Alpinwanderweg' },
  { value: 'T5', label: 'T5 Anspruchsvoller Alpinwanderweg' },
  { value: 'T6', label: 'T6 Schwierige Hochtour' },
]

const MTB_SCALES = [
  { value: 'S0', label: 'S0 Leicht' },
  { value: 'S1', label: 'S1 Einfach' },
  { value: 'S2', label: 'S2 Mittel' },
  { value: 'S3', label: 'S3 Schwer' },
  { value: 'S4', label: 'S4 Sehr schwer' },
  { value: 'S5', label: 'S5 Extrem schwer' },
]

const SURFACES = [
  'Asphalt', 'Schotter', 'Gras', 'Wald', 'Fels', 'Sand', 'Schnee', 'Eis', 'Gemischt',
]

interface Props {
  onSuccess: () => void
}

export default function SegmentCreationPanel({ onSuccess }: Props) {
  const [name, setName] = useState('')
  const [sportGroup, setSportGroup] = useState('bike')
  const [segmentType, setSegmentType] = useState('standard')
  const [sacScale, setSacScale] = useState('')
  const [mtbScale, setMtbScale] = useState('')
  const [surface, setSurface] = useState('')

  const mutation = useMutation({
    mutationFn: (payload: CreateSegmentPayload) => createSegment(payload),
    onSuccess: () => {
      setName('')
      setSportGroup('bike')
      setSegmentType('standard')
      setSacScale('')
      setMtbScale('')
      setSurface('')
      onSuccess()
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return

    const meta: Record<string, unknown> = {}
    if (segmentType === 'trail') {
      if (sacScale) meta.sac_scale = sacScale
      if (mtbScale) meta.mtb_scale = mtbScale
    } else {
      if (surface) meta.surface = surface
    }

    mutation.mutate({
      name: name.trim(),
      sport_group: sportGroup,
      segment_type: segmentType,
      segment_type_meta: Object.keys(meta).length > 0 ? meta : undefined,
    })
  }

  return (
    <form onSubmit={handleSubmit} className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 space-y-4">
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Neues Segment erstellen</h2>

      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Name *</label>
        <input
          type="text"
          value={name}
          onChange={e => setName(e.target.value)}
          required
          placeholder="Segment-Name"
          className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm
                     bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                     placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                     focus:outline-none focus:ring-1 focus:ring-orange-400"
        />
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Sportgruppe</label>
          <select
            value={sportGroup}
            onChange={e => setSportGroup(e.target.value)}
            className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          >
            {SPORT_GROUPS.map(g => (
              <option key={g.value} value={g.value}>{g.label}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Typ</label>
          <select
            value={segmentType}
            onChange={e => setSegmentType(e.target.value)}
            className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          >
            <option value="standard">Standard</option>
            <option value="trail">Trail</option>
          </select>
        </div>
      </div>

      {/* Trail-specific options */}
      {segmentType === 'trail' && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">SAC-Skala</label>
            <select
              value={sacScale}
              onChange={e => setSacScale(e.target.value)}
              className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm
                         bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                         focus:outline-none focus:ring-1 focus:ring-neutral-400"
            >
              <option value="">-- Keine --</option>
              {SAC_SCALES.map(s => (
                <option key={s.value} value={s.value}>{s.label}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">MTB-Skala</label>
            <select
              value={mtbScale}
              onChange={e => setMtbScale(e.target.value)}
              className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm
                         bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                         focus:outline-none focus:ring-1 focus:ring-neutral-400"
            >
              <option value="">-- Keine --</option>
              {MTB_SCALES.map(s => (
                <option key={s.value} value={s.value}>{s.label}</option>
              ))}
            </select>
          </div>
        </div>
      )}

      {/* Standard-specific options */}
      {segmentType === 'standard' && (
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Oberflaeche</label>
          <select
            value={surface}
            onChange={e => setSurface(e.target.value)}
            className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          >
            <option value="">-- Keine --</option>
            {SURFACES.map(s => (
              <option key={s} value={s.toLowerCase()}>{s}</option>
            ))}
          </select>
        </div>
      )}

      {mutation.isError && (
        <p className="text-sm text-red-500">Fehler beim Erstellen des Segments.</p>
      )}

      <button
        type="submit"
        disabled={!name.trim() || mutation.isPending}
        className="px-4 py-2 text-sm font-medium rounded-lg bg-orange-500 text-white
                   hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        {mutation.isPending ? 'Erstelle...' : 'Segment erstellen'}
      </button>
    </form>
  )
}

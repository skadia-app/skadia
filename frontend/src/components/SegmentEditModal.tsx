import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchSegment, updateSegment, publishSegment, reverseSegment, type Segment, type UpdateSegmentPayload } from '@/api/segments'
import { ModalShell } from '@/components/ui/ModalShell'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'
import { SPORT_DEFS } from '@/lib/sportTaxonomy'

// Shared Components
import { DifficultySelects } from './segment-editor/DifficultySelects'
import { SurfaceDropdown } from './segment-editor/SurfaceDropdown'
import { DifficultyColorPicker } from './segment-editor/DifficultyColorPicker'
import { ClimbTypePills } from './segment-editor/ClimbTypePills'
import { MapPreview } from './segment-editor/MapPreview'

interface Props {
  segment: Segment
  onClose: () => void
}

export default function SegmentEditModal({ segment: initialSegment, onClose }: Props) {
  const qc = useQueryClient()
  
  // Load full segment data including GeoJSON if missing
  const { data: segment = initialSegment, isLoading: isLoadingFull } = useQuery({
    queryKey: ['segment', initialSegment.id, 'full'],
    queryFn: () => fetchSegment(initialSegment.id, { includeGeoJSON: true }),
    staleTime: 5 * 60 * 1000,
  })

  const [name, setName] = useState(initialSegment.name)
  const [description, setDescription] = useState(initialSegment.description ?? '')
  const [diffColor, setDiffColor] = useState((initialSegment.segment_type_meta?.difficulty_color as 'blue' | 'red' | 'black' | null) ?? null)
  const [sacScale, setSacScale] = useState((initialSegment.segment_type_meta?.sac_scale as string | null) ?? null)
  const [mtbScale, setMtbScale] = useState((initialSegment.segment_type_meta?.mtb_scale as string | null) ?? null)
  const [surface, setSurface] = useState((initialSegment.segment_type_meta?.surface as string | null) ?? null)
  const [newVersion, setNewVersion] = useState(false)
  
  const initialClimbType = initialSegment.climb_type ?? 'standard'
  const [climbType, setClimbType] = useState<any>(initialClimbType)
  
  const [visibility, setVisibility] = useState<'public' | 'private'>(
    (initialSegment.visibility as 'public' | 'private') ?? 'public'
  )
  const [allowedSportTypes, setAllowedSportTypes] = useState<string[] | null>(
    initialSegment.allowed_sport_types ?? null
  )
  const [bidirectional, setBidirectional] = useState<boolean>(initialSegment.bidirectional ?? true)
  
  const { openModal } = useModal()
  const { addToast } = useToast()

  const groupSportDefs = SPORT_DEFS.filter(d => d.segmentGroup === initialSegment.sport_group)

  const mutation = useMutation({
    mutationFn: () => {
      const meta: Record<string, unknown> = { ...segment.segment_type_meta }
      if (segment.segment_type === 'trail') {
        if (sacScale) meta.sac_scale = sacScale; else delete meta.sac_scale
        if (mtbScale) meta.mtb_scale = mtbScale; else delete meta.mtb_scale
        if (surface) meta.surface = surface; else delete meta.surface
        // difficultyColor is auto-derived in backend for trails
      }
      if (segment.segment_type === 'wintersport') {
        if (diffColor) meta.difficulty_color = diffColor; else delete meta.difficulty_color
      }
      
      const payload: UpdateSegmentPayload = {
        name: name.trim(),
        description: description.trim() || undefined,
        segment_type_meta: Object.keys(meta).length > 0 ? meta : undefined,
        new_version: newVersion || undefined,
        climb_type: segment.segment_type === 'lap' ? 'standard' : climbType,
        bidirectional,
        allowed_sport_types: allowedSportTypes ?? [],
      }
      if (initialSegment.visibility === 'public' && visibility === 'private') {
        payload.visibility = 'private'
      }
      return updateSegment(initialSegment.id, payload)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['segments'] })
      qc.invalidateQueries({ queryKey: ['segment', initialSegment.id] })
      onClose()
    },
  })

  const publishMut = useMutation({
    mutationFn: () => publishSegment(initialSegment.id),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Segment published', message: 'Recompute started.' })
      qc.invalidateQueries({ queryKey: ['segments'] })
      qc.invalidateQueries({ queryKey: ['segment', initialSegment.id] })
      onClose()
    },
    onError: () => {
      addToast({ type: 'error', title: 'Publishing failed', duration: 0 })
    },
  })

  const reverseMutation = useMutation({
    mutationFn: () => reverseSegment(initialSegment.id),
    onSuccess: (resp) => {
      addToast({
        type: 'success',
        title: 'Segment reversed',
        message: resp.efforts_updated === 1 ? '1 Effort updated' : `${resp.efforts_updated} Efforts updated`,
      })
      qc.invalidateQueries({ queryKey: ['segment', initialSegment.id] })
      qc.invalidateQueries({ queryKey: ['segments'] })
      onClose()
    },
    onError: () => {
      addToast({ type: 'error', title: 'Reverse failed', duration: 0 })
    },
  })

  function handleReverse() {
    openModal({
      title: 'Reverse segment?',
      message: 'Reversing this segment cannot be undone.',
      buttons: [
        { label: 'Cancel', variant: 'default', onClick: () => {} },
        { label: 'Reverse', variant: 'destructive', onClick: () => reverseMutation.mutate() },
      ],
    })
  }

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md" className="bg-neutral-900 rounded-xl border border-neutral-800 space-y-4">
        <h2 className="text-lg font-semibold text-neutral-100">Edit Segment</h2>

        {/* Map Preview */}
        <MapPreview 
          geojsonTrack={segment.track_geojson} 
          height="160px" 
          className="mb-2"
        />

        <input
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder="Name *"
          className={AUTH_INPUT_CLASS}
        />

        <textarea
          value={description}
          onChange={e => setDescription(e.target.value)}
          placeholder="Description (optional)"
          rows={2}
          className={`${AUTH_INPUT_CLASS} resize-none`}
        />

        <div className="space-y-4">
          <DifficultySelects
            segType={segment.segment_type as any}
            sacScale={sacScale}
            mtbScale={mtbScale}
            onChange={(f, v) => f === 'sac' ? setSacScale(v) : setMtbScale(v)}
            disabled={isLoadingFull}
          />

          <SurfaceDropdown
            segType={segment.segment_type as any}
            value={surface}
            onChange={setSurface}
            disabled={isLoadingFull}
          />

          <DifficultyColorPicker
            segType={segment.segment_type as any}
            value={diffColor}
            onChange={setDiffColor}
            disabled={isLoadingFull}
          />
        </div>

        <div className="space-y-3">
          <div className="space-y-1.5">
            <label className="text-[11px] font-semibold tracking-wider text-neutral-500 uppercase">Climb Profile</label>
            <ClimbTypePills
              segType={segment.segment_type as any}
              value={climbType}
              onChange={setClimbType}
              disabled={isLoadingFull}
            />
          </div>

          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={bidirectional}
              onChange={e => setBidirectional(e.target.checked)}
              className="rounded border-neutral-600 accent-accent-DEFAULT"
            />
            <span className="text-sm text-neutral-300">Bidirectional matching</span>
          </label>
        </div>

        {groupSportDefs.length > 1 && (
          <div className="space-y-1">
            <p className="text-xs text-neutral-400">Allowed Sports <span className="text-neutral-500">(empty = all)</span></p>
            <div className="flex flex-wrap gap-x-4 gap-y-1">
              {groupSportDefs.map(d => (
                <label key={d.sportType} className="flex items-center gap-1.5 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={allowedSportTypes === null ? false : allowedSportTypes.includes(d.sportType)}
                    onChange={e => {
                      const current = allowedSportTypes ?? []
                      if (e.target.checked) setAllowedSportTypes([...current, d.sportType])
                      else {
                        const next = current.filter(s => s !== d.sportType)
                        setAllowedSportTypes(next.length > 0 ? next : null)
                      }
                    }}
                    className="rounded border-neutral-600 accent-accent-DEFAULT"
                  />
                  <span className="text-sm text-neutral-300">{d.label}</span>
                </label>
              ))}
            </div>
          </div>
        )}

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
              Public
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
              Private
            </button>
          </div>
        </div>

        <label className="flex items-start gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={newVersion}
            onChange={e => setNewVersion(e.target.checked)}
            className="mt-0.5 rounded border-neutral-600 accent-accent-DEFAULT"
          />
          <span className="text-sm text-neutral-300">Create new version</span>
        </label>

        <div className="flex gap-2 items-center justify-between pt-2">
          {segment.segment_type !== 'lap' && (
            <button
              type="button"
              onClick={handleReverse}
              disabled={mutation.isPending || publishMut.isPending || reverseMutation.isPending}
              className="px-3 py-1.5 text-sm border border-amber-800/40 rounded-lg text-amber-500 hover:bg-amber-900/20"
            >
              Reverse
            </button>
          )}
          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="px-3 py-1.5 text-sm border border-neutral-700 rounded-lg text-neutral-300 hover:bg-neutral-800"
            >
              Cancel
            </button>
            <button
              onClick={() => {
                if (initialSegment.visibility === 'private' && visibility === 'public') {
                  openModal({
                    title: 'Publish segment?',
                    message: 'Everyone will see this segment. A recompute will start.',
                    buttons: [
                      { label: 'Cancel', variant: 'default', onClick: () => {} },
                      { label: 'Publish', variant: 'destructive', onClick: () => publishMut.mutate() },
                    ],
                  })
                } else {
                  mutation.mutate()
                }
              }}
              disabled={!name.trim() || mutation.isPending || publishMut.isPending || reverseMutation.isPending}
              className="px-4 py-1.5 text-sm bg-accent-DEFAULT rounded-lg text-white hover:bg-accent-600 disabled:opacity-50"
            >
              Save
            </button>
          </div>
        </div>
    </ModalShell>
  )
}

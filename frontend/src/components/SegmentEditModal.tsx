import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchSegment, updateSegment, publishSegment, reverseSegment, type Segment, type UpdateSegmentPayload } from '@/api/segments'
import { getMe } from '@/api/auth'
import { ApiError } from '@/api/client'
import { ModalShell } from '@/components/ui/ModalShell'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'

// Shared Components
import { DifficultySelects } from './segment-editor/DifficultySelects'
import { SurfaceDropdown } from './segment-editor/SurfaceDropdown'
import { DifficultyColorPicker } from './segment-editor/DifficultyColorPicker'
import { ClimbTypePills } from './segment-editor/ClimbTypePills'
import { MapPreview } from './segment-editor/MapPreview'
import { SportCategoryChips } from './segment-editor/SportCategoryChips'
import { expandCategoryToSportTypes, isCategoryFullyActive, whitelistForSegmentType } from '@/lib/sportCategoryHelpers'
import { BIDIRECTIONAL_DEFAULT, type SegType } from './segment-editor/constants'
import { Pill } from './ui/Pill'
import { filterSegmentSportSelection, removeSegmentSportSelectionKeys } from './segment-editor/segmentSportSelection'
import { SPORT_DEFS } from '@/lib/sportTaxonomy'
import { buildExistingSegmentSportPatch } from './segment-editor/segmentSave'

const SEG_TYPE_LABELS: Record<SegType, string> = {
  standard: 'Standard',
  trail: 'Trail',
  lap: 'Runde',
  wintersport: 'Winter',
  wassersport: 'Wassersport',
  lift: 'Lift',
}

interface Props {
  segment: Segment
  onClose: () => void
}

export default function SegmentEditModal({ segment: initialSegment, onClose }: Props) {
  const qc = useQueryClient()
  const allSportTypes = useMemo(
    () => SPORT_DEFS.map(s => ({ key: s.sportType, label: s.label, category: s.categoryKey })),
    [],
  )
  
  // Vollständige Segment-Daten laden (inkl. GeoJSON für MapPreview)
  const { data: segment = initialSegment, isLoading: isLoadingFull } = useQuery({
    queryKey: ['segment', initialSegment.id, 'full'],
    queryFn: () => fetchSegment(initialSegment.id, { includeGeoJSON: true }),
    staleTime: 5 * 60 * 1000,
  })

  const { data: me } = useQuery({ queryKey: ['me'], queryFn: getMe, staleTime: 60_000 })

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
  const [allowedSportTypes, setAllowedSportTypes] = useState<string[] | null>(() => {
    if (initialSegment.allowed_predicates != null) {
      return initialSegment.allowed_predicates.map(p => p.sub_type ? `${p.sport}:${p.sub_type}` : p.sport)
    }
    return null
  })
  const [bidirectional, setBidirectional] = useState<boolean>(initialSegment.bidirectional ?? true)
  const [userEditedBidirectional, setUserEditedBidirectional] = useState(false)
  const initialSegType = (initialSegment.segment_type as SegType) ?? 'standard'
  const [segType, setSegType] = useState<SegType>(initialSegType)
  const [sportSelectionTouched, setSportSelectionTouched] = useState(false)

  function handleSegTypeChange(newType: SegType) {
    setSegType(newType)
    if (!userEditedBidirectional) {
      setBidirectional(BIDIRECTIONAL_DEFAULT[newType])
    }
    if (newType === 'wintersport' || newType === 'wassersport' || newType === 'lift') {
      setSacScale(null)
      setMtbScale(null)
      setSurface(null)
      setDiffColor(null)
    }
    const whitelist = whitelistForSegmentType(newType)
    if (whitelist) {
      setAllowedSportTypes(prev => {
        return filterSegmentSportSelection(prev, whitelist)
      })
    }
  }

  const { openModal } = useModal()
  const { addToast } = useToast()

  const mutation = useMutation({
    mutationFn: () => {
      const meta: Record<string, unknown> = { ...segment.segment_type_meta }
      if (segType === 'trail') {
        if (sacScale) meta.sac_scale = sacScale; else delete meta.sac_scale
        if (mtbScale) meta.mtb_scale = mtbScale; else delete meta.mtb_scale
        if (surface) meta.surface = surface; else delete meta.surface
      } else {
        delete meta.sac_scale
        delete meta.mtb_scale
        delete meta.surface
      }
      if (segType === 'trail' || segType === 'wintersport') {
        if (diffColor) meta.difficulty_color = diffColor; else delete meta.difficulty_color
      } else {
        delete meta.difficulty_color
      }

      const sportPatch = buildExistingSegmentSportPatch({
        segType,
        initialSegType,
        selection: allowedSportTypes,
        initialAllowedPredicates: initialSegment.allowed_predicates,
        touched: sportSelectionTouched,
      })
      const payload: UpdateSegmentPayload = {
        name: name.trim(),
        description: description.trim() || undefined,
        segment_type: segType,
        segment_type_meta: Object.keys(meta).length > 0 ? meta : undefined,
        new_version: newVersion || undefined,
        climb_type: segType === 'lap' ? 'standard' : climbType,
        bidirectional,
        ...sportPatch,
      }
      if (initialSegment.visibility === 'public' && visibility === 'private') {
        payload.visibility = 'private'
      }
      return updateSegment(initialSegment.id, payload)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['segments'] })
      qc.invalidateQueries({ queryKey: ['segment', initialSegment.id] })
      addToast({ type: 'success', title: 'Segment aktualisiert' })
      onClose()
    },
  })

  const publishMut = useMutation({
    mutationFn: () => publishSegment(initialSegment.id),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Segment veröffentlicht', message: 'Berechnung gestartet.' })
      qc.invalidateQueries({ queryKey: ['segments'] })
      qc.invalidateQueries({ queryKey: ['segment', initialSegment.id] })
      onClose()
    },
    onError: () => {
      addToast({ type: 'error', title: 'Veröffentlichung fehlgeschlagen', duration: 0 })
    },
  })

  const reverseMutation = useMutation({
    mutationFn: () => reverseSegment(initialSegment.id),
    onSuccess: (resp) => {
      addToast({
        type: 'success',
        title: 'Segment umgekehrt',
        message: resp.efforts_updated === 1 ? '1 Effort aktualisiert' : `${resp.efforts_updated} Efforts aktualisiert`,
      })
      qc.invalidateQueries({ queryKey: ['segment', initialSegment.id] })
      qc.invalidateQueries({ queryKey: ['segments'] })
      onClose()
    },
    onError: () => {
      addToast({ type: 'error', title: 'Umkehren fehlgeschlagen', duration: 0 })
    },
  })

  function handleReverse() {
    const effortMsg = segment.effort_count > 0
      ? `${segment.effort_count} öffentliche Efforts + eigene private Efforts werden mit invertiertem is_reverse-Flag aktualisiert und die Segment-Ranks neu berechnet. Diese Aktion kann nicht rückgängig gemacht werden.`
      : 'Keine öffentlichen Efforts betroffen. Richtung des Segments umkehren?'
    openModal({
      title: 'Richtung umkehren?',
      message: effortMsg,
      buttons: [
        { label: 'Abbrechen', variant: 'default', onClick: () => {} },
        { label: 'Umkehren', variant: 'destructive', onClick: () => reverseMutation.mutate() },
      ],
    })
  }

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md" className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800 space-y-4">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Segment bearbeiten</h2>

        {/* Map Preview */}
        <MapPreview 
          geojsonTrack={segment.track_geojson || ''} 
          height="160px" 
          className="mb-2 rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-800"
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
          placeholder="Beschreibung (optional)"
          rows={2}
          className={`${AUTH_INPUT_CLASS} resize-none`}
        />

        {/* Segment-Typ Picker */}
        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">Segment-Typ</label>
          <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label="Segment-Typ">
            {(['standard', 'trail', 'lap', 'wintersport', 'wassersport', 'lift'] as SegType[]).map(t => (
              <Pill
                key={t}
                active={segType === t}
                onClick={() => handleSegTypeChange(t)}
                role="radio"
                ariaChecked={segType === t}
                disabled={isLoadingFull}
              >
                {SEG_TYPE_LABELS[t]}
              </Pill>
            ))}
          </div>
        </div>

        <div className="space-y-4">
          <DifficultySelects
            segType={segType}
            sacScale={sacScale}
            mtbScale={mtbScale}
            onChange={(f, v) => f === 'sac' ? setSacScale(v) : setMtbScale(v)}
            disabled={isLoadingFull}
          />

          <SurfaceDropdown
            segType={segType}
            value={surface}
            onChange={setSurface}
            disabled={isLoadingFull}
          />

          <DifficultyColorPicker
            segType={segType}
            value={diffColor}
            onChange={setDiffColor}
            disabled={isLoadingFull}
          />
        </div>

        <div className="space-y-3">
          {(segType === 'standard' || segType === 'trail') && (
            <div className="space-y-1.5">
              <label className="text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">Höhenprofil</label>
              <ClimbTypePills
                segType={segType}
                value={climbType}
                onChange={setClimbType}
                disabled={isLoadingFull}
              />
            </div>
          )}

          {/* Bidirectional toggle — only shown for non-lap segments (lap = closed loop) */}
          {segType !== 'lap' && (
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={bidirectional}
                onChange={e => {
                  const newVal = e.target.checked
                  setBidirectional(newVal)
                  setUserEditedBidirectional(newVal !== BIDIRECTIONAL_DEFAULT[segType])
                }}
                className="rounded border-neutral-400 dark:border-neutral-600 accent-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-700 dark:text-neutral-300">Bidirektionales Matching</span>
            </label>
          )}
        </div>

        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">Erlaubte Sportarten</label>
          <SportCategoryChips
            segType={segType}
            activeTypes={allowedSportTypes}
            onToggleCategory={(catKey) => {
              setSportSelectionTouched(true)
              const whitelist = whitelistForSegmentType(segType)
              const types = expandCategoryToSportTypes(catKey, whitelist)
              if (types.length === 0) return

              const isFull = isCategoryFullyActive(catKey, allowedSportTypes, whitelist)
              setAllowedSportTypes(prev => {
                const current = prev ?? []
                if (isFull) {
                  return removeSegmentSportSelectionKeys(current, types)
                } else {
                  const next = [...current]
                  types.forEach(t => { if (!next.includes(t)) next.push(t) })
                  return next
                }
              })
            }}
            onSetAll={() => {
              setSportSelectionTouched(true)
              setAllowedSportTypes(null)
            }}
            availableSportTypes={allSportTypes}
            showDetailPicker
            onToggleSportType={(sportType) => {
              setSportSelectionTouched(true)
              setAllowedSportTypes(prev => {
                if (prev === null) return [sportType]
                const next = prev.includes(sportType)
                  ? prev.filter(t => t !== sportType)
                  : [...prev, sportType]
                return next
              })
            }}
          />
        </div>

        {/* Visibility toggle — not for lift segments */}
        {segType !== 'lift' && (
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setVisibility('public')}
                className={`flex-1 py-1.5 text-sm rounded-lg border cursor-pointer transition-colors ${
                  visibility === 'public'
                    ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                    : 'border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                Öffentlich
              </button>
              <button
                type="button"
                onClick={() => setVisibility('private')}
                className={`flex-1 py-1.5 text-sm rounded-lg border cursor-pointer transition-colors ${
                  visibility === 'private'
                    ? 'bg-neutral-200 dark:bg-neutral-700 text-neutral-900 dark:text-neutral-100 border-neutral-300 dark:border-neutral-600'
                    : 'border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                Privat
              </button>
            </div>
            {initialSegment.visibility === 'private' && visibility === 'public' && (
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
            className="mt-0.5 rounded border-neutral-400 dark:border-neutral-600 accent-accent-DEFAULT"
          />
          <span className="text-sm text-neutral-700 dark:text-neutral-300">
            Neue Version erstellen
            <span className="block text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              Nur aktivieren wenn sich die Route grundlegend ändert.
              Bestehende Bestzeiten bleiben der alten Version zugeordnet.
            </span>
          </span>
        </label>

        {mutation.isError && (
          <p className="text-sm text-red-600 dark:text-red-400">
            {(() => {
              const err = mutation.error
              if (err instanceof ApiError) {
                if (err.status === 403) return 'Keine Berechtigung: Du bist nicht der Besitzer dieses Segments.'
                if (err.status === 404) return 'Segment nicht gefunden.'
                const detail = (err.json?.detail as string | undefined) ?? (err.json?.message as string | undefined)
                if (detail) return `Fehler beim Speichern: ${detail}`
              }
              return 'Fehler beim Speichern.'
            })()}
          </p>
        )}

        <div className="flex gap-2 items-center justify-between pt-2">
          {segType !== 'lap' && (
            <button
              type="button"
              onClick={handleReverse}
              disabled={mutation.isPending || publishMut.isPending || reverseMutation.isPending}
              className="cursor-pointer rounded-lg border border-amber-300 dark:border-amber-800/60 bg-amber-50 dark:bg-amber-950/30 px-3 py-1.5 text-sm text-amber-700 dark:text-amber-400 hover:bg-amber-100 dark:hover:bg-amber-900/40 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              ⇄ Richtung umkehren
            </button>
          )}
          <div className="flex gap-2">
            <button
              onClick={onClose}
              disabled={reverseMutation.isPending}
              className="cursor-pointer px-3 py-1.5 text-sm border border-neutral-300 dark:border-neutral-700 rounded-lg text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Abbrechen
            </button>
            <button
              onClick={() => {
                if (initialSegment.visibility === 'private' && visibility === 'public') {
                  const isForeignAdmin = me?.is_admin && initialSegment.created_by !== me?.user_id
                  openModal({
                    title: 'Segment veröffentlichen?',
                    message: isForeignAdmin
                      ? 'Admin-Aktion: Du veröffentlichst ein fremdes Segment. Alle Nutzer können es danach sehen, alle Aktivitäten werden rückwirkend gematcht. Der Ersteller wird nicht benachrichtigt.'
                      : 'Alle Nutzer können das Segment danach sehen. Eine Neuberechnung wird gestartet.',
                    buttons: [
                      { label: 'Abbrechen', variant: 'default', onClick: () => {} },
                      { label: 'Veröffentlichen', variant: 'destructive', onClick: () => publishMut.mutate() },
                    ],
                  })
                } else {
                  mutation.mutate()
                }
              }}
              disabled={!name.trim() || mutation.isPending || publishMut.isPending || reverseMutation.isPending}
              className="cursor-pointer px-4 py-1.5 text-sm bg-accent-DEFAULT rounded-lg text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {mutation.isPending || publishMut.isPending ? 'Speichern...' : 'Speichern'}
            </button>
          </div>
        </div>
    </ModalShell>
  )
}

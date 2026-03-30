import { useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Lock, Map, Pencil, RefreshCw, Trash2 } from 'lucide-react'
import { getMe } from '@/api/auth'
import { fetchSegment, recomputeSegment, publishSegment, deleteSegment, exportSegment } from '@/api/segments'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'
import { useActiveJobs } from '@/lib/useActiveJobs'
import { fmtDist } from '@/lib/format'
import { triggerBlobDownload } from '@/lib/utils'
import { SegmentMinimap } from '@/components/SegmentDetail/SegmentMinimap'
import { SegmentActivityList } from '@/components/SegmentDetail/SegmentActivityList'
import { SegmentPerformanceChart } from '@/components/SegmentDetail/SegmentPerformanceChart'
import { FavoriteButton } from '@/components/SegmentDetail/FavoriteButton'
import SegmentEditModal from '@/components/SegmentEditModal'
import DifficultyBadge from '@/components/DifficultyBadge'
import ActivityFilters, { EMPTY_FILTERS, type ActivityFilterState } from '@/components/ActivityFilters'
import DateRangePicker from '@/components/DateRangePicker'

const SPORT_GROUP_LABELS: Record<string, string> = {
  bike: 'Radfahren', foot: 'Laufsport', swim: 'Schwimmen',
  ski: 'Wintersport', paddle: 'Wassersport', generic: 'Sonstiges',
}
const TYPE_LABELS: Record<string, string> = {
  standard: 'Standard', trail: 'Trail', lift: 'Lift', lap: 'Rundkurs',
}


export default function SegmentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [showEdit, setShowEdit] = useState(false)
  const [localPending, setLocalPending] = useState(false)
  const [filters, setFilters] = useState<ActivityFilterState>(EMPTY_FILTERS)

  const queryClient = useQueryClient()
  const { isJobActiveForEntity } = useActiveJobs()
  const { openModal } = useModal()
  const { addToast } = useToast()

  const { data: me } = useQuery({
    queryKey: ['me'],
    queryFn: getMe,
    staleTime: 60_000,
  })

  const publishMut = useMutation({
    mutationFn: () => publishSegment(id!),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Segment veröffentlicht', message: 'Recompute gestartet.' })
      queryClient.invalidateQueries({ queryKey: ['segment', id] })
      queryClient.invalidateQueries({ queryKey: ['segments'] })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Veröffentlichung fehlgeschlagen', duration: 0 })
    },
  })

  const deleteMut = useMutation({
    mutationFn: () => deleteSegment(id!),
    onSuccess: () => navigate('/segments'),
    onError: () => addToast({ type: 'error', title: 'Löschen fehlgeschlagen', duration: 0 }),
  })

  const recomputeMut = useMutation({
    mutationFn: () => recomputeSegment(id!),
    onMutate: () => {
      setLocalPending(true)
      queryClient.invalidateQueries({ queryKey: ['active-jobs'] })
    },
    onSuccess: () => {
      setTimeout(() => {
        queryClient.invalidateQueries({ queryKey: ['segment', id] })
        queryClient.invalidateQueries({ queryKey: ['segment-performances', id] })
      }, 3000)
      setTimeout(() => setLocalPending(false), 10000)
    },
    onError: () => {
      setLocalPending(false)
    },
  })

  const isRecomputing = recomputeMut.isPending || localPending || isJobActiveForEntity(id!)

  const handleExport = async (format: 'gpx' | 'geojson') => {
    try {
      const ext = format === 'gpx' ? '.gpx' : '.geojson'
      const { blob, filename } = await exportSegment(id!, format)
      triggerBlobDownload(blob, filename ?? `segment${ext}`)
    } catch (err) {
      console.error('Segment-Export fehlgeschlagen', err)
      addToast({ type: 'error', title: 'Export fehlgeschlagen', message: 'Der Export konnte nicht erstellt werden.', duration: 0 })
    }
  }

  const { data: segment, isLoading } = useQuery({
    queryKey: ['segment', id],
    queryFn: () => fetchSegment(id!),
    enabled: !!id,
  })

  if (isLoading) return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Segment...</div>
  if (!segment) return <div className="p-6 text-neutral-400">Segment nicht gefunden</div>

  return (
    <div className="flex flex-col gap-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <Link to="/segments" className="flex items-center gap-2 text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300">
          <ArrowLeft size={16} /> Segmente
        </Link>
        <div className="flex items-center gap-2">
          <DateRangePicker
            dateFrom={filters.dateFrom}
            dateTo={filters.dateTo}
            onChange={(from, to) => setFilters(f => ({ ...f, dateFrom: from, dateTo: to }))}
          />
          <ActivityFilters filters={filters} onChange={setFilters} />
          {segment.segment_type !== 'lift' && (
          <button
            onClick={() => setShowEdit(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
          >
            <Pencil size={14} /> Bearbeiten
          </button>
          )}
          <button
            onClick={() => { if (!isRecomputing) recomputeMut.mutate() }}
            disabled={isRecomputing}
            className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
              isRecomputing
                ? 'border-accent-DEFAULT text-accent-DEFAULT bg-accent-100 dark:bg-accent-100/20'
                : 'border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
            }`}
            title="Matching neu berechnen"
          >
            <RefreshCw size={14} className={isRecomputing ? 'animate-spin' : ''} /> {isRecomputing ? 'Läuft…' : 'Neu berechnen'}
          </button>
          <button
            onClick={() => handleExport('gpx')}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
          >
            GPX exportieren
          </button>
          <button
            onClick={() => handleExport('geojson')}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
          >
            GeoJSON exportieren
          </button>
          <button
            onClick={() => navigate('/map', { state: { activateSegmentsOverlay: true } })}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
          >
            <Map size={14} /> Auf Karte
          </button>
          {/* Publish: shown for creator of private segment */}
          {segment.visibility === 'private' && segment.created_by === me?.user_id && (
            <button
              onClick={() => openModal({
                title: 'Segment veröffentlichen?',
                message: 'Alle Nutzer können das Segment danach sehen. Ein Recompute-Job wird gestartet.',
                buttons: [
                  { label: 'Abbrechen', variant: 'default', onClick: () => {} },
                  { label: 'Veröffentlichen', variant: 'destructive', onClick: () => publishMut.mutate() },
                ],
              })}
              disabled={publishMut.isPending}
              className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-amber-400 text-amber-600 dark:text-amber-400 hover:bg-amber-50 dark:hover:bg-amber-900/20 cursor-pointer transition-colors disabled:opacity-50"
            >
              Veröffentlichen
            </button>
          )}

          {/* Delete: visible for creator or admin, not for OSM lift segments */}
          {segment.segment_type !== 'lift' && (me?.user_id === segment.created_by || me?.is_admin) && (
            <button
              onClick={() => openModal({
                title: 'Segment löschen?',
                message: 'Löschen ist nicht rückgängig zu machen.',
                buttons: [
                  { label: 'Abbrechen', variant: 'default', onClick: () => {} },
                  { label: 'Löschen', variant: 'destructive', onClick: () => deleteMut.mutate() },
                ],
              })}
              className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 cursor-pointer transition-colors"
            >
              <Trash2 className="w-3.5 h-3.5" />
              Löschen
            </button>
          )}
          <FavoriteButton segmentId={id!} />
        </div>
      </div>

      {/* Title + Metadata */}
      <div>
        <div className="flex items-center gap-3 mb-2">
          <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{segment.name}</h1>
          {segment.segment_type === 'trail' && <DifficultyBadge meta={segment.segment_type_meta ?? {}} />}
        </div>
        <div className="flex flex-wrap gap-3 text-sm text-neutral-500 dark:text-neutral-400">
          <span className="px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-neutral-800">
            {SPORT_GROUP_LABELS[segment.sport_group] ?? segment.sport_group}
          </span>
          {segment.segment_type !== 'standard' && (
            <span className="px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-neutral-800">
              {TYPE_LABELS[segment.segment_type] ?? segment.segment_type}
            </span>
          )}
          {segment.distance_m != null && <span>{fmtDist(segment.distance_m)}</span>}
          {segment.elevation_gain_m != null && segment.elevation_gain_m > 0 && (
            <span>+{Math.round(segment.elevation_gain_m)} m</span>
          )}
          {segment.climb_type && segment.climb_type !== 'standard' && (
            <span className="capitalize">{segment.climb_type}</span>
          )}
        </div>
        {/* Effort count stats */}
        <div className="flex flex-wrap gap-x-4 gap-y-1 mt-2 text-xs text-neutral-500 dark:text-neutral-400">
          <span>
            {segment.effort_count} {segment.effort_count === 1 ? 'Effort' : 'Efforts'} gesamt
          </span>
          {segment.ridden_count > 0 && (
            <span>· {segment.ridden_count} {segment.ridden_count === 1 ? 'eigener' : 'eigene'}</span>
          )}
          {segment.athlete_count > 0 && (
            <span>· {segment.athlete_count} {segment.athlete_count === 1 ? 'Athlet' : 'Athleten'}</span>
          )}
          {segment.visibility === 'private' && (
            <span className="flex items-center gap-1 text-neutral-400">
              <Lock className="w-3 h-3" />
              Privat
            </span>
          )}
        </div>
        {segment.description && (
          <p className="mt-2 text-sm text-neutral-600 dark:text-neutral-400">{segment.description}</p>
        )}
      </div>

      {/* Minimap + Activity List */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <SegmentMinimap segment={segment} />
        <SegmentActivityList segmentId={id!} segment={segment} filters={filters} />
      </div>

      {/* Performance Chart */}
      <SegmentPerformanceChart segmentId={id!} dateFrom={filters.dateFrom} dateTo={filters.dateTo} />

      {/* Edit Modal */}
      {showEdit && (
        <SegmentEditModal
          segment={segment}
          onClose={() => {
            setShowEdit(false)
            queryClient.invalidateQueries({ queryKey: ['segment', id] })
          }}
        />
      )}
    </div>
  )
}

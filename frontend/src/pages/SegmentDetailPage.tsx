import { useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Map, RefreshCw, Pencil } from 'lucide-react'
import { fetchSegment, recomputeSegment } from '@/api/segments'
import { useActiveJobs } from '@/lib/useActiveJobs'
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

function fmtDist(m?: number): string {
  if (m == null || isNaN(m)) return '--'
  return m >= 1000 ? `${(m / 1000).toFixed(2)} km` : `${Math.round(m)} m`
}

export default function SegmentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [showEdit, setShowEdit] = useState(false)
  const [localPending, setLocalPending] = useState(false)
  const [filters, setFilters] = useState<ActivityFilterState>(EMPTY_FILTERS)

  const queryClient = useQueryClient()
  const { isJobActiveForEntity } = useActiveJobs()

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
          <button
            onClick={() => setShowEdit(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
          >
            <Pencil size={14} /> Bearbeiten
          </button>
          <button
            onClick={() => { if (!isRecomputing) recomputeMut.mutate() }}
            disabled={isRecomputing}
            className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border transition-colors disabled:opacity-50 ${
              isRecomputing
                ? 'border-accent-DEFAULT text-accent-DEFAULT bg-orange-50 dark:bg-orange-900/20'
                : 'border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
            }`}
            title="Matching neu berechnen"
          >
            <RefreshCw size={14} className={isRecomputing ? 'animate-spin' : ''} /> {isRecomputing ? 'Läuft…' : 'Neu berechnen'}
          </button>
          <button
            onClick={() => navigate('/map', { state: { activateSegmentsOverlay: true } })}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
          >
            <Map size={14} /> Auf Karte
          </button>
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

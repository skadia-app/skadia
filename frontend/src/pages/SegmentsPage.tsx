import { useState, useMemo, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchSegments, fetchSegmentLeaderboard, deleteSegment, type Segment, type LeaderboardEntry } from '@/api/segments'
import SegmentCreationPanel from '@/components/SegmentCreationPanel'

const SPORT_GROUP_LABELS: Record<string, string> = {
  bike: 'Rad',
  foot: 'Zu Fuss',
  swim: 'Schwimmen',
  ski: 'Ski',
  paddle: 'Paddeln',
  generic: 'Allgemein',
}

const SPORT_GROUP_ICONS: Record<string, string> = {
  bike: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
  foot: 'M13.5 5.5c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2z',
  swim: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
  ski: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
  paddle: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
  generic: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
}

const SPORT_GROUP_COLORS: Record<string, string> = {
  bike: 'text-orange-500',
  foot: 'text-green-500',
  swim: 'text-cyan-500',
  ski: 'text-blue-500',
  paddle: 'text-blue-400',
  generic: 'text-neutral-500',
}

const SEGMENT_TYPE_LABELS: Record<string, string> = {
  standard: 'Standard',
  trail: 'Trail',
  lift: 'Lift',
}

function fmtDuration(s: number): string {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = Math.floor(s % 60)
  if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${sec.toString().padStart(2, '0')}`
  return `${m}:${sec.toString().padStart(2, '0')}`
}

function fmtDist(m?: number): string {
  if (m == null || isNaN(m)) return '--'
  if (m >= 1000) return `${(m / 1000).toFixed(2)} km`
  return `${Math.round(m)} m`
}

export default function SegmentsPage() {
  const queryClient = useQueryClient()
  const [sportGroup, setSportGroup] = useState('')
  const [segmentType, setSegmentType] = useState('')
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [selectedSegmentId, setSelectedSegmentId] = useState<string | null>(null)
  const [showCreatePanel, setShowCreatePanel] = useState(false)

  // Debounced search
  const [searchTimer, setSearchTimer] = useState<ReturnType<typeof setTimeout> | null>(null)
  const handleSearchChange = useCallback((value: string) => {
    setSearchInput(value)
    if (searchTimer) clearTimeout(searchTimer)
    const timer = setTimeout(() => setSearch(value), 300)
    setSearchTimer(timer)
  }, [searchTimer])

  const filterParams = useMemo(() => ({
    sport_group: sportGroup || undefined,
    segment_type: segmentType || undefined,
    search: search || undefined,
  }), [sportGroup, segmentType, search])

  const { data: segments = [], isLoading } = useQuery({
    queryKey: ['segments', filterParams],
    queryFn: () => fetchSegments(filterParams),
  })

  const deleteMut = useMutation({
    mutationFn: deleteSegment,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['segments'] })
    },
  })

  const handleDelete = (e: React.MouseEvent, id: string) => {
    e.stopPropagation()
    if (window.confirm('Segment wirklich loeschen?')) {
      deleteMut.mutate(id)
    }
  }

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Segmente...</div>
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Segmente</h1>
        <button
          onClick={() => setShowCreatePanel(!showCreatePanel)}
          className="px-4 py-2 text-sm font-medium rounded-lg bg-orange-500 text-white hover:bg-orange-600 transition-colors"
        >
          {showCreatePanel ? 'Abbrechen' : 'Neues Segment'}
        </button>
      </div>

      {/* Create Panel */}
      {showCreatePanel && (
        <div className="mb-6">
          <SegmentCreationPanel onSuccess={() => {
            setShowCreatePanel(false)
            queryClient.invalidateQueries({ queryKey: ['segments'] })
          }} />
        </div>
      )}

      {/* Filter Bar */}
      <div className="flex flex-wrap items-center gap-3 mb-6">
        <select
          value={sportGroup}
          onChange={e => setSportGroup(e.target.value)}
          className="border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm
                     bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                     focus:outline-none focus:ring-1 focus:ring-neutral-400"
        >
          <option value="">Alle Sportarten</option>
          {Object.entries(SPORT_GROUP_LABELS).map(([key, label]) => (
            <option key={key} value={key}>{label}</option>
          ))}
        </select>

        <select
          value={segmentType}
          onChange={e => setSegmentType(e.target.value)}
          className="border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm
                     bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                     focus:outline-none focus:ring-1 focus:ring-neutral-400"
        >
          <option value="">Alle Typen</option>
          {Object.entries(SEGMENT_TYPE_LABELS).map(([key, label]) => (
            <option key={key} value={key}>{label}</option>
          ))}
        </select>

        <div className="relative">
          <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-neutral-400 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35M11 19a8 8 0 100-16 8 8 0 000 16z" />
          </svg>
          <input
            type="text"
            value={searchInput}
            onChange={e => handleSearchChange(e.target.value)}
            placeholder="Segment suchen..."
            className="border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 pl-8 text-sm w-56
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
        </div>
      </div>

      {/* Segment Cards */}
      {segments.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-48 text-neutral-400 dark:text-neutral-500">
          <svg className="w-12 h-12 mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l5.447 2.724A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
          </svg>
          <p className="text-sm">Noch keine Segmente vorhanden</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {segments.map(segment => (
            <div
              key={segment.id}
              onClick={() => setSelectedSegmentId(segment.id)}
              className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4
                         hover:border-neutral-300 dark:hover:border-neutral-700
                         hover:shadow-sm transition-all cursor-pointer group"
            >
              <div className="flex items-start justify-between mb-2">
                <div className="flex items-center gap-2">
                  <svg className={`w-5 h-5 ${SPORT_GROUP_COLORS[segment.sport_group] ?? 'text-neutral-500'}`} viewBox="0 0 24 24" fill="currentColor">
                    <path d={SPORT_GROUP_ICONS[segment.sport_group] ?? SPORT_GROUP_ICONS.generic} />
                  </svg>
                  <h3 className="font-medium text-neutral-900 dark:text-neutral-100 group-hover:text-orange-500 transition-colors">
                    {segment.name}
                  </h3>
                </div>
                <button
                  onClick={(e) => handleDelete(e, segment.id)}
                  className="p-1 rounded text-neutral-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors opacity-0 group-hover:opacity-100"
                  title="Segment loeschen"
                >
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                </button>
              </div>

              <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-neutral-500 dark:text-neutral-400">
                {segment.distance_m != null && (
                  <span>{fmtDist(segment.distance_m)}</span>
                )}
                {segment.elevation_gain_m != null && segment.elevation_gain_m > 0 && (
                  <span>+{Math.round(segment.elevation_gain_m)} m</span>
                )}
                <span>{segment.effort_count} {segment.effort_count === 1 ? 'Effort' : 'Efforts'}</span>
              </div>

              <div className="flex items-center gap-2 mt-2">
                <span className="inline-block px-2 py-0.5 text-xs rounded-full bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400">
                  {SPORT_GROUP_LABELS[segment.sport_group] ?? segment.sport_group}
                </span>
                {segment.segment_type !== 'standard' && (
                  <span className="inline-block px-2 py-0.5 text-xs rounded-full bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400">
                    {SEGMENT_TYPE_LABELS[segment.segment_type] ?? segment.segment_type}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Leaderboard Modal */}
      {selectedSegmentId && (
        <LeaderboardModal
          segmentId={selectedSegmentId}
          segmentName={segments.find(s => s.id === selectedSegmentId)?.name ?? 'Segment'}
          onClose={() => setSelectedSegmentId(null)}
        />
      )}
    </div>
  )
}

function LeaderboardModal({ segmentId, segmentName, onClose }: { segmentId: string; segmentName: string; onClose: () => void }) {
  const { data: entries = [], isLoading } = useQuery({
    queryKey: ['segment-leaderboard', segmentId],
    queryFn: () => fetchSegmentLeaderboard(segmentId),
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="bg-white dark:bg-neutral-900 rounded-xl shadow-2xl max-w-2xl w-full mx-4 max-h-[80vh] flex flex-col"
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-4 border-b border-neutral-200 dark:border-neutral-800">
          <div>
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Bestenliste</h2>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">{segmentName}</p>
          </div>
          <button
            onClick={onClose}
            className="p-2 rounded-lg text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <div className="overflow-y-auto flex-1 p-4">
          {isLoading ? (
            <div className="flex items-center justify-center h-32 text-neutral-400">Lade Bestenliste...</div>
          ) : entries.length === 0 ? (
            <div className="flex items-center justify-center h-32 text-neutral-400">Noch keine Eintraege</div>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-200 dark:border-neutral-700">
                  <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Rang</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Athlet</th>
                  <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Zeit</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Datum</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Ausruestung</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry, i) => (
                  <tr
                    key={`${entry.activity_id}-${i}`}
                    className={`border-b border-neutral-100 dark:border-neutral-800 ${i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''}`}
                  >
                    <td className="px-3 py-2 text-neutral-900 dark:text-neutral-100">
                      {entry.rank === 1 ? (
                        <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 font-bold text-xs">
                          1
                        </span>
                      ) : (
                        <span className="text-neutral-600 dark:text-neutral-400">#{entry.rank}</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-neutral-900 dark:text-neutral-100">{entry.athlete_name}</td>
                    <td className="px-3 py-2 text-right tabular-nums text-neutral-900 dark:text-neutral-100">
                      {fmtDuration(entry.elapsed_time_s)}
                    </td>
                    <td className="px-3 py-2 text-neutral-500 dark:text-neutral-400">
                      {new Date(entry.started_at).toLocaleDateString('de-DE')}
                    </td>
                    <td className="px-3 py-2 text-neutral-500 dark:text-neutral-400">
                      {entry.gear_name ?? '--'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

import { useState, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import ReactECharts from 'echarts-for-react'
import { fetchActivity, fetchActivityStreams } from '@/api/activities'
import { apiClient } from '@/api/client'
import type { StreamPoint } from '@/api/types'

// ── Types ─────────────────────────────────────────────────────────────────────

interface Edit {
  type: string
  timestamp?: string
  from_utc?: string
  to_utc?: string
  timestamps?: string[]
  method?: string
}

interface EditSidecar {
  version: number
  activity_id: string
  created_at: string
  updated_at: string
  edits: Edit[]
}

interface DetectSpikesResponse {
  count: number
  timestamps: string[]
  threshold: number
}

// ── API calls ─────────────────────────────────────────────────────────────────

function getEdits(activityId: string): Promise<EditSidecar> {
  return apiClient<EditSidecar>(`/activities/${activityId}/edits`)
}

function putEdits(activityId: string, sidecar: EditSidecar): Promise<EditSidecar> {
  return apiClient<EditSidecar>(`/activities/${activityId}/edits`, {
    method: 'PUT',
    body: sidecar,
  })
}

function deleteEdits(activityId: string): Promise<void> {
  return apiClient<void>(`/activities/${activityId}/edits`, { method: 'DELETE' })
}

function detectSpikes(activityId: string, threshold = 25): Promise<DetectSpikesResponse> {
  return apiClient<DetectSpikesResponse>(`/activities/${activityId}/edits/detect-hr-spikes`, {
    method: 'POST',
    body: { threshold },
  })
}

function applyEdits(activityId: string): Promise<unknown> {
  return apiClient(`/activities/${activityId}/edits/apply`, { method: 'POST' })
}

// ── HR Chart ──────────────────────────────────────────────────────────────────

function buildHRChartOption(
  streams: StreamPoint[],
  spikeTimestamps: Set<string>,
  showPreview: boolean,
  sidecar: EditSidecar | null,
) {
  const times: string[] = []
  const hrValues: (number | null)[] = []
  const spikeMarkers: { coord: [string, number]; itemStyle: { color: string } }[] = []

  const smoothedMap = new Map<string, number | null>()
  if (showPreview && sidecar) {
    // Build smoothed values for spikes found in edits
    const hrSmoothEdits = sidecar.edits.filter(e => e.type === 'hr_spike_smooth')
    for (const edit of hrSmoothEdits) {
      if (!edit.timestamps) continue
      for (const ts of edit.timestamps) {
        smoothedMap.set(ts, null) // will be interpolated below
      }
    }
  }

  for (let i = 0; i < streams.length; i++) {
    const s = streams[i]
    if (s.heart_rate == null) continue

    const timeStr = new Date(s.time).toISOString()
    times.push(timeStr)

    if (showPreview && smoothedMap.has(timeStr)) {
      // Find neighbors for interpolation
      let prevHR: number | null = null
      let nextHR: number | null = null
      for (let j = i - 1; j >= 0; j--) {
        if (streams[j].heart_rate != null) { prevHR = streams[j].heart_rate!; break }
      }
      for (let j = i + 1; j < streams.length; j++) {
        if (streams[j].heart_rate != null) { nextHR = streams[j].heart_rate!; break }
      }
      const interpolated = prevHR != null && nextHR != null
        ? Math.round((prevHR + nextHR) / 2)
        : (prevHR ?? nextHR)
      hrValues.push(interpolated)
    } else {
      hrValues.push(s.heart_rate)
    }

    if (spikeTimestamps.has(timeStr)) {
      spikeMarkers.push({
        coord: [timeStr, s.heart_rate],
        itemStyle: { color: '#f97316' },
      })
    }
  }

  return {
    animation: false,
    grid: { left: 50, right: 20, top: 20, bottom: 60 },
    xAxis: {
      type: 'time',
      axisLabel: {
        formatter: (val: number) => {
          const d = new Date(val)
          return `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
        },
        color: '#9ca3af',
        fontSize: 11,
      },
      axisLine: { lineStyle: { color: '#374151' } },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      name: 'HR (bpm)',
      nameTextStyle: { color: '#9ca3af', fontSize: 11 },
      axisLabel: { color: '#9ca3af', fontSize: 11 },
      axisLine: { lineStyle: { color: '#374151' } },
      splitLine: { lineStyle: { color: '#1f2937', type: 'dashed' } },
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: '#111827',
      borderColor: '#374151',
      textStyle: { color: '#f9fafb', fontSize: 12 },
      formatter: (params: { axisValue: string; value: number }[]) => {
        if (!params?.length) return ''
        const t = new Date(params[0].axisValue)
        const time = `${t.getHours().toString().padStart(2, '0')}:${t.getMinutes().toString().padStart(2, '0')}:${t.getSeconds().toString().padStart(2, '0')}`
        return `${time}<br/>HR: <b>${params[0].value} bpm</b>`
      },
    },
    series: [
      {
        type: 'line',
        data: times.map((t, i) => [t, hrValues[i]]),
        lineStyle: { color: showPreview ? '#22c55e' : '#ef4444', width: 1.5 },
        symbol: 'none',
        smooth: false,
        markPoint: {
          symbol: 'circle',
          symbolSize: 8,
          data: spikeMarkers,
        },
      },
    ],
  }
}

// ── Main Page ──────────────────────────────────────────────────────────────────

export default function ActivityEditorPage() {
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()

  const [detectedSpikes, setDetectedSpikes] = useState<string[]>([])
  const [showPreview, setShowPreview] = useState(false)
  const [statusMsg, setStatusMsg] = useState<string | null>(null)
  const [isDetecting, setIsDetecting] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [isApplying, setIsApplying] = useState(false)
  const [isResetting, setIsResetting] = useState(false)

  const { data: activity, isLoading: activityLoading } = useQuery({
    queryKey: ['activity', id],
    queryFn: () => fetchActivity(id!),
    enabled: !!id,
  })

  const { data: streams = [], isLoading: streamsLoading } = useQuery({
    queryKey: ['activity-streams', id],
    queryFn: () => fetchActivityStreams(id!),
    enabled: !!id,
  })

  const { data: sidecar, refetch: refetchSidecar } = useQuery({
    queryKey: ['activity-edits', id],
    queryFn: () => getEdits(id!),
    enabled: !!id,
  })

  // Build spike set from sidecar (for chart markers if already saved)
  const savedSpikeTimestamps = new Set<string>(
    sidecar?.edits
      .filter(e => e.type === 'hr_spike_smooth')
      .flatMap(e => e.timestamps ?? []) ?? []
  )
  const displaySpikeTimestamps = detectedSpikes.length > 0
    ? new Set(detectedSpikes)
    : savedSpikeTimestamps

  const hasHR = streams.some(s => s.heart_rate != null)

  const showStatus = useCallback((msg: string, duration = 3000) => {
    setStatusMsg(msg)
    setTimeout(() => setStatusMsg(null), duration)
  }, [])

  // Detect spikes
  async function handleDetectSpikes() {
    if (!id) return
    setIsDetecting(true)
    try {
      const result = await detectSpikes(id, 25)
      setDetectedSpikes(result.timestamps)
      showStatus(`${result.count} HR-Spike${result.count !== 1 ? 's' : ''} erkannt (Schwelle: ${result.threshold} bpm)`)
    } catch {
      showStatus('Spike-Erkennung fehlgeschlagen.')
    } finally {
      setIsDetecting(false)
    }
  }

  // Save spikes as edit to sidecar
  async function handleSaveSpikes() {
    if (!id || detectedSpikes.length === 0) return
    setIsSaving(true)
    try {
      const now = new Date().toISOString()
      const newSidecar: EditSidecar = {
        version: 1,
        activity_id: id,
        created_at: sidecar?.created_at ?? now,
        updated_at: now,
        edits: [
          // Keep non-HR edits from existing sidecar
          ...(sidecar?.edits.filter(e => e.type !== 'hr_spike_smooth') ?? []),
          {
            type: 'hr_spike_smooth',
            method: 'interpolate',
            timestamps: detectedSpikes,
          },
        ],
      }
      await putEdits(id, newSidecar)
      await refetchSidecar()
      showStatus('Edits gespeichert.')
    } catch {
      showStatus('Speichern fehlgeschlagen.')
    } finally {
      setIsSaving(false)
    }
  }

  // Apply edits (trigger recompute)
  async function handleApply() {
    if (!id) return
    setIsApplying(true)
    try {
      await applyEdits(id)
      queryClient.invalidateQueries({ queryKey: ['activity', id] })
      queryClient.invalidateQueries({ queryKey: ['activity-streams', id] })
      showStatus('Edits angewendet und Metriken neu berechnet.', 5000)
    } catch {
      showStatus('Anwenden fehlgeschlagen.')
    } finally {
      setIsApplying(false)
    }
  }

  // Reset edits
  async function handleReset() {
    if (!id) return
    setIsResetting(true)
    try {
      await deleteEdits(id)
      setDetectedSpikes([])
      setShowPreview(false)
      await refetchSidecar()
      showStatus('Edits zurückgesetzt.')
    } catch {
      showStatus('Zurücksetzen fehlgeschlagen.')
    } finally {
      setIsResetting(false)
    }
  }

  if (activityLoading || streamsLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-neutral-400">
        Lade Aktivität...
      </div>
    )
  }

  if (!activity) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <span className="text-neutral-400">Aktivität nicht gefunden</span>
        <Link to="/activities" className="text-sm text-orange-500 hover:text-orange-600">
          Zur Liste
        </Link>
      </div>
    )
  }

  const chartOption = hasHR
    ? buildHRChartOption(streams, displaySpikeTimestamps, showPreview, sidecar ?? null)
    : null

  const hasEdits = (sidecar?.edits.length ?? 0) > 0 || detectedSpikes.length > 0
  const hasUnsavedSpikes = detectedSpikes.length > 0

  return (
    <div className="space-y-6">
      {/* Back link */}
      <Link
        to={`/activities/${id}`}
        className="inline-flex items-center gap-1 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
      >
        <span>&#8592;</span> Zur Aktivitatsdetailseite
      </Link>

      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-neutral-900 dark:text-neutral-100">
            Track bearbeiten
          </h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-0.5">
            {activity.title ?? activity.title_auto ?? 'Kein Titel'}
          </p>
        </div>
        {activity.has_edits && (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400">
            Bearbeitet
          </span>
        )}
      </div>

      {/* Status message */}
      {statusMsg && (
        <div className="rounded-lg border border-blue-200 dark:border-blue-800 bg-blue-50 dark:bg-blue-900/20 px-4 py-2.5 text-sm text-blue-700 dark:text-blue-400">
          {statusMsg}
        </div>
      )}

      {/* HR Chart */}
      {hasHR && chartOption ? (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Herzfrequenz
              {detectedSpikes.length > 0 && (
                <span className="ml-2 text-xs text-orange-500">
                  {detectedSpikes.length} Spike{detectedSpikes.length !== 1 ? 's' : ''} markiert
                </span>
              )}
            </h2>
            <div className="flex items-center gap-2">
              <span className="text-xs text-neutral-500 dark:text-neutral-400">
                {showPreview ? 'Vorschau (korrigiert)' : 'Original'}
              </span>
              <button
                onClick={() => setShowPreview(p => !p)}
                className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none ${
                  showPreview ? 'bg-green-500' : 'bg-neutral-300 dark:bg-neutral-600'
                }`}
                role="switch"
                aria-checked={showPreview}
              >
                <span
                  className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                    showPreview ? 'translate-x-5' : 'translate-x-0.5'
                  }`}
                />
              </button>
            </div>
          </div>
          <ReactECharts
            option={chartOption}
            style={{ height: 260 }}
            theme="dark"
            notMerge={true}
          />
          <div className="mt-2 flex items-center gap-4 text-xs text-neutral-500 dark:text-neutral-400">
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-2 w-4 rounded bg-orange-500 opacity-80" />
              Erkannte Spikes
            </span>
            {showPreview && (
              <span className="flex items-center gap-1.5">
                <span className="inline-block h-2 w-4 rounded bg-green-500 opacity-80" />
                Korrigierte Werte
              </span>
            )}
          </div>
        </div>
      ) : !hasHR ? (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-8 text-center text-sm text-neutral-400">
          Diese Aktivitat enthalt keine Herzfrequenzdaten.
        </div>
      ) : null}

      {/* Toolbar */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
        <h2 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-4">
          Werkzeuge
        </h2>

        <div className="flex flex-wrap gap-3">
          {/* Detect HR spikes */}
          {hasHR && (
            <>
              <button
                onClick={handleDetectSpikes}
                disabled={isDetecting}
                className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                           border border-neutral-200 dark:border-neutral-700
                           text-neutral-700 dark:text-neutral-300
                           hover:bg-neutral-100 dark:hover:bg-neutral-800
                           disabled:opacity-50 disabled:cursor-not-allowed
                           transition-colors"
              >
                <HeartPulseIcon className="w-4 h-4 text-red-500" />
                {isDetecting ? 'Erkenne...' : 'HR-Spikes erkennen'}
              </button>

              {detectedSpikes.length > 0 && (
                <button
                  onClick={handleSaveSpikes}
                  disabled={isSaving}
                  className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                             border border-orange-300 dark:border-orange-700
                             text-orange-700 dark:text-orange-400
                             hover:bg-orange-50 dark:hover:bg-orange-900/20
                             disabled:opacity-50 disabled:cursor-not-allowed
                             transition-colors"
                >
                  <CheckCircleIcon className="w-4 h-4" />
                  {isSaving ? 'Speichere...' : `Spikes korrigieren (${detectedSpikes.length})`}
                </button>
              )}
            </>
          )}

          {/* Spacer */}
          <div className="flex-1" />

          {/* Apply */}
          {hasEdits && (
            <button
              onClick={handleApply}
              disabled={isApplying || hasUnsavedSpikes}
              title={hasUnsavedSpikes ? 'Bitte Edits zuerst speichern' : 'Edits anwenden und Metriken neu berechnen'}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                         bg-orange-500 text-white
                         hover:bg-orange-600
                         disabled:opacity-50 disabled:cursor-not-allowed
                         transition-colors"
            >
              <SaveIcon className="w-4 h-4" />
              {isApplying ? 'Berechne...' : 'Anwenden'}
            </button>
          )}

          {/* Reset */}
          {hasEdits && (
            <button
              onClick={handleReset}
              disabled={isResetting}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                         border border-neutral-200 dark:border-neutral-700
                         text-neutral-500 dark:text-neutral-400
                         hover:bg-neutral-100 dark:hover:bg-neutral-800
                         disabled:opacity-50 disabled:cursor-not-allowed
                         transition-colors"
            >
              <RotateCcwIcon className="w-4 h-4" />
              {isResetting ? 'Setze zurück...' : 'Zurücksetzen'}
            </button>
          )}
        </div>

        {/* Saved edits summary */}
        {(sidecar?.edits.length ?? 0) > 0 && (
          <div className="mt-4 pt-4 border-t border-neutral-100 dark:border-neutral-800">
            <p className="text-xs text-neutral-500 dark:text-neutral-400 font-medium mb-2">
              Gespeicherte Edits ({sidecar!.edits.length}):
            </p>
            <ul className="space-y-1">
              {sidecar!.edits.map((edit, i) => (
                <li key={i} className="text-xs text-neutral-500 dark:text-neutral-400 flex items-center gap-1.5">
                  <span className="w-1.5 h-1.5 rounded-full bg-orange-400 flex-shrink-0" />
                  {editLabel(edit)}
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function editLabel(edit: Edit): string {
  switch (edit.type) {
    case 'hr_spike_smooth':
      return `HR-Spikes glatten: ${edit.timestamps?.length ?? 0} Punkte (${edit.method ?? 'interpolate'})`
    case 'trim_start':
      return `Anfang trimmen ab ${edit.timestamp ?? '?'}`
    case 'trim_end':
      return `Ende trimmen bis ${edit.timestamp ?? '?'}`
    case 'crop':
      return `Ausschneiden: ${edit.from_utc ?? '?'} – ${edit.to_utc ?? '?'}`
    default:
      return edit.type
  }
}

// ── Inline Icons (Lucide-style SVGs) ─────────────────────────────────────────

function HeartPulseIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z" />
      <path d="M3.22 12H9.5l.75-1.5L11.5 14l1.25-2.5 1 2H21" />
    </svg>
  )
}

function RotateCcwIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
      <path d="M3 3v5h5" />
    </svg>
  )
}

function SaveIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z" />
      <polyline points="17 21 17 13 7 13 7 21" />
      <polyline points="7 3 7 8 15 8" />
    </svg>
  )
}

function CheckCircleIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
      <polyline points="22 4 12 14.01 9 11.01" />
    </svg>
  )
}

import { useState, useCallback, useRef, useEffect, useMemo } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchActivity, fetchActivityStreams } from '@/api/activities'
import { EditorMap } from '@/components/editor/EditorMap'
import { TrimControls } from '@/components/editor/TrimControls'

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])
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

interface DetectGPSOutliersResponse {
  count: number
  timestamps: string[]
  max_speed_ms: number
  sport_type: string
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

function detectGPSOutliers(activityId: string): Promise<DetectGPSOutliersResponse> {
  return apiClient<DetectGPSOutliersResponse>(`/activities/${activityId}/edits/detect-gps-outliers`, {
    method: 'POST',
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

  const [detectedGPSOutliers, setDetectedGPSOutliers] = useState<string[]>([])
  const [isDetectingGPS, setIsDetectingGPS] = useState(false)
  const [isSavingGPS, setIsSavingGPS] = useState(false)
  const [gpsMethod, setGpsMethod] = useState<'interpolate' | 'remove'>('interpolate')

  // ── Track / Trim State ─────────────────────────────────────────────────────
  const [trimStart, setTrimStart] = useState(0)
  const [trimEnd, setTrimEnd] = useState(0)
  const [activeMapSelect, setActiveMapSelect] = useState<'start' | 'end' | null>(null)

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

  // ── Track GeoJSON (aus Streams abgeleitet) ────────────────────────────────
  const trackGeoJSON = useMemo<GeoJSON.LineString | null>(() => {
    const gpsPoints = streams.filter(s => s.lat != null && s.lng != null)
    if (gpsPoints.length < 2) return null
    return {
      type: 'LineString',
      coordinates: gpsPoints.map(s => [s.lng!, s.lat!]),
    }
  }, [streams])

  // Saved GPS outlier timestamps from sidecar
  const savedGPSOutlierTimestamps = useMemo(() =>
    sidecar?.edits
      .filter(e => e.type === 'gps_outlier_smooth')
      .flatMap(e => e.timestamps ?? []) ?? [],
    [sidecar],
  )

  // GPS-Outlier-Timestamps → Koordinaten (from detection or saved sidecar)
  const displayGPSOutlierTimestamps = useMemo(() =>
    detectedGPSOutliers.length > 0
      ? detectedGPSOutliers
      : savedGPSOutlierTimestamps,
    [detectedGPSOutliers, savedGPSOutlierTimestamps],
  )
  const outlierPoints = useMemo(() => {
    if (!displayGPSOutlierTimestamps.length || !trackGeoJSON) return []
    const tsSet = new Set(displayGPSOutlierTimestamps)
    return streams
      .filter(s => s.lat != null && s.lng != null && tsSet.has(new Date(s.time).toISOString()))
      .map(s => ({ lat: s.lat!, lng: s.lng!, time: s.time }))
  }, [displayGPSOutlierTimestamps, streams, trackGeoJSON])

  // trimEnd auf letzen Index setzen wenn Track das erste Mal geladen wird
  const didInitTrim = useRef(false)
  useEffect(() => {
    if (!trackGeoJSON || didInitTrim.current) return
    const maxIdx = trackGeoJSON.coordinates.length - 1
    setTrimEnd(maxIdx)
    didInitTrim.current = true
  }, [trackGeoJSON])

  // Map-Klick-Handler: je nach activeMapSelect Start oder Ende setzen
  const handleTrackClick = useCallback((index: number) => {
    if (activeMapSelect === 'start') {
      setTrimStart(index)
    } else if (activeMapSelect === 'end') {
      setTrimEnd(index)
    }
    setActiveMapSelect(null)
  }, [activeMapSelect])

  // ── Build spike set from sidecar (for chart markers if already saved)
  const savedSpikeTimestamps = new Set<string>(
    sidecar?.edits
      .filter(e => e.type === 'hr_spike_smooth')
      .flatMap(e => e.timestamps ?? []) ?? []
  )
  const displaySpikeTimestamps = detectedSpikes.length > 0
    ? new Set(detectedSpikes)
    : savedSpikeTimestamps

  const hasHR = streams.some(s => s.heart_rate != null)

  const chartRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!chartRef.current || !hasHR) return
    const option = buildHRChartOption(streams, displaySpikeTimestamps, showPreview, sidecar ?? null)
    const chart = echarts.getInstanceByDom(chartRef.current) ?? echarts.init(chartRef.current, undefined, { renderer: 'canvas' })
    chart.setOption(option, true)
  })

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
      setDetectedSpikes([])
      setDetectedGPSOutliers([])
      setShowPreview(false)
      queryClient.invalidateQueries({ queryKey: ['activity', id] })
      queryClient.invalidateQueries({ queryKey: ['activity-streams', id] })
      await refetchSidecar()
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
      setDetectedGPSOutliers([])
      setShowPreview(false)
      // Reset trim to full track
      setTrimStart(0)
      if (trackGeoJSON) setTrimEnd(trackGeoJSON.coordinates.length - 1)
      didInitTrim.current = false
      await refetchSidecar()
      showStatus('Edits zurückgesetzt.')
    } catch {
      showStatus('Zurücksetzen fehlgeschlagen.')
    } finally {
      setIsResetting(false)
    }
  }

  // Detect GPS outliers
  async function handleDetectGPSOutliers() {
    if (!id) return
    setIsDetectingGPS(true)
    try {
      const result = await detectGPSOutliers(id)
      setDetectedGPSOutliers(result.timestamps)
      showStatus(
        result.count > 0
          ? `${result.count} GPS-Ausreißer erkannt (Max. ${result.max_speed_ms.toFixed(1)} m/s für ${result.sport_type})`
          : 'Keine GPS-Ausreißer erkannt.',
      )
    } catch {
      showStatus('GPS-Ausreißer-Erkennung fehlgeschlagen.')
    } finally {
      setIsDetectingGPS(false)
    }
  }

  // Save trim as sidecar edits
  async function handleSaveTrim() {
    if (!id || !trackGeoJSON) return
    const maxIdx = trackGeoJSON.coordinates.length - 1
    const trimActive = trimStart > 0 || trimEnd < maxIdx
    if (!trimActive) return

    setIsSaving(true)
    try {
      const now = new Date().toISOString()
      const newEdits: Edit[] = [
        // Keep non-trim edits from existing sidecar
        ...(sidecar?.edits.filter(e => e.type !== 'trim_start' && e.type !== 'trim_end') ?? []),
      ]

      // Map index → timestamp from streams (GPS points only)
      const gpsStreams = streams.filter(s => s.lat != null && s.lng != null)
      if (trimStart > 0 && gpsStreams[trimStart]) {
        newEdits.push({
          type: 'trim_start',
          timestamp: new Date(gpsStreams[trimStart].time).toISOString(),
        })
      }
      if (trimEnd < maxIdx && gpsStreams[trimEnd]) {
        newEdits.push({
          type: 'trim_end',
          timestamp: new Date(gpsStreams[trimEnd].time).toISOString(),
        })
      }

      const newSidecar: EditSidecar = {
        version: 1,
        activity_id: id,
        created_at: sidecar?.created_at ?? now,
        updated_at: now,
        edits: newEdits,
      }
      await putEdits(id, newSidecar)
      await refetchSidecar()
      showStatus('Trim gespeichert.')
    } catch {
      showStatus('Trim-Speichern fehlgeschlagen.')
    } finally {
      setIsSaving(false)
    }
  }

  // Save GPS outliers as edit to sidecar
  async function handleSaveGPSOutliers() {
    if (!id || detectedGPSOutliers.length === 0) return
    setIsSavingGPS(true)
    try {
      const now = new Date().toISOString()
      const newSidecar: EditSidecar = {
        version: 1,
        activity_id: id,
        created_at: sidecar?.created_at ?? now,
        updated_at: now,
        edits: [
          // Keep non-GPS edits from existing sidecar
          ...(sidecar?.edits.filter(e => e.type !== 'gps_outlier_smooth') ?? []),
          {
            type: 'gps_outlier_smooth',
            method: gpsMethod,
            timestamps: detectedGPSOutliers,
          },
        ],
      }
      await putEdits(id, newSidecar)
      await refetchSidecar()
      setDetectedGPSOutliers([])
      showStatus('GPS-Korrektur gespeichert.')
    } catch {
      showStatus('Speichern fehlgeschlagen.')
    } finally {
      setIsSavingGPS(false)
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
        <Link to="/activities" className="text-sm text-accent-DEFAULT hover:text-accent-600">
          Zur Liste
        </Link>
      </div>
    )
  }

  const maxTrackIdx = (trackGeoJSON?.coordinates.length ?? 1) - 1
  const trimActive = trackGeoJSON != null && (trimStart > 0 || trimEnd < maxTrackIdx)
  const hasEdits = (sidecar?.edits.length ?? 0) > 0 || detectedSpikes.length > 0 || detectedGPSOutliers.length > 0 || trimActive
  const hasUnsavedSpikes = detectedSpikes.length > 0 || detectedGPSOutliers.length > 0

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
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-accent-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400">
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

      {/* Track Map + Trim Controls */}
      {trackGeoJSON && (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Strecke
              {activity.has_gps && (
                <span className="ml-2 text-xs text-neutral-400 dark:text-neutral-500 font-normal">
                  {trackGeoJSON.coordinates.length} GPS-Punkte
                </span>
              )}
            </h2>
            {outlierPoints.length > 0 && (
              <span className="text-xs text-orange-600 dark:text-orange-400">
                {outlierPoints.length} Ausreißer markiert
              </span>
            )}
          </div>

          <EditorMap
            trackGeoJSON={trackGeoJSON}
            outlierPoints={outlierPoints}
            trimStart={trimStart}
            trimEnd={trimEnd}
            onTrackClick={activeMapSelect ? handleTrackClick : undefined}
            className="h-72 rounded-lg overflow-hidden"
          />

          <TrimControls
            totalPoints={trackGeoJSON.coordinates.length}
            trimStart={trimStart}
            trimEnd={trimEnd}
            onTrimStartChange={setTrimStart}
            onTrimEndChange={setTrimEnd}
            onSetFromMap={(which) => setActiveMapSelect(prev => prev === which ? null : which)}
            activeMapSelect={activeMapSelect}
          />

          {trimActive && (
            <div className="flex justify-end">
              <button
                onClick={handleSaveTrim}
                disabled={isSaving}
                className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                           border border-amber-300 dark:border-amber-700
                           text-amber-700 dark:text-amber-400
                           hover:bg-amber-50 dark:hover:bg-amber-900/20
                           disabled:opacity-50 disabled:cursor-not-allowed
                           transition-colors"
              >
                <SaveIcon className="w-4 h-4" />
                {isSaving ? 'Speichere...' : 'Trim speichern'}
              </button>
            </div>
          )}
        </div>
      )}

      {/* HR Chart */}
      {hasHR ? (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Herzfrequenz
              {detectedSpikes.length > 0 && (
                <span className="ml-2 text-xs text-accent-DEFAULT">
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
          <div ref={chartRef} style={{ height: 260 }} />
          <div className="mt-2 flex items-center gap-4 text-xs text-neutral-500 dark:text-neutral-400">
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-2 w-4 rounded bg-accent-DEFAULT opacity-80" />
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

          {/* Detect GPS outliers */}
          <>
            <button
              onClick={handleDetectGPSOutliers}
              disabled={isDetectingGPS}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                         border border-neutral-200 dark:border-neutral-700
                         text-neutral-700 dark:text-neutral-300
                         hover:bg-neutral-100 dark:hover:bg-neutral-800
                         disabled:opacity-50 disabled:cursor-not-allowed
                         transition-colors"
            >
              <MapPinIcon className="w-4 h-4 text-blue-500" />
              {isDetectingGPS ? 'Erkenne...' : 'GPS-Ausreißer erkennen'}
            </button>

            {detectedGPSOutliers.length > 0 && (
              <>
                <select
                  value={gpsMethod}
                  onChange={(e) => setGpsMethod(e.target.value as 'interpolate' | 'remove')}
                  className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                             text-sm text-neutral-700 dark:text-neutral-300 px-3 py-2
                             focus:outline-none focus:ring-1 focus:ring-blue-500"
                >
                  <option value="interpolate">Interpolieren</option>
                  <option value="remove">Entfernen</option>
                </select>
                <button
                  onClick={handleSaveGPSOutliers}
                  disabled={isSavingGPS}
                  className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                             border border-blue-300 dark:border-blue-700
                             text-blue-700 dark:text-blue-400
                             hover:bg-blue-50 dark:hover:bg-blue-900/20
                             disabled:opacity-50 disabled:cursor-not-allowed
                             transition-colors"
                >
                  <CheckCircleIcon className="w-4 h-4" />
                  {isSavingGPS ? 'Speichere...' : `GPS korrigieren (${detectedGPSOutliers.length})`}
                </button>
              </>
            )}
          </>

          {/* Spacer */}
          <div className="flex-1" />

          {/* Apply */}
          {hasEdits && (
            <button
              onClick={handleApply}
              disabled={isApplying || hasUnsavedSpikes}
              title={hasUnsavedSpikes ? 'Bitte Edits zuerst speichern' : 'Edits anwenden und Metriken neu berechnen'}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                         bg-accent-DEFAULT text-white
                         hover:bg-accent-600
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
                  <span className="w-1.5 h-1.5 rounded-full bg-accent-400 flex-shrink-0" />
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
    case 'gps_outlier_smooth': {
      const methodLabel = edit.method === 'remove' ? 'Entfernen' : 'Interpolieren'
      return `GPS-Ausreißer korrigieren: ${edit.timestamps?.length ?? 0} Punkte (${methodLabel})`
    }
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

function MapPinIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z" />
      <circle cx="12" cy="10" r="3" />
    </svg>
  )
}

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

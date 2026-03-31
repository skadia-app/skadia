import { useState, useEffect, useRef, useCallback } from 'react'
import { useSearchParams, useNavigate, Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  fetchFusionCandidates,
  executeFusionMerge,
  type FusionCandidate,
} from '@/api/fusion'
import { fmtDateCompact } from '@/lib/format'
import { fetchActivityStreams, fetchActivity } from '@/api/activities'
import type { StreamPoint } from '@/api/types'
import FusionMap, { type TrackInfo, type FusionMapHandle } from '@/components/fusion/FusionMap'
import FusionFieldTable from '@/components/fusion/FusionFieldTable'
import FusionStreamPreview, { type StreamData, type HoverGpsPoint } from '@/components/fusion/FusionStreamPreview'
import FusionMaximizeOverlay from '@/components/fusion/FusionMaximizeOverlay'
import { useToast } from '@/lib/toast'

const TRACK_COLORS = ['#EF4444', '#3B82F6', '#22C55E'] as const
const TRACK_LABELS = ['Kandidat A', 'Kandidat B', 'Kandidat C'] as const

function deviceLabel(device?: string | null, source?: string | null, format?: string | null): string | null {
  const devicePart = device || source || null
  const formatPart = format ? format.toUpperCase() : null
  if (!devicePart && !formatPart) return null
  if (devicePart && formatPart) return `${devicePart} · ${formatPart}`
  return devicePart ?? formatPart
}

function streamToGPS(points: StreamPoint[]): Array<{ lat: number; lng: number }> {
  return points
    .filter(p => p.lat != null && p.lng != null)
    .map(p => ({ lat: p.lat!, lng: p.lng! }))
}

function streamToGPSIndexed(points: StreamPoint[]): Array<{ elapsed: number; lat: number; lng: number }> {
  return points
    .map((p, i) => (p.lat != null && p.lng != null ? { elapsed: i, lat: p.lat, lng: p.lng } : null))
    .filter((p): p is { elapsed: number; lat: number; lng: number } => p !== null)
}

function streamToHR(points: StreamPoint[]): Array<[number, number]> {
  let elapsed = 0
  const result: Array<[number, number]> = []
  for (let i = 0; i < points.length; i++) {
    if (points[i].heart_rate != null) {
      result.push([elapsed, points[i].heart_rate!])
    }
    elapsed++
  }
  return result
}

function streamToPower(points: StreamPoint[]): Array<[number, number]> {
  let elapsed = 0
  const result: Array<[number, number]> = []
  for (let i = 0; i < points.length; i++) {
    if (points[i].power_w != null) {
      result.push([elapsed, points[i].power_w!])
    }
    elapsed++
  }
  return result
}

function streamToAltitude(points: StreamPoint[]): Array<[number, number]> {
  let elapsed = 0
  const result: Array<[number, number]> = []
  for (let i = 0; i < points.length; i++) {
    if (points[i].altitude_m != null) {
      result.push([elapsed, points[i].altitude_m!])
    }
    elapsed++
  }
  return result
}

function streamToSpeed(points: StreamPoint[]): Array<[number, number]> {
  let elapsed = 0
  const result: Array<[number, number]> = []
  for (let i = 0; i < points.length; i++) {
    if (points[i].speed_ms != null) {
      result.push([elapsed, points[i].speed_ms!])
    }
    elapsed++
  }
  return result
}

function streamToCadence(points: StreamPoint[]): Array<[number, number]> {
  let elapsed = 0
  const result: Array<[number, number]> = []
  for (let i = 0; i < points.length; i++) {
    if (points[i].cadence != null) {
      result.push([elapsed, points[i].cadence!])
    }
    elapsed++
  }
  return result
}

function streamToTemperature(points: StreamPoint[]): Array<[number, number]> {
  let elapsed = 0
  const result: Array<[number, number]> = []
  for (let i = 0; i < points.length; i++) {
    if (points[i].temperature_c != null) {
      result.push([elapsed, points[i].temperature_c!])
    }
    elapsed++
  }
  return result
}

interface ActivityInfo {
  id: string
  label: string
  color: string
  candidate: FusionCandidate | null
  isTarget: boolean
  deviceLabel?: string | null
}

export default function FusionPage() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const { addToast } = useToast()
  const targetId = searchParams.get('target') ?? ''
  const initialSource = searchParams.get('source')

  const [selectedSources, setSelectedSources] = useState<string[]>(
    initialSource ? [initialSource] : []
  )
  const [fieldMap, setFieldMap] = useState<Record<string, string>>({})
  const [tagsMerge, setTagsMerge] = useState(false)
  const [titleSource, setTitleSource] = useState<string | null>(null)
  const [gpsPrimarySource, setGpsPrimarySource] = useState('')
  const [gpsExtendTrack, setGpsExtendTrack] = useState(false)
  const [isMerging, setIsMerging] = useState(false)
  const [showMaximize, setShowMaximize] = useState(false)
  const fusionMapRef = useRef<FusionMapHandle>(null)
  const handleHoverGps = useCallback((points: HoverGpsPoint[] | null) => {
    fusionMapRef.current?.setHoverPoints(points)
  }, [])

  const { data: candidates = [], isLoading: loadingCandidates } = useQuery({
    queryKey: ['fusion-candidates', targetId],
    queryFn: () => fetchFusionCandidates(targetId),
    enabled: !!targetId,
  })

  const { data: targetActivity } = useQuery({
    queryKey: ['activity', targetId],
    queryFn: () => fetchActivity(targetId),
    enabled: !!targetId,
  })

  // Auto-select fields based on richest candidate when ?source= is provided and no field selections made yet
  useEffect(() => {
    if (candidates.length === 0 || Object.keys(fieldMap).length > 0) return
    const richer = candidates.reduce((best, c) => {
      const score = [c.has_hr, c.has_power, c.has_altitude, c.has_cadence, c.has_temperature].filter(Boolean).length
      const bestScore = [best?.has_hr, best?.has_power, best?.has_altitude, best?.has_cadence, best?.has_temperature].filter(Boolean).length
      return score > bestScore ? c : best
    }, candidates[0])
    if (!richer) return
    const auto: Record<string, string> = {}
    if (richer.has_hr) auto['hr'] = richer.id
    if (richer.has_power) auto['power'] = richer.id
    if (richer.has_altitude) auto['altitude'] = richer.id
    if (richer.has_cadence) auto['cadence'] = richer.id
    if (richer.has_temperature) auto['temperature'] = richer.id
    if (richer.has_gps) auto['gps'] = richer.id
    setFieldMap(auto)
  }, [candidates])

  // Fetch streams for target + selected sources
  const allIds = [targetId, ...selectedSources].slice(0, 3)

  const targetStreams = useQuery({
    queryKey: ['activity-streams', targetId],
    queryFn: () => fetchActivityStreams(targetId),
    enabled: !!targetId,
  })

  const sourceB = selectedSources[0]
  const sourceC = selectedSources[1]

  const streamB = useQuery({
    queryKey: ['activity-streams', sourceB],
    queryFn: () => fetchActivityStreams(sourceB!),
    enabled: !!sourceB,
  })

  const streamC = useQuery({
    queryKey: ['activity-streams', sourceC],
    queryFn: () => fetchActivityStreams(sourceC!),
    enabled: !!sourceC,
  })

  const streamsByID: Record<string, StreamPoint[]> = {}
  if (targetStreams.data) streamsByID[targetId] = targetStreams.data
  if (sourceB && streamB.data) streamsByID[sourceB] = streamB.data
  if (sourceC && streamC.data) streamsByID[sourceC] = streamC.data

  // Build track info for map
  const tracks: TrackInfo[] = allIds.map((id, i) => ({
    activityId: id,
    label: TRACK_LABELS[i] ?? `Kandidat ${i + 1}`,
    color: TRACK_COLORS[i] ?? '#888888',
    stream: streamsByID[id] ? streamToGPS(streamsByID[id]) : [],
  }))

  // Build stream data for preview
  const streamPreviews: StreamData[] = allIds.map((id, i) => {
    const pts = streamsByID[id] ?? []
    return {
      activityId: id,
      label: TRACK_LABELS[i] ?? `Kandidat ${i + 1}`,
      color: TRACK_COLORS[i] ?? '#888888',
      hrData: streamToHR(pts),
      powerData: streamToPower(pts),
      altitudeData: streamToAltitude(pts),
      speedData: streamToSpeed(pts),
      cadenceData: streamToCadence(pts),
      temperatureData: streamToTemperature(pts),
      gpsPoints: streamToGPSIndexed(pts),
    }
  })

  // Build activities info for field table
  const activities: ActivityInfo[] = allIds.map((id, i) => {
    const candidate = id === targetId ? null : (candidates.find(c => c.id === id) ?? null)
    const dl = id === targetId
      ? deviceLabel(targetActivity?.source_device, targetActivity?.source, targetActivity?.original_file_format)
      : deviceLabel(candidate?.source_device, candidate?.source, candidate?.original_file_format)
    return {
      id,
      label: TRACK_LABELS[i] ?? `Kandidat ${i + 1}`,
      color: TRACK_COLORS[i] ?? '#888888',
      candidate,
      isTarget: id === targetId,
      deviceLabel: dl,
    }
  })

  const handleSourceChange = (slotIndex: number, value: string) => {
    setSelectedSources(prev => {
      const next = [...prev]
      if (value) {
        next[slotIndex] = value
      } else {
        next.splice(slotIndex, 1)
      }
      return next.filter(Boolean)
    })
    // Reset field map when sources change
    setFieldMap({})
    setTitleSource(null)
  }

  const handleExecute = async () => {
    if (!targetId || selectedSources.length === 0) return
    setIsMerging(true)
    try {
      const result = await executeFusionMerge(targetId, {
        sources: selectedSources,
        field_map: fieldMap,
        gps_fusion_config:
          fieldMap['gps'] === 'fuse'
            ? {
                primary_source: gpsPrimarySource || targetId,
                extend_track: gpsExtendTrack,
              }
            : undefined,
        title_source: titleSource ?? undefined,
        tags_merge: tagsMerge,
      })
      addToast({
        type: 'success',
        title: 'Fusion abgeschlossen',
        message: 'Die Aktivität wurde erfolgreich fusioniert.',
      })
      navigate(`/activities/${result.id}`)
    } catch (err) {
      console.error('Fusion failed:', err)
      addToast({
        type: 'error',
        title: 'Fusion fehlgeschlagen',
        message: err instanceof Error ? err.message : 'Unbekannter Fehler',
        duration: 0,
      })
    } finally {
      setIsMerging(false)
    }
  }

  const canExecute =
    selectedSources.length > 0 &&
    Object.keys(fieldMap).length > 0 &&
    !isMerging

  if (!targetId) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-12 text-center">
        <p className="text-neutral-500">
          Keine Ziel-Aktivität angegeben. Öffne diese Seite über eine Aktivität.
        </p>
        <Link
          to="/activities"
          className="mt-4 inline-block text-sm text-accent-DEFAULT hover:underline"
        >
          Zur Aktivitätsliste
        </Link>
      </div>
    )
  }

  return (
    <div className="max-w-7xl mx-auto px-4 py-6 space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <Link
              to="/workshop/fusion"
              className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
            >
              ← Fusionskandidaten
            </Link>
          </div>
          <h1 className="text-2xl font-bold text-neutral-900 dark:text-neutral-100">
            Aktivitäten fusionieren
          </h1>
          <p className="text-sm text-neutral-500 mt-1">
            Kombiniere GPS, HR, Power und andere Streams aus bis zu 3 Aktivitäten
          </p>
        </div>
      </div>

      {/* Quellen-Auswahl */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
        <h2 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">
          Quell-Aktivitäten auswählen
        </h2>

        <div className="space-y-3">
          {/* Target row */}
          <div className="flex items-center gap-3">
            <span
              className="inline-block w-3 h-3 rounded-full flex-shrink-0"
              style={{ backgroundColor: TRACK_COLORS[0] }}
            />
            <div className="flex-1 text-sm text-neutral-700 dark:text-neutral-300 bg-neutral-50 dark:bg-neutral-800 px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700">
              <span className="font-medium">Kandidat A</span>
              <span className="text-neutral-400 ml-2 font-mono text-xs">{targetId.slice(0, 8)}…</span>
              {targetActivity && deviceLabel(targetActivity.source_device, targetActivity.source, targetActivity.original_file_format) && (
                <span className="ml-2 text-xs text-neutral-500 dark:text-neutral-400">
                  {deviceLabel(targetActivity.source_device, targetActivity.source, targetActivity.original_file_format)}
                </span>
              )}
              <span className="ml-2 text-xs text-neutral-400">(Aus den Kandidaten wird eine neue Aktivität erstellt — Originale bleiben erhalten)</span>
            </div>
          </div>

          {/* Source slots */}
          {[0, 1].map(slotIndex => (
            <div key={slotIndex} className="flex items-center gap-3">
              <span
                className="inline-block w-3 h-3 rounded-full flex-shrink-0"
                style={{ backgroundColor: TRACK_COLORS[slotIndex + 1] }}
              />
              <div className="flex-1 flex items-center rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 overflow-hidden focus-within:ring-2 focus-within:ring-accent-DEFAULT/40 transition-shadow">
                <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300 px-3 py-1.5 flex-shrink-0 border-r border-neutral-200 dark:border-neutral-700">
                  {TRACK_LABELS[slotIndex + 1]}
                </span>
                <select
                value={selectedSources[slotIndex] ?? ''}
                onChange={e => handleSourceChange(slotIndex, e.target.value)}
                disabled={loadingCandidates}
                className="flex-1 text-sm bg-transparent px-3 py-1.5 focus:outline-none"
              >
                <option value="">— nicht ausgewählt</option>
                {candidates
                  .filter(
                    c =>
                      !selectedSources.includes(c.id) ||
                      selectedSources[slotIndex] === c.id,
                  )
                  .map(c => (
                    <option key={c.id} value={c.id}>
                      {c.title ?? 'Ohne Titel'} —{' '}
                      {fmtDateCompact(c.started_at, c.timezone)} —{' '}
                      {Math.round((c.distance_m / 1000) * 10) / 10} km
                      {deviceLabel(c.source_device, c.source, c.original_file_format)
                        ? ` — ${deviceLabel(c.source_device, c.source, c.original_file_format)}`
                        : ''}
                    </option>
                  ))}
              </select>
              </div>
            </div>
          ))}

          {loadingCandidates && (
            <p className="text-xs text-neutral-400">Lade Kandidaten…</p>
          )}
          {!loadingCandidates && candidates.length === 0 && (
            <p className="text-xs text-neutral-400">
              Keine passenden Aktivitäten gefunden (gleicher Zeitbereich + Sport erforderlich).
            </p>
          )}
        </div>
      </div>

      {/* Karte + Stream-Vorschau (side by side on large screens) */}
      {selectedSources.length > 0 && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <div className="relative h-64 lg:h-72">
            <FusionMap ref={fusionMapRef} tracks={tracks} />
            <button
              onClick={() => setShowMaximize(true)}
              title="Vergrößern"
              className="absolute top-28 right-2 z-20 cursor-pointer bg-white dark:bg-neutral-800 rounded-md p-1.5 shadow-md hover:bg-neutral-50 dark:hover:bg-neutral-700 transition-colors"
            >
              {/* Maximize SVG Icon: 4 Pfeile nach außen */}
              <svg className="w-4 h-4 text-neutral-600 dark:text-neutral-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 8V4m0 0h4M4 4l5 5m11-5h-4m4 0v4m0-4l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" />
              </svg>
            </button>
          </div>
          <div>
            <FusionStreamPreview streams={streamPreviews} onHoverGps={handleHoverGps} />
          </div>
        </div>
      )}

      {/* Feld-Tabelle */}
      {selectedSources.length > 0 && (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
          <h2 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">
            Datenquellen pro Feld wählen
          </h2>
          <FusionFieldTable
            activities={activities}
            fieldMap={fieldMap}
            onFieldChange={(field, sourceId) =>
              setFieldMap(prev => ({ ...prev, [field]: sourceId }))
            }
            tagsMerge={tagsMerge}
            onTagsMergeChange={setTagsMerge}
            titleSource={titleSource}
            onTitleSourceChange={setTitleSource}
          />

          {/* GPS-Fusion-Konfiguration */}
          {fieldMap['gps'] === 'fuse' && (
            <div className="mt-4 p-3 bg-neutral-50 dark:bg-neutral-800 rounded-lg space-y-2">
              <p className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
                GPS-Fusion-Konfiguration
              </p>
              <div className="flex flex-wrap items-center gap-4">
                <label className="flex items-center gap-2 text-xs text-neutral-600 dark:text-neutral-400">
                  <span>Primäre GPS-Quelle:</span>
                  <select
                    value={gpsPrimarySource || targetId}
                    onChange={e => setGpsPrimarySource(e.target.value)}
                    className="text-xs rounded border border-neutral-300 dark:border-neutral-600 px-2 py-1 bg-white dark:bg-neutral-700 focus:outline-none"
                  >
                    {allIds.map((id, i) => (
                      <option key={id} value={id}>
                        {TRACK_LABELS[i] ?? `Kandidat ${i + 1}`}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="flex items-center gap-1.5 text-xs text-neutral-600 dark:text-neutral-400 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={gpsExtendTrack}
                    onChange={e => setGpsExtendTrack(e.target.checked)}
                    className="accent-accent-DEFAULT"
                  />
                  Track verlängern (Zeitbereiche außerhalb verbinden)
                </label>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Execute-Button */}
      {selectedSources.length > 0 && (
        <div className="flex items-center justify-between gap-4">
          <p className="text-xs text-neutral-400">
            {Object.keys(fieldMap).length === 0
              ? 'Wähle mindestens ein Datenfeld aus, um die Fusion zu starten.'
              : `${Object.keys(fieldMap).length} Feld(er) konfiguriert`}
          </p>
          <button
            onClick={handleExecute}
            disabled={!canExecute}
            className="cursor-pointer px-6 py-2.5 bg-accent-DEFAULT text-white rounded-lg font-medium text-sm hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            {isMerging ? 'Fusion läuft…' : 'Fusion ausführen'}
          </button>
        </div>
      )}

      {showMaximize && (
        <FusionMaximizeOverlay
          tracks={tracks}
          streams={streamPreviews}
          onClose={() => setShowMaximize(false)}
        />
      )}
    </div>
  )
}

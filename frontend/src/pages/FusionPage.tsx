import { useState, useEffect, useMemo, useRef } from 'react'
import { useSearchParams, useNavigate, Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchActivity, fetchActivityStreams } from '@/api/activities'
import {
  fetchRecordingGroup,
  fetchRecordingGroupMembers,
  executeFusionMergeV2,
  setGroupPrimary,
} from '@/api/recordingGroups'
import { fmtDateCompact } from '@/lib/format'
import { deviceLabel } from '@/lib/deviceUtils'
import { getSourceColor } from '@/lib/comparisonColors'
import { SourcesProvider, type SourceInfo } from '@/components/comparison/SourcesContext'
import MultiTrackMap from '@/components/comparison/MultiTrackMap'
import StreamChartContainer from '@/components/comparison/StreamChartContainer'
import FusionFieldTable, { FUSION_FIELDS } from '@/components/fusion/FusionFieldTable'
import { useToast } from '@/lib/toast'
import { invalidateAfterFusion } from '@/lib/fusionCacheInvalidation'

// ── Source letter labels ───────────────────────────────────────────────────────

const SOURCE_LETTERS = ['A', 'B', 'C', 'D', 'E', 'F'] as const

function sourceLabel(index: number): string {
  return SOURCE_LETTERS[index] ?? String(index + 1)
}

// ── Activity metadata for the field table ─────────────────────────────────────

export interface FusionActivityInfo {
  id: string
  label: string
  color: string
  deviceLabel?: string | null
  /** Capability flags — derived from member data or fetched activity */
  has_gps: boolean
  has_hr: boolean
  has_power: boolean
  has_altitude: boolean
  has_cadence: boolean
  has_temperature: boolean
  has_running_dynamics: boolean
  distance_m: number
}

// ── Main component ────────────────────────────────────────────────────────────

export default function FusionPage() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  const groupId = searchParams.get('group') ?? null
  const idsParam = searchParams.get('ids') ?? null

  // selectedSources: ordered list of activity IDs (2–6)
  const [selectedSources, setSelectedSources] = useState<string[]>([])
  const [fieldMap, setFieldMap] = useState<Record<string, string>>({})
  // tagsSelection: either a source UUID (= tags from that source only)
  // or the literal "fuse" (= merge tags from all sources).
  const [tagsSelection, setTagsSelection] = useState<string | null>(null)
  const [titleSource, setTitleSource] = useState<string | null>(null)
  const [gpsPrimarySource, setGpsPrimarySource] = useState('')
  const [gpsExtendTrack, setGpsExtendTrack] = useState(false)
  const [isMerging, setIsMerging] = useState(false)
  const [showGroupPrimaryDialog, setShowGroupPrimaryDialog] = useState(false)
  const [fusedActivityId, setFusedActivityId] = useState<string | null>(null)
  const initialisedRef = useRef(false)

  // ── Data loading ─────────────────────────────────────────────────────────────

  // Load group members when ?group= is set
  const { data: groupMembers = [], isLoading: loadingGroup } = useQuery({
    queryKey: ['recording-group-members', groupId],
    queryFn: () => fetchRecordingGroupMembers(groupId!),
    enabled: !!groupId,
  })

  const { data: group } = useQuery({
    queryKey: ['recording-group', groupId],
    queryFn: () => fetchRecordingGroup(groupId!),
    enabled: !!groupId,
  })

  // Auto-populate sources on first load — from group members or ?ids= param.
  // Uses a ref so that neither branch can fire twice and they don't race each other.
  useEffect(() => {
    if (initialisedRef.current) return
    if (groupMembers.length > 0) {
      initialisedRef.current = true
      // Prefer members that are not currently baked into another fusion
      // (fusion_source_count=0 means not a source of any merged activity).
      // If that filter would leave fewer than two candidates — e.g. all members
      // are sources of a prior merged activity — fall back to the full group so
      // the user has something to work with. They can still remove individual
      // sources manually.
      const eligible = groupMembers.filter(m => m.fusion_source_count === 0)
      const pick = eligible.length >= 2 ? eligible : groupMembers
      setSelectedSources(pick.slice(0, 6).map(m => m.activity_id))
    } else if (idsParam) {
      const ids = idsParam.split(',').filter(Boolean).slice(0, 6)
      if (ids.length > 0) {
        initialisedRef.current = true
        setSelectedSources(ids)
      }
    }
  }, [groupMembers, idsParam])

  // Build the pool of available activity IDs for the dropdowns
  // When from group: all group members. When from ids: just those ids (no extra candidates).
  const poolIds: string[] = useMemo(() => {
    if (groupMembers.length > 0) return groupMembers.map(m => m.activity_id)
    if (idsParam) return idsParam.split(',').filter(Boolean)
    return []
  }, [groupMembers, idsParam])

  // Fetch streams for all selected sources (up to 6 queries)
  const s0Id = selectedSources[0]
  const s1Id = selectedSources[1]
  const s2Id = selectedSources[2]
  const s3Id = selectedSources[3]
  const s4Id = selectedSources[4]
  const s5Id = selectedSources[5]

  const q0 = useQuery({ queryKey: ['activity-streams', s0Id], queryFn: () => fetchActivityStreams(s0Id!), enabled: !!s0Id })
  const q1 = useQuery({ queryKey: ['activity-streams', s1Id], queryFn: () => fetchActivityStreams(s1Id!), enabled: !!s1Id })
  const q2 = useQuery({ queryKey: ['activity-streams', s2Id], queryFn: () => fetchActivityStreams(s2Id!), enabled: !!s2Id })
  const q3 = useQuery({ queryKey: ['activity-streams', s3Id], queryFn: () => fetchActivityStreams(s3Id!), enabled: !!s3Id })
  const q4 = useQuery({ queryKey: ['activity-streams', s4Id], queryFn: () => fetchActivityStreams(s4Id!), enabled: !!s4Id })
  const q5 = useQuery({ queryKey: ['activity-streams', s5Id], queryFn: () => fetchActivityStreams(s5Id!), enabled: !!s5Id })

  const streamQueries = [q0, q1, q2, q3, q4, q5]

  // Fetch activity metadata for capability detection (used when no group members available)
  const a0 = useQuery({ queryKey: ['activity', s0Id], queryFn: () => fetchActivity(s0Id!), enabled: !!s0Id && groupMembers.length === 0 })
  const a1 = useQuery({ queryKey: ['activity', s1Id], queryFn: () => fetchActivity(s1Id!), enabled: !!s1Id && groupMembers.length === 0 })
  const a2 = useQuery({ queryKey: ['activity', s2Id], queryFn: () => fetchActivity(s2Id!), enabled: !!s2Id && groupMembers.length === 0 })
  const a3 = useQuery({ queryKey: ['activity', s3Id], queryFn: () => fetchActivity(s3Id!), enabled: !!s3Id && groupMembers.length === 0 })
  const a4 = useQuery({ queryKey: ['activity', s4Id], queryFn: () => fetchActivity(s4Id!), enabled: !!s4Id && groupMembers.length === 0 })
  const a5 = useQuery({ queryKey: ['activity', s5Id], queryFn: () => fetchActivity(s5Id!), enabled: !!s5Id && groupMembers.length === 0 })
  const activityQueries = [a0, a1, a2, a3, a4, a5]

  // ── Build SourceInfo + FusionActivityInfo in a single pass ───────────────────

  const { sources, fusionActivities } = useMemo(() => {
    const srcs: SourceInfo[] = []
    const fusions: FusionActivityInfo[] = []

    for (let i = 0; i < selectedSources.length; i++) {
      const id = selectedSources[i]
      const member = groupMembers.find(m => m.activity_id === id)
      const streams = streamQueries[i]?.data
      const act = activityQueries[i]?.data

      const dl = member
        ? deviceLabel(member.recording_device_name ?? member.source_device, member.source, member.original_file_format)
        : deviceLabel(act?.source_device, act?.source, act?.original_file_format)

      srcs.push({
        id,
        label: `${sourceLabel(i)}${dl ? ` · ${dl}` : ''}`,
        color: getSourceColor(i),
        activity: act,
        streams,
      })

      // Single pass over stream points to detect all capability flags
      const pts = streams ?? []
      let hasGpsStream = false, hasHr = false, hasPower = false
      let hasAlt = false, hasCad = false, hasTemp = false, hasRunDyn = false
      for (const p of pts) {
        if (!hasGpsStream && p.lat != null) hasGpsStream = true
        if (!hasHr && p.heart_rate != null) hasHr = true
        if (!hasPower && p.power_w != null) hasPower = true
        if (!hasAlt && p.altitude_m != null) hasAlt = true
        if (!hasCad && p.cadence != null) hasCad = true
        if (!hasTemp && p.temperature_c != null) hasTemp = true
        if (!hasRunDyn && p.ground_contact_time_ms != null) hasRunDyn = true
        if (hasGpsStream && hasHr && hasPower && hasAlt && hasCad && hasTemp && hasRunDyn) break
      }

      fusions.push({
        id,
        label: sourceLabel(i),
        color: getSourceColor(i),
        deviceLabel: dl,
        has_gps: member?.has_gps ?? hasGpsStream,
        has_hr: hasHr,
        has_power: hasPower,
        has_altitude: hasAlt,
        has_cadence: hasCad,
        has_temperature: hasTemp,
        has_running_dynamics: hasRunDyn,
        distance_m: member?.distance_m ?? act?.distance_m ?? 0,
      })
    }

    return { sources: srcs, fusionActivities: fusions }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedSources, groupMembers,
    q0.data, q1.data, q2.data, q3.data, q4.data, q5.data,
    a0.data, a1.data, a2.data, a3.data, a4.data, a5.data,
  ])

  // ── Auto-select fields based on richest source ────────────────────────────────

  useEffect(() => {
    if (fusionActivities.length === 0) return
    if (Object.keys(fieldMap).length > 0) return
    const richer = fusionActivities.reduce((best, a) => {
      const score = [a.has_hr, a.has_power, a.has_altitude, a.has_cadence, a.has_temperature].filter(Boolean).length
      const bestScore = [best.has_hr, best.has_power, best.has_altitude, best.has_cadence, best.has_temperature].filter(Boolean).length
      return score > bestScore ? a : best
    }, fusionActivities[0])
    if (!richer) return
    const auto: Record<string, string> = {}
    if (richer.has_gps) auto[FUSION_FIELDS.GPS] = richer.id
    if (richer.has_hr) auto[FUSION_FIELDS.HR] = richer.id
    if (richer.has_power) auto[FUSION_FIELDS.POWER] = richer.id
    if (richer.has_altitude) auto[FUSION_FIELDS.ALTITUDE] = richer.id
    if (richer.has_cadence) auto[FUSION_FIELDS.CADENCE] = richer.id
    if (richer.has_temperature) auto[FUSION_FIELDS.TEMPERATURE] = richer.id
    setFieldMap(auto)
  }, [fusionActivities.map(a => a.id).join(',')])

  // ── Source manipulation ───────────────────────────────────────────────────────

  const handleSourceChange = (index: number, value: string) => {
    setSelectedSources(prev => {
      const next = [...prev]
      next[index] = value
      return next
    })
    setFieldMap({})
    setTitleSource(null)
    setTagsSelection(null)
  }

  const handleSourceRemove = (index: number) => {
    setSelectedSources(prev => prev.filter((_, i) => i !== index))
    setFieldMap({})
    setTitleSource(null)
    setTagsSelection(null)
  }

  const handleAddSource = () => {
    const available = poolIds.find(id => !selectedSources.includes(id))
    if (!available) return
    setSelectedSources(prev => [...prev, available])
  }

  // ── Pool item labels ──────────────────────────────────────────────────────────

  function poolItemLabel(id: string): string {
    const member = groupMembers.find(m => m.activity_id === id)
    if (member) {
      const dl = deviceLabel(member.recording_device_name ?? member.source_device, member.source, member.original_file_format) ?? ''
      const dist = `${Math.round((member.distance_m / 1000) * 10) / 10} km`
      const date = fmtDateCompact(member.started_at, null)
      return [member.title || 'Ohne Titel', date, dist, dl].filter(Boolean).join(' — ')
    }
    return id.slice(0, 8) + '…'
  }

  // ── Fusion execution ──────────────────────────────────────────────────────────

  const handleExecute = async () => {
    if (selectedSources.length < 2 || Object.keys(fieldMap).length === 0) return
    setIsMerging(true)
    try {
      const result = await executeFusionMergeV2({
        sources: selectedSources,
        field_map: fieldMap,
        gps_fusion_config:
          fieldMap[FUSION_FIELDS.GPS] === 'fuse'
            ? { primary_source: gpsPrimarySource || selectedSources[0], extend_track: gpsExtendTrack }
            : undefined,
        title_source: titleSource ?? undefined,
        tags_merge: tagsSelection === 'fuse',
        tags_source: tagsSelection && tagsSelection !== 'fuse' ? tagsSelection : undefined,
        fit_level: false,
        group_id: groupId ?? undefined,
      })

      invalidateAfterFusion(queryClient, {
        recordingGroupId: groupId,
        activityIds: [...selectedSources, result.id],
      })

      if (groupId) {
        setFusedActivityId(result.id)
        setShowGroupPrimaryDialog(true)
      } else {
        addToast({ type: 'success', title: 'Fusion abgeschlossen', message: 'Die Aktivität wurde erfolgreich fusioniert.' })
        navigate(`/activities/${result.id}`)
      }
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

  const handleSetGroupPrimary = async (setAsPrimary: boolean) => {
    setShowGroupPrimaryDialog(false)
    if (setAsPrimary && groupId && fusedActivityId) {
      try {
        await setGroupPrimary(groupId, fusedActivityId)
        addToast({ type: 'success', title: 'Primäre Aktivität gesetzt', message: 'Die fusionierte Aktivität ist nun primär für die Gruppe.' })
      } catch (err) {
        addToast({ type: 'error', title: 'Konnte nicht als primär setzen', message: err instanceof Error ? err.message : 'Unbekannter Fehler', duration: 0 })
      }
    } else {
      addToast({ type: 'success', title: 'Fusion abgeschlossen', message: 'Die Aktivität wurde erfolgreich fusioniert.' })
    }
    if (fusedActivityId) navigate(`/activities/${fusedActivityId}`)
  }

  const canExecute = selectedSources.length >= 2 && Object.keys(fieldMap).length > 0 && !isMerging
  const canAddMore = selectedSources.length < 6 && poolIds.some(id => !selectedSources.includes(id))

  const isLoading = loadingGroup

  // ── Empty state ───────────────────────────────────────────────────────────────

  if (!groupId && !idsParam) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-12 text-center">
        <p className="text-neutral-500">
          Keine Quell-Aktivitäten angegeben. Öffne diese Seite über eine Gruppe oder mit ?ids=…
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
            {groupId ? (
              <Link
                to={`/recording-groups/${groupId}`}
                className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
              >
                ← Gruppenansicht
              </Link>
            ) : (
              <Link
                to="/workshop/fusion"
                className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
              >
                ← Fusionskandidaten
              </Link>
            )}
          </div>
          <h1 className="text-2xl font-bold text-neutral-900 dark:text-neutral-100">
            Aktivitäten fusionieren
          </h1>
          <p className="text-sm text-neutral-500 mt-1">
            Kombiniere GPS, HR, Power und andere Streams aus bis zu 6 Aktivitäten
          </p>
          <p className="text-xs text-neutral-400 mt-0.5">
            Es wird eine neue Aktivität erstellt — alle Originale bleiben erhalten.
          </p>
        </div>
      </div>

      {/* Quellen-Auswahl */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-5">
        <h2 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">
          Quell-Aktivitäten
        </h2>

        {isLoading ? (
          <p className="text-xs text-neutral-400">Lade Aktivitäten…</p>
        ) : (
          <div className="space-y-0">
            {selectedSources.map((id, index) => (
              <div key={`${id}-${index}`} className="flex items-center gap-3 py-2 border-b border-neutral-100 dark:border-neutral-800">
                {/* Color dot */}
                <span
                  className="inline-block w-3 h-3 rounded-full flex-shrink-0"
                  style={{ backgroundColor: getSourceColor(index) }}
                />
                {/* Letter label */}
                <span className="text-sm font-medium w-6 text-neutral-700 dark:text-neutral-300 flex-shrink-0">
                  {sourceLabel(index)}
                </span>
                {/* Dropdown */}
                <select
                  value={id}
                  onChange={e => handleSourceChange(index, e.target.value)}
                  className="flex-1 text-sm border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 bg-white dark:bg-neutral-800 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT/40"
                >
                  {poolIds.length > 0 ? (
                    poolIds
                      .filter(pid => pid === id || !selectedSources.includes(pid))
                      .map(pid => (
                        <option key={pid} value={pid}>
                          {poolItemLabel(pid)}
                        </option>
                      ))
                  ) : (
                    <option value={id}>{poolItemLabel(id)}</option>
                  )}
                </select>
                {/* Remove button — keep min 2 sources */}
                <button
                  onClick={() => handleSourceRemove(index)}
                  disabled={selectedSources.length <= 2}
                  title="Quelle entfernen"
                  className="text-neutral-400 hover:text-red-500 p-1 cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed transition-colors flex-shrink-0"
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            ))}

            {/* Add source button */}
            {canAddMore && (
              <button
                onClick={handleAddSource}
                className="flex items-center gap-2 text-sm text-accent-DEFAULT hover:text-accent-600 py-2 mt-1 cursor-pointer transition-colors"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                Weitere Quelle hinzufügen
                <span className="text-xs text-neutral-400">(max. 6 Quellen)</span>
              </button>
            )}
          </div>
        )}
      </div>

      {/* Map + Chart — only when 2+ sources selected */}
      {selectedSources.length >= 2 && (
        <SourcesProvider sources={sources}>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <MultiTrackMap height="16rem" showHeatmap={false} />
            <StreamChartContainer chartHeight={140} />
          </div>
        </SourcesProvider>
      )}

      {/* Feld-Tabelle (inkl. Meta-Zeilen Titel/Tags) */}
      {selectedSources.length >= 2 && (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-5">
          <h2 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">
            Datenquellen pro Feld wählen
          </h2>
          <FusionFieldTable
            activities={fusionActivities}
            fieldMap={fieldMap}
            onFieldChange={(field, sourceId) =>
              setFieldMap(prev => ({ ...prev, [field]: sourceId }))
            }
            titleSource={titleSource}
            onTitleChange={setTitleSource}
            tagsSelection={tagsSelection}
            onTagsChange={setTagsSelection}
          />

          {/* GPS-Fusion-Konfiguration */}
          {fieldMap[FUSION_FIELDS.GPS] === 'fuse' && (
            <div className="mt-4 p-3 bg-neutral-50 dark:bg-neutral-800 rounded-lg space-y-2">
              <p className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
                GPS-Fusion-Konfiguration
              </p>
              <div className="flex flex-wrap items-center gap-4">
                <label className="flex items-center gap-2 text-xs text-neutral-600 dark:text-neutral-400">
                  <span>Primäre GPS-Quelle:</span>
                  <select
                    value={gpsPrimarySource || selectedSources[0]}
                    onChange={e => setGpsPrimarySource(e.target.value)}
                    className="text-xs rounded border border-neutral-300 dark:border-neutral-600 px-2 py-1 bg-white dark:bg-neutral-700 focus:outline-none"
                  >
                    {selectedSources.map((id, i) => (
                      <option key={id} value={id}>
                        {sourceLabel(i)}
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
      {selectedSources.length >= 2 && (
        <div className="flex items-center justify-between gap-4">
          <p className="text-xs text-neutral-400">
            {Object.keys(fieldMap).length === 0
              ? 'Wähle mindestens ein Datenfeld aus, um die Fusion zu starten.'
              : `${Object.keys(fieldMap).length} Feld(er) konfiguriert`}
          </p>
          <div className="flex items-center gap-3">
            {groupId && (
              <Link
                to={`/recording-groups/${groupId}`}
                className="px-4 py-2.5 border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 rounded-lg text-sm font-medium hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
              >
                Abbrechen
              </Link>
            )}
            <button
              onClick={handleExecute}
              disabled={!canExecute}
              className="cursor-pointer px-6 py-2.5 bg-accent-DEFAULT text-white rounded-lg font-medium text-sm hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              {isMerging ? 'Fusion läuft…' : 'Fusionieren'}
            </button>
          </div>
        </div>
      )}

      {/* Post-fusion dialog: set as group primary? */}
      {showGroupPrimaryDialog && groupId && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
          <div className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800 shadow-xl p-6 max-w-md w-full space-y-4">
            <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
              Fusion abgeschlossen
            </h2>
            <p className="text-sm text-neutral-600 dark:text-neutral-400">
              Fusioniertes Ergebnis als primäre Aktivität der Gruppe setzen?
            </p>
            <div className="flex items-center gap-3 justify-end">
              <button
                onClick={() => handleSetGroupPrimary(false)}
                className="cursor-pointer px-4 py-2 border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 rounded-lg text-sm hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
              >
                Nein
              </button>
              <button
                onClick={() => handleSetGroupPrimary(true)}
                className="cursor-pointer px-4 py-2 bg-accent-DEFAULT text-white rounded-lg text-sm hover:bg-accent-600 transition-colors"
              >
                Ja, als primär setzen
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

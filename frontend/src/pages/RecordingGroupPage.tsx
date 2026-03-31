// RecordingGroupPage — View and manage a single recording group.
// Shows member cards, multi-track map, stream chart comparison, and summary table.

import { useState, useMemo } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { useQuery, useQueries, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchRecordingGroup, setGroupPrimary, setGroupSportType, dissolveRecordingGroup } from '@/api/recordingGroups'
import { fetchActivityStreams } from '@/api/activities'
import { SourcesProvider, type SourceInfo } from '@/components/comparison/SourcesContext'
import MultiTrackMap from '@/components/comparison/MultiTrackMap'
import StreamChartContainer from '@/components/comparison/StreamChartContainer'
import ComparisonSummaryTable from '@/components/comparison/ComparisonSummaryTable'
import { getSourceColor } from '@/lib/comparisonColors'
import { sportLabel } from '@/lib/sportTaxonomy'
import { formatDistance, formatDuration } from '@/lib/utils'
import { useToast } from '@/lib/toast'

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleDateString('de-DE', { day: 'numeric', month: 'short', year: 'numeric' })
}

function formatFileFormat(fmt: string | null | undefined): string {
  if (!fmt) return '—'
  return fmt.toUpperCase()
}

function formatFieldCount(n: number): string {
  return n === 1 ? '1 Feld' : `${n} Felder`
}

// ── Source Card ───────────────────────────────────────────────────────────────

interface SourceCardProps {
  activityId: string
  color: string
  isPrimary: boolean
  deviceName: string
  fileFormat: string | null
  fieldCount: number
  distanceM: number | null | undefined
  durationS: number | null | undefined
  sportType: string
  onSetPrimary: () => void
  isSettingPrimary: boolean
}

function SourceCard({
  activityId,
  color,
  isPrimary,
  deviceName,
  fileFormat,
  fieldCount,
  distanceM,
  durationS,
  sportType,
  onSetPrimary,
  isSettingPrimary,
}: SourceCardProps) {
  const parts: string[] = []
  if (fileFormat) parts.push(formatFileFormat(fileFormat))
  if (fieldCount > 0) parts.push(formatFieldCount(fieldCount))
  if (distanceM != null && distanceM > 0) parts.push(formatDistance(distanceM))
  if (durationS != null && durationS > 0) parts.push(formatDuration(durationS))

  return (
    <div
      className={[
        'bg-white dark:bg-neutral-900 rounded-xl p-4 transition-all duration-150 cursor-pointer',
        'hover:shadow-md hover:-translate-y-px',
        isPrimary
          ? 'border-2 border-accent-DEFAULT'
          : 'border border-neutral-200 dark:border-neutral-800',
      ].join(' ')}
    >
      {/* Header row: dot + name + primary badge/button */}
      <div className="flex items-center gap-2 mb-2">
        <span
          className="w-2.5 h-2.5 rounded-full flex-shrink-0"
          style={{ backgroundColor: color }}
        />
        <Link
          to={`/activities/${activityId}`}
          className="font-semibold text-sm text-neutral-800 dark:text-neutral-100 truncate hover:underline flex-1 min-w-0"
          onClick={e => e.stopPropagation()}
        >
          {deviceName}
        </Link>
        {isPrimary ? (
          <span className="ml-auto flex-shrink-0 text-xs bg-accent-DEFAULT/10 text-accent-DEFAULT px-2 py-0.5 rounded-full font-semibold">
            ★ Primär
          </span>
        ) : (
          <button
            className="ml-auto flex-shrink-0 text-xs text-neutral-400 hover:text-accent-DEFAULT border border-neutral-200 hover:border-accent-DEFAULT dark:border-neutral-700 dark:hover:border-accent-DEFAULT rounded px-2 py-0.5 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            onClick={e => { e.preventDefault(); onSetPrimary() }}
            disabled={isSettingPrimary}
          >
            Primär setzen
          </button>
        )}
      </div>

      {/* Details */}
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        {parts.join(' · ')}
      </p>
      <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-1">
        Sportart:{' '}
        <span className="text-neutral-600 dark:text-neutral-300">
          {sportLabel(sportType)}
        </span>
      </p>
    </div>
  )
}

// ── Sport type mismatch banner ────────────────────────────────────────────────

interface SportTypeBannerProps {
  sportTypes: string[]
  selectedSportType: string | null
  onSelect: (st: string) => void
  isSaving: boolean
}

function SportTypeBanner({ sportTypes, selectedSportType, onSelect, isSaving }: SportTypeBannerProps) {
  return (
    <div className="bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 rounded-xl px-4 py-3 mb-6">
      <p className="text-amber-700 dark:text-amber-400 text-sm font-medium mb-2">
        Unterschiedliche Sportarten erkannt — bitte eine Sportart für die Gruppe wählen
      </p>
      <div className="flex flex-wrap gap-2">
        {sportTypes.map(st => (
          <button
            key={st}
            onClick={() => onSelect(st)}
            disabled={isSaving}
            className={[
              'px-3 py-1.5 rounded-lg text-sm border-2 transition-colors cursor-pointer disabled:opacity-50',
              selectedSportType === st
                ? 'border-amber-500 bg-amber-100 dark:bg-amber-900/40 text-amber-800 dark:text-amber-300 font-semibold'
                : 'border-transparent bg-white dark:bg-neutral-800 hover:border-amber-400 text-neutral-700 dark:text-neutral-200',
            ].join(' ')}
          >
            {sportLabel(st)}
          </button>
        ))}
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function RecordingGroupPage() {
  const { groupId } = useParams<{ groupId: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  // ── Data loading ──────────────────────────────────────────────────────────

  const { data: group, isLoading: groupLoading, error: groupError } = useQuery({
    queryKey: ['recording-group', groupId],
    queryFn: () => fetchRecordingGroup(groupId!),
    enabled: !!groupId,
  })

  const streamQueries = useQueries({
    queries: (group?.members ?? []).map(m => ({
      queryKey: ['activity-streams', m.activity_id],
      queryFn: () => fetchActivityStreams(m.activity_id),
      enabled: !!group,
    })),
  })

  // ── Build SourceInfo[] ────────────────────────────────────────────────────

  const sources = useMemo<SourceInfo[]>(() => {
    if (!group) return []
    return group.members.map((m, i) => ({
      id: m.activity_id,
      label: m.source_device ?? m.source ?? `Quelle ${i + 1}`,
      color: getSourceColor(i),
      streams: streamQueries[i]?.data,
    }))
  }, [group, streamQueries])

  // ── Mutations ─────────────────────────────────────────────────────────────

  const [settingPrimaryId, setSettingPrimaryId] = useState<string | null>(null)

  const setPrimaryMutation = useMutation({
    mutationFn: ({ activityId }: { activityId: string }) =>
      setGroupPrimary(groupId!, activityId),
    onMutate: ({ activityId }) => { setSettingPrimaryId(activityId) },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['recording-group', groupId] })
      addToast({ type: 'success', title: 'Primäre Aufnahme gesetzt' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Setzen der primären Aufnahme', duration: 0 })
    },
    onSettled: () => { setSettingPrimaryId(null) },
  })

  const setSportTypeMutation = useMutation({
    mutationFn: (sportType: string) => setGroupSportType(groupId!, sportType),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['recording-group', groupId] })
      addToast({ type: 'success', title: 'Sportart aktualisiert' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Speichern der Sportart', duration: 0 })
    },
  })

  const dissolveMutation = useMutation({
    mutationFn: () => dissolveRecordingGroup(groupId!),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Gruppe aufgelöst' })
      navigate('/activities')
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Auflösen der Gruppe', duration: 0 })
    },
  })

  // ── Sport type conflict detection ─────────────────────────────────────────

  const uniqueSportTypes = useMemo(() => {
    if (!group) return []
    const types = [...new Set(group.members.map(m => m.sport_type))]
    return types
  }, [group])

  const hasSportTypeConflict = uniqueSportTypes.length > 1
  const effectiveSportType = group?.sport_type ?? uniqueSportTypes[0] ?? 'generic'

  // ── Header info ───────────────────────────────────────────────────────────

  const primaryMember = group?.members.find(m => m.activity_id === group.primary_activity_id)
    ?? group?.members[0]

  // ── Loading / Error states ────────────────────────────────────────────────

  if (groupLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-neutral-300 border-t-accent-DEFAULT" />
      </div>
    )
  }

  if (groupError || !group) {
    return (
      <div className="max-w-4xl mx-auto px-6 py-8">
        <p className="text-red-600 dark:text-red-400">
          Gruppe konnte nicht geladen werden.
        </p>
        <Link to="/activities" className="text-accent-DEFAULT text-sm hover:underline mt-2 inline-block">
          ← Zurück zu Aktivitäten
        </Link>
      </div>
    )
  }

  // ── Render ────────────────────────────────────────────────────────────────

  const memberCount = group.members.length

  return (
    <SourcesProvider sources={sources}>
      <div className="max-w-6xl mx-auto px-6 py-8">
        {/* Back link */}
        <Link
          to="/activities"
          className="text-accent-DEFAULT text-sm hover:underline"
        >
          ← Aktivitäten
        </Link>

        {/* Header */}
        <div className="flex items-center gap-3 mt-2 mb-6 flex-wrap">
          <h1 className="text-2xl font-bold text-neutral-900 dark:text-neutral-100">
            {primaryMember
              ? `${sportLabel(primaryMember.sport_type)} · ${formatDate(primaryMember.started_at)}`
              : 'Aufnahmegruppe'}
          </h1>
          <span className="text-xs bg-accent-DEFAULT/10 text-accent-DEFAULT px-2.5 py-1 rounded-full font-semibold">
            {memberCount === 1 ? '1 Aufnahme' : `${memberCount} Aufnahmen`}
          </span>
        </div>

        {/* Sport type conflict banner */}
        {hasSportTypeConflict && (
          <SportTypeBanner
            sportTypes={uniqueSportTypes}
            selectedSportType={group.sport_type}
            onSelect={st => setSportTypeMutation.mutate(st)}
            isSaving={setSportTypeMutation.isPending}
          />
        )}

        {/* Source cards grid */}
        <div className={`grid gap-4 mb-6 ${
          memberCount <= 2
            ? 'grid-cols-1 sm:grid-cols-2'
            : memberCount <= 3
              ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3'
              : 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3'
        }`}>
          {group.members.map((member, i) => {
            return (
              <SourceCard
                key={member.activity_id}
                activityId={member.activity_id}
                color={getSourceColor(i)}
                isPrimary={member.activity_id === group.primary_activity_id}
                deviceName={member.source_device ?? member.source ?? `Quelle ${i + 1}`}
                fileFormat={member.original_file_format}
                fieldCount={member.field_richness}
                distanceM={member.distance_m}
                durationS={member.elapsed_time_s}
                sportType={member.sport_type}
                onSetPrimary={() => setPrimaryMutation.mutate({ activityId: member.activity_id })}
                isSettingPrimary={settingPrimaryId === member.activity_id}
              />
            )
          })}
        </div>

        {/* Map + Stream chart */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
          {/* Map */}
          <MultiTrackMap
            height="288px"
            showHeatmap={false}
          />

          {/* Stream chart */}
          <StreamChartContainer chartHeight={180} />
        </div>

        {/* Summary table */}
        <div className="mb-8">
          <ComparisonSummaryTable metricColumnLabel="Metrik" />
        </div>

        {/* Actions */}
        <div className="flex items-center justify-between gap-4 flex-wrap">
          {/* Fuse button (left, primary orange) */}
          <button
            onClick={() => navigate(`/fusion?group=${groupId}`)}
            className="bg-accent-DEFAULT hover:bg-accent-600 text-white px-5 py-2.5 rounded-lg text-sm font-semibold transition-colors cursor-pointer"
          >
            Aktivitäten fusionieren
          </button>

          {/* Dissolve group button (right, red outline + confirm) */}
          <button
            className="border border-red-400 text-red-600 hover:bg-red-50 dark:hover:bg-red-950/20 dark:text-red-400 dark:border-red-700 px-5 py-2.5 rounded-lg text-sm font-semibold transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            disabled={dissolveMutation.isPending}
            onClick={() => {
              if (window.confirm(
                'Gruppe auflösen?\n\nDie Aufnahmegruppe wird aufgelöst. Die einzelnen Aktivitäten bleiben erhalten, werden aber nicht mehr als zusammengehörig behandelt. Diese Aktion kann nicht rückgängig gemacht werden.'
              )) {
                dissolveMutation.mutate()
              }
            }}
          >
            {dissolveMutation.isPending ? 'Wird aufgelöst…' : 'Gruppe auflösen'}
          </button>
        </div>
      </div>
    </SourcesProvider>
  )
}

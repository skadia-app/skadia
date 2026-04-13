import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { useToast } from '@/lib/toast'
import { type RecordingGroup, type RecordingGroupMember } from '@/api/types'
import { dissolveRecordingGroup } from '@/api/recordingGroups'
import { fetchActivityStreams } from '@/api/activities'
import { sportLabel, sportColor } from '@/lib/sportTaxonomy'
import { fmtDist } from '@/lib/format'
import { getSourceColor } from '@/lib/comparisonColors'
import { deviceLabel } from '@/lib/deviceUtils'
import QueueMiniMap, { type MiniMapTrack } from '@/components/fusion/QueueMiniMap'
import SportConflictBanner from '@/components/fusion/SportConflictBanner'

interface DuplicateReviewPanelProps {
  groups: RecordingGroup[]
  onDone: () => void
}

function sportBadgeClass(sportType: string): string {
  const color = sportColor(sportType)
  // Map known sport colors to Tailwind badge classes
  const map: Record<string, string> = {
    '#F97316': 'bg-orange-100 text-orange-700',
    '#3B82F6': 'bg-blue-100 text-blue-700',
    '#22C55E': 'bg-green-100 text-green-700',
    '#EF4444': 'bg-red-100 text-red-700',
    '#8B5CF6': 'bg-purple-100 text-purple-700',
    '#EC4899': 'bg-pink-100 text-pink-700',
    '#06B6D4': 'bg-cyan-100 text-cyan-700',
    '#6B7280': 'bg-gray-100 text-gray-700',
  }
  return map[color] ?? 'bg-gray-100 text-gray-700'
}

function overlapBadgeText(members: RecordingGroupMember[]): string {
  // We don't have overlap data on member level — show count-based info
  return `${members.length} Aufnahmen`
}

function timeDiffText(members: RecordingGroupMember[]): string {
  if (members.length < 2) return ''
  const times = members.map(m => new Date(m.started_at).getTime())
  const minT = Math.min(...times)
  const maxT = Math.max(...times)
  const diffMin = Math.round((maxT - minT) / 60000)
  return diffMin === 0 ? 'Zeitdifferenz: 0 min' : `Zeitdifferenz: ${diffMin} min`
}

function uniqueSportTypes(members: RecordingGroupMember[]): string[] {
  return [...new Set(members.map(m => m.sport_type))]
}

function SportBadge({ sportType }: { sportType: string }) {
  return (
    <span className={`text-xs px-1.5 py-0.5 rounded font-medium ${sportBadgeClass(sportType)}`}>
      {sportLabel(sportType)}
    </span>
  )
}

interface GroupCardProps {
  member: RecordingGroupMember
  isRichest: boolean
}

function GroupCard({ member, isRichest }: GroupCardProps) {
  const dl = deviceLabel(member.recording_device_name ?? member.source_device, member.source, member.original_file_format)

  return (
    <div
      className={`flex-1 min-w-0 border rounded-lg p-3 ${
        isRichest
          ? 'border-neutral-200 dark:border-neutral-700 bg-orange-50/40 dark:bg-orange-900/10'
          : 'border-neutral-200 dark:border-neutral-700'
      }`}
    >
      {/* First line: "Mehr Daten" spacer + sport badge — ALL cards same height */}
      <div className="flex items-center justify-between mb-1.5">
        {isRichest ? (
          <span className="text-xs text-accent-DEFAULT dark:text-orange-400 font-semibold">Mehr Daten</span>
        ) : (
          <span className="text-xs invisible" aria-hidden="true">Mehr Daten</span>
        )}
        <SportBadge sportType={member.sport_type} />
      </div>

      <p className="font-semibold text-sm text-neutral-900 dark:text-neutral-100 truncate">{member.title}</p>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        {new Date(member.started_at).toLocaleString('de-DE', { dateStyle: 'short', timeStyle: 'short' })}
      </p>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        {fmtDist(member.distance_m)} · {member.field_richness} Felder
      </p>
      <p className="text-xs text-neutral-400 dark:text-neutral-500 truncate mt-1">
        {dl ?? <span className="invisible">—</span>}
      </p>
    </div>
  )
}

interface GroupItemProps {
  group: RecordingGroup
  onDissolved: () => void
}

function GroupItem({ group, onDissolved }: GroupItemProps) {
  const navigate = useNavigate()
  const { addToast } = useToast()
  const [dissolving, setDissolving] = useState(false)
  const [selectedSport, setSelectedSport] = useState<string | null>(null)

  // Lazy-load streams only once the card scrolls into view
  const cardRef = useRef<HTMLDivElement>(null)
  const [inView, setInView] = useState(false)
  useEffect(() => {
    const el = cardRef.current
    if (!el) return
    const observer = new IntersectionObserver(
      ([entry]) => { if (entry.isIntersecting) { setInView(true); observer.disconnect() } },
      { rootMargin: '200px' },
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const gpsMembers = (group.members ?? []).filter(m => m.has_gps)
  const streamQueries = useQueries({
    queries: gpsMembers.map(m => ({
      queryKey: ['activity-streams', m.activity_id],
      queryFn: () => fetchActivityStreams(m.activity_id),
      enabled: inView,
      staleTime: 5 * 60 * 1000,
    })),
  })

  const tracks: MiniMapTrack[] = (group.members ?? []).map((m, i) => {
    const gpsIdx = gpsMembers.findIndex(gm => gm.activity_id === m.activity_id)
    const points = gpsIdx >= 0
      ? (streamQueries[gpsIdx]?.data ?? [])
          .filter(p => p.lat != null && p.lng != null)
          .map(p => ({ lat: p.lat!, lng: p.lng! }))
      : []
    return {
      points,
      color: getSourceColor(i),
      label: deviceLabel(m.recording_device_name ?? m.source_device, m.source, m.original_file_format) ?? `Aufnahme ${i + 1}`,
    }
  })

  const members = group.members ?? []

  // Sort members by field_richness descending — richest first
  const sorted = [...members].sort((a, b) => b.field_richness - a.field_richness)
  const richestId = sorted[0]?.activity_id

  const sportTypes = uniqueSportTypes(members)
  const hasSportConflict = sportTypes.length > 1

  // Editor disabled when sport conflict unresolved
  const editorDisabled = hasSportConflict && !selectedSport

  async function handleDissolve() {
    setDissolving(true)
    try {
      await dissolveRecordingGroup(group.id)
      onDissolved()
    } catch {
      addToast({ type: 'error', title: 'Auflösung fehlgeschlagen', message: 'Gruppe konnte nicht aufgelöst werden' })
    } finally {
      setDissolving(false)
    }
  }

  // Cards: up to 3 per row, then grid-cols-3 for ≥4
  const useGrid = members.length > 3

  const timeDiff = timeDiffText(members)

  return (
    <div ref={cardRef} className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 mb-6 shadow-sm">
      {/* Header */}
      <div className="flex items-center gap-3 mb-4">
        <span className="text-xs bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 px-2 py-0.5 rounded-full font-semibold">
          {overlapBadgeText(members)}
        </span>
        <span className="text-neutral-400 text-xs">
          {timeDiff && `· ${timeDiff}`}
          {` · ${members.length} Aufnahmen`}
        </span>
      </div>

      {/* Activity Cards */}
      <div className={`${useGrid ? 'grid grid-cols-3' : 'flex'} gap-3 mb-4`}>
        {sorted.map(member => (
          <GroupCard
            key={member.activity_id}
            member={member}
            isRichest={member.activity_id === richestId}
          />
        ))}
      </div>

      {/* Sport conflict warning */}
      {hasSportConflict && (
        <SportConflictBanner
          sportTypes={sportTypes}
          selectedSportType={selectedSport}
          onSelect={setSelectedSport}
        />
      )}

      {/* Mini-Map */}
      <div className="mb-4">
        <QueueMiniMap tracks={tracks} height={112} />
      </div>

      {/* Buttons */}
      <div className="flex gap-3">
        <button
          onClick={() => navigate(`/activities/group/${group.id}`)}
          className="flex-1 bg-accent-DEFAULT hover:bg-accent-600 text-white py-2.5 rounded-lg text-sm font-medium transition-colors cursor-pointer"
        >
          Gruppe ansehen
        </button>
        <button
          onClick={() => navigate(`/fusion?group=${group.id}`)}
          disabled={editorDisabled}
          title={editorDisabled ? 'Sportart muss zuerst gewählt werden' : undefined}
          className="flex-1 bg-accent-DEFAULT hover:bg-accent-600 disabled:bg-neutral-300 dark:disabled:bg-neutral-700 disabled:text-neutral-500 disabled:cursor-not-allowed text-white py-2.5 rounded-lg text-sm font-medium transition-colors cursor-pointer"
        >
          Editor öffnen
        </button>
        <button
          onClick={handleDissolve}
          disabled={dissolving}
          className="flex-1 border border-neutral-300 dark:border-neutral-600 hover:bg-neutral-50 dark:hover:bg-neutral-800 text-neutral-700 dark:text-neutral-300 py-2.5 rounded-lg text-sm font-medium transition-colors cursor-pointer disabled:opacity-60"
        >
          {dissolving ? 'Trenne…' : 'Separate Aktivitäten'}
        </button>
      </div>

      {hasSportConflict && !selectedSport && (
        <p className="text-xs text-amber-600 dark:text-amber-400 mt-2 ml-1">
          Sportart wählen um den Editor zu öffnen, oder in der Gruppenansicht entscheiden.
        </p>
      )}
    </div>
  )
}

export default function DuplicateReviewPanel({ groups, onDone }: DuplicateReviewPanelProps) {
  const [dissolved, setDissolved] = useState<Set<string>>(new Set())

  const visible = groups.filter(g => !dissolved.has(g.id))

  if (visible.length === 0) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-8 text-center">
        <p className="text-neutral-500 dark:text-neutral-400 text-sm">
          Keine Fusionskandidaten gefunden.
        </p>
      </div>
    )
  }

  return (
    <div>
      <p className="text-sm text-neutral-400 dark:text-neutral-500 mb-6">
        {visible.length === 1
          ? '1 Aufnahmegruppe erkannt.'
          : `${visible.length} Aufnahmegruppen erkannt.`}{' '}
        Wähle für jede Gruppe ob die Aktivitäten zusammengehören.
      </p>

      {visible.map(group => (
        <GroupItem
          key={group.id}
          group={group}
          onDissolved={() => setDissolved(prev => new Set([...prev, group.id]))}
        />
      ))}

      <div className="pt-2 mb-8">
        <button
          onClick={onDone}
          className="cursor-pointer w-full border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 rounded-lg px-4 py-2 text-sm font-medium text-neutral-600 dark:text-neutral-400 transition-colors"
        >
          Liste aktualisieren
        </button>
      </div>
    </div>
  )
}

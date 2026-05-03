import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchRecordingGroups } from '@/api/recordingGroups'
import { type RecordingGroup } from '@/api/types'
import { getFusionQueue, dismissFusionCandidate, type FusionQueueEntry } from '@/api/importWizard'
import { useToast } from '@/lib/toast'
import DuplicateReviewPanel from '@/components/DuplicateReviewPanel'
import FusionGroupContextDialog from '@/components/fusion/FusionGroupContextDialog'
import { fmtDateCompact } from '@/lib/format'
import { sportDetailLabel } from '@/lib/sportTaxonomy'
import { activityModifierLabels } from '@/lib/activityModifierLabels'

function fmtDist(m: number): string {
  if (m < 1000) return `${Math.round(m)} m`
  return `${Math.round((m / 1000) * 10) / 10} km`
}

function CandidateRow({ c, onDismissed }: { c: FusionQueueEntry; onDismissed: () => void }) {
  const { addToast } = useToast()
  const navigate = useNavigate()
  const [showGroupDialog, setShowGroupDialog] = useState(false)

  const hasGroupContext = (c.group_id_a ?? null) !== null || (c.group_id_b ?? null) !== null

  async function handleDismiss() {
    try {
      await dismissFusionCandidate(c.activity_id_a, c.activity_id_b)
      onDismissed()
    } catch {
      addToast({ type: 'error', title: 'Dismiss fehlgeschlagen', message: 'Kandidat konnte nicht ignoriert werden.', duration: 0 })
    }
  }

  function sportMeta(sportType: string, subType: string, isElectric: boolean, isIndoor: boolean, isVirtual: boolean) {
    const modifiers = activityModifierLabels({ isElectric, isIndoor, isVirtual })
    return [sportDetailLabel(sportType, subType || null), ...modifiers].join(' · ')
  }

  function handleFusionClick() {
    if (!hasGroupContext) {
      navigate(`/fusion?ids=${c.activity_id_a},${c.activity_id_b}`)
      return
    }
    setShowGroupDialog(true)
  }

  return (
    <>
      <div className="flex items-start gap-4 py-3 border-b border-neutral-100 dark:border-neutral-800 last:border-0">
        <div className="flex-1 min-w-0">
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-sm">
            <div className="min-w-0">
              <div className="flex items-center gap-1.5">
                <p className="font-medium text-neutral-900 dark:text-neutral-100 truncate">{c.title_a || 'Ohne Titel'}</p>
                {(c.group_id_a ?? null) !== null && (
                  <span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400">
                    Gruppe
                  </span>
                )}
              </div>
              <p className="text-xs text-neutral-400">{fmtDateCompact(c.started_at_a, null)} · {fmtDist(c.distance_m_a)} · {sportMeta(c.sport_type_a, c.sub_type_a, c.is_electric_a, c.is_indoor_a, c.is_virtual_a)}</p>
            </div>
            <div className="text-neutral-400 self-center text-xs">↔</div>
            <div className="min-w-0">
              <div className="flex items-center gap-1.5">
                <p className="font-medium text-neutral-900 dark:text-neutral-100 truncate">{c.title_b || 'Ohne Titel'}</p>
                {(c.group_id_b ?? null) !== null && (
                  <span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400">
                    Gruppe
                  </span>
                )}
              </div>
              <p className="text-xs text-neutral-400">{fmtDateCompact(c.started_at_b, null)} · {fmtDist(c.distance_m_b)} · {sportMeta(c.sport_type_b, c.sub_type_b, c.is_electric_b, c.is_indoor_b, c.is_virtual_b)}</p>
            </div>
          </div>
          <p className="text-xs text-neutral-400 mt-1">
            {Math.round(c.overlap_percent * 10) / 10}% Überlappung · {Math.round(Math.abs(c.time_diff_sec) / 60)} min Zeitdifferenz
          </p>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <button
            onClick={handleFusionClick}
            className="cursor-pointer px-3 py-1.5 text-xs rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 transition-colors"
          >
            Fusionieren
          </button>
          <button
            onClick={handleDismiss}
            className="cursor-pointer px-3 py-1.5 text-xs rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
            title="Dieses Paar nicht mehr vorschlagen"
          >
            Nicht fusionieren
          </button>
        </div>
      </div>

      {showGroupDialog && (
        <FusionGroupContextDialog
          activityIdA={c.activity_id_a}
          titleA={c.title_a}
          activityIdB={c.activity_id_b}
          titleB={c.title_b}
          groupIdA={c.group_id_a ?? null}
          groupIdB={c.group_id_b ?? null}
          onClose={() => setShowGroupDialog(false)}
        />
      )}
    </>
  )
}

export default function FusionQueuePage() {
  const queryClient = useQueryClient()

  const { data, isLoading, refetch } = useQuery<RecordingGroup[]>({
    queryKey: ['recording-groups'],
    queryFn: () => fetchRecordingGroups(50),
  })
  const groups = data ?? []

  const { data: queueData, isLoading: queueLoading } = useQuery({
    queryKey: ['fusion-queue'],
    queryFn: () => getFusionQueue(50),
  })
  const queueItems = queueData?.items ?? []
  const queueComputing = queueData?.computing ?? false

  function handleDismissed() {
    queryClient.invalidateQueries({ queryKey: ['fusion-queue'] })
  }

  return (
    <div className="max-w-5xl mx-auto space-y-6 py-8 px-6">
      <div>
        <h1 className="text-2xl font-bold text-neutral-900 dark:text-neutral-100">
          Fusionskandidaten
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Aktivitäten die möglicherweise mit mehreren Geräten aufgezeichnet wurden.
        </p>
        {!isLoading && groups.length > 0 && (
          <p className="mt-0.5 text-sm text-neutral-400 dark:text-neutral-500">
            {groups.length === 1
              ? '1 Aufnahmegruppe erkannt.'
              : `${groups.length} Aufnahmegruppen erkannt.`}{' '}
            Wähle für jede Gruppe ob die Aktivitäten zusammengehören.
          </p>
        )}
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-12 text-neutral-400 text-sm">Lade…</div>
      ) : (
        <DuplicateReviewPanel
          groups={groups}
          onDone={() => refetch()}
        />
      )}

      {/* Tier-2: GPS-Ähnlichkeits-Kandidaten */}
      {(queueLoading || queueItems.length > 0 || queueComputing) && (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-5">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
              GPS-Ähnlichkeitspaare
            </h2>
            {queueComputing && (
              <span className="text-xs text-neutral-400 animate-pulse">Berechne…</span>
            )}
          </div>

          {queueLoading ? (
            <p className="text-xs text-neutral-400">Lade…</p>
          ) : queueItems.length === 0 ? (
            <p className="text-xs text-neutral-400">Keine GPS-Ähnlichkeitspaare erkannt.</p>
          ) : (
            <div>
              {queueItems.map(c => (
                <CandidateRow
                  key={`${c.activity_id_a}-${c.activity_id_b}`}
                  c={c}
                  onDismissed={handleDismissed}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

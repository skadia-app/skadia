import { useState, useRef, useEffect, useCallback, useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HelpCircle, Check, X, Undo2, XIcon } from 'lucide-react'
import { ModalShell } from '@/components/ui/ModalShell'
import { apiFetch } from '@/lib/auth'
import { ApiError, throwOnApiResponse } from '@/api/client'
import { fetchActivityStreams } from '@/api/activities'
import { useToast } from '@/lib/toast'
import type { Activity, LiftCandidate, StreamPoint } from '@/api/types'
import LiftInspectorMap, {
  splitTrackAtGaps,
  CONFIDENCE_LABEL,
  CONFIDENCE_DESCRIPTIONS,
  getConfidenceKey,
} from './LiftInspectorMap'
import LiftHelpModal from './LiftHelpModal'
import { liftDisplayLabel } from './liftLabel'
import LiftDetectionHeader from './LiftDetectionHeader'

// Matches PAUSE_GAP_THRESHOLD_S in LiftInspectorMap — must stay in sync with
// backend enrichment/lifts/window.go SmartRecMaxGapS default (75s).
const PAUSE_GAP_THRESHOLD_S = 75

interface LiftReviewModalProps {
  open: boolean
  onClose: () => void
  activity: Pick<Activity, 'id' | 'lifts_acknowledged_at'>
  candidates: LiftCandidate[]
}

type ModalMode = 'compact' | 'expanded'
type MobileTab = 'list' | 'map'
// F5: state-machine for the lift_recompute polling lifecycle.
// idle      — no job in flight
// starting  — job was triggered; waiting for the first poll showing running=true
// running   — at least one poll confirmed running=true; waiting for running=false
// done      — recompute finished; invalidation fired
type JobPhase = 'idle' | 'starting' | 'running' | 'done'
// Maximum time (ms) to wait in 'starting' before giving up.
const JOB_START_TIMEOUT_MS = 30_000

export function nextLiftJobPhase(phase: JobPhase, liftJobRunning: boolean | undefined): JobPhase {
  if (phase === 'starting' && liftJobRunning === true) return 'running'
  if (phase === 'starting' && liftJobRunning === false) return 'done'
  if (phase === 'running' && liftJobRunning === false) return 'done'
  return phase
}

function useIsDesktop(): boolean {
  // Guard against SSR / environments without window (e.g. test runners using jsdom
  // with a narrow viewport) — default false so mobile layout is the safe fallback.
  const [isDesktop, setIsDesktop] = useState(
    () => typeof window !== 'undefined' ? window.innerWidth >= 1024 : false,
  )
  useEffect(() => {
    if (typeof window === 'undefined') return
    const mq = window.matchMedia('(min-width: 1024px)')
    const handler = (e: MediaQueryListEvent) => setIsDesktop(e.matches)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])
  return isDesktop
}

// Color-coded source+confidence badge matching the inspector map's liftColor() colors.
// Uses both color AND text to be accessible without color vision (F16 a11y fix).
// Fix 6a: each badge has a title attribute from CONFIDENCE_DESCRIPTIONS.
// Phase 11.9: labels and descriptions sourced from centralised CONFIDENCE_LABEL /
// CONFIDENCE_DESCRIPTIONS exports in LiftInspectorMap — single source of truth.
function sourceConfidenceBadge(c: LiftCandidate) {
  const key = getConfidenceKey(c)
  const label = CONFIDENCE_LABEL[key]
  const description = CONFIDENCE_DESCRIPTIONS[key]

  const styleMap: Record<typeof key, { bg: string; text: string; dot: string; border?: string }> = {
    rejected: { bg: '#fef2f2', text: '#dc2626', dot: '#ef4444', border: '1px dashed #ef4444' },
    osm:      { bg: '#eff6ff', text: '#1d4ed8', dot: '#3b82f6' },
    high:     { bg: '#f0fdf4', text: '#15803d', dot: '#22c55e' },
    bidir:    { bg: '#f5f3ff', text: '#7c3aed', dot: '#8b5cf6' },
    medium:   { bg: '#fff7ed', text: '#c2410c', dot: '#f97316' },
  }
  const meta = styleMap[key]

  return (
    <span
      className="inline-flex items-center gap-1 rounded-full text-xs font-medium px-2 py-0.5"
      style={{ backgroundColor: meta.bg, color: meta.text, border: meta.border }}
      title={description}
    >
      <span aria-hidden="true" style={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: meta.dot, display: 'inline-block' }} />
      {label}
    </span>
  )
}

function formatDistance(m?: number | null): string {
  if (m == null) return '–'
  if (m >= 1000) return `${(m / 1000).toFixed(1)} km`
  return `${Math.round(m)} m`
}

function apiErrorCode(err: unknown): string | undefined {
  if (err instanceof ApiError) {
    const code = err.json?.code ?? err.json?.error_code
    return typeof code === 'string' ? code : undefined
  }
  const code = (err as { code?: unknown } | null)?.code
  return typeof code === 'string' ? code : undefined
}

function apiErrorStatus(err: unknown): number | undefined {
  if (err instanceof ApiError) return err.status
  const status = (err as { status?: unknown } | null)?.status
  return typeof status === 'number' ? status : undefined
}

function apiErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    const detail = err.json?.detail ?? err.json?.message
    return typeof detail === 'string' ? detail : (err.body || err.message)
  }
  return err instanceof Error ? err.message : 'Unbekannter Fehler'
}

export default function LiftReviewModal({ open, onClose, activity, candidates }: LiftReviewModalProps) {
  const activityId = activity.id
  const qc = useQueryClient()
  const { addToast } = useToast()

  const [bulkActionDone, setBulkActionDone] = useState(false)
  const [mode, setMode] = useState<ModalMode>('compact')
  const [mobileTab, setMobileTab] = useState<MobileTab>('list')
  const [highlightedId, setHighlightedId] = useState<string | null>(null)
  const [multiSelectMode, setMultiSelectMode] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [showRecalcConfirm, setShowRecalcConfirm] = useState(false)
  // Phase 11.9: help modal state — shared across all three HelpCircle triggers
  // (legend, pause-gap banner, modal header).
  const [helpOpen, setHelpOpen] = useState(false)
  // F5: state machine for lift_recompute polling lifecycle — see JobPhase above.
  const [jobPhase, setJobPhase] = useState<JobPhase>('idle')
  // Expose a derived boolean to child components that only need a binary signal.
  const jobRunning = jobPhase === 'starting' || jobPhase === 'running'
  const startTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const listItemRefs = useRef<Map<string, HTMLElement>>(new Map())
  const isDesktop = useIsDesktop()

  // Poll /jobs/active every 2 s while a job may be in flight (starting or running).
  // TanStack Query v5 removed onSuccess from useQuery — side effects live in useEffect.
  const { data: liftJobRunning } = useQuery({
    queryKey: ['lift-recompute-poll', activityId],
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/jobs/active`)
      const body = await res.json() as { jobs: Array<{ job_type: string; entity_id?: string; status: string }> }
      return body.jobs.some(
        j => j.job_type === 'lift_recompute' && j.entity_id === activityId && j.status === 'running',
      )
    },
    refetchInterval: jobRunning ? 2000 : false,
    enabled: jobRunning,
  })

  // State-machine transitions on each poll result.
  useEffect(() => {
    const next = nextLiftJobPhase(jobPhase, liftJobRunning)
    if (next === jobPhase) return
    if (next === 'done') {
      qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] })
      qc.invalidateQueries({ queryKey: ['lift-candidates-inspector', activityId] })
      qc.invalidateQueries({ queryKey: ['activity', activityId] })
    }
    setJobPhase(next)
    // A fast recompute can complete before the first /jobs/active poll; in
    // that case starting+false is success, not a failed start.
  }, [liftJobRunning, jobPhase, qc, activityId])

  // Timeout guard: if stuck in 'starting' for too long, transition to 'done'
  // so the UI doesn't spin forever when a job never appears in the active list.
  useEffect(() => {
    if (jobPhase === 'starting') {
      startTimeoutRef.current = setTimeout(() => {
        setJobPhase(prev => {
          if (prev !== 'starting') return prev
          addToast({ type: 'warning', title: 'Job nicht gestartet — bitte erneut versuchen' })
          return 'done'
        })
      }, JOB_START_TIMEOUT_MS)
    } else {
      if (startTimeoutRef.current !== null) {
        clearTimeout(startTimeoutRef.current)
        startTimeoutRef.current = null
      }
    }
    return () => {
      if (startTimeoutRef.current !== null) {
        clearTimeout(startTimeoutRef.current)
        startTimeoutRef.current = null
      }
    }
  }, [jobPhase, addToast])

  const unreviewedCandidates = candidates.filter(c => !c.confirmed && !c.rejected_by_user)
  const n = unreviewedCandidates.length
  const totalAscent = Math.round(unreviewedCandidates.reduce((s, c) => s + (c.ascent_m ?? 0), 0))
  const totalDescent = Math.round(unreviewedCandidates.reduce((s, c) => s + (c.descent_m ?? 0), 0))

  useEffect(() => {
    if (open && candidates.length === 0) {
      setMode('expanded')
    }
  }, [open, candidates.length])

  // Fetch all candidates including rejected, for inspector view.
  const { data: inspectorCandidates } = useQuery<LiftCandidate[]>({
    queryKey: ['lift-candidates-inspector', activityId],
    queryFn: () =>
      apiFetch(`/api/v1/activities/${activityId}/lifts/candidates?include_rejected=true`).then(r => r.json()),
    enabled: mode === 'expanded',
  })

  const { data: streams = [] } = useQuery<StreamPoint[]>({
    queryKey: ['activity-streams', activityId],
    queryFn: () => fetchActivityStreams(activityId),
    enabled: mode === 'expanded',
    staleTime: Infinity,
  })

  const displayedCandidates = mode === 'expanded' ? (inspectorCandidates ?? candidates) : candidates

  const candidateHasMapTrack = useMemo(() => {
    const coverage = new Map<string, boolean>()
    for (const c of displayedCandidates) {
      const t0 = new Date(c.started_at).getTime()
      const t1 = new Date(c.ended_at).getTime()
      let count = 0
      for (const s of streams) {
        if (s.lat == null || s.lng == null) continue
        const t = new Date(s.time).getTime()
        if (t >= t0 && t <= t1) {
          count++
          if (count >= 2) break
        }
      }
      coverage.set(c.id, count >= 2)
    }
    return coverage
  }, [displayedCandidates, streams])

  const invalidateAfterBulk = useCallback(() => {
    qc.invalidateQueries({ queryKey: ['activity', activityId] })
    qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] })
    qc.invalidateQueries({ queryKey: ['lift-candidates-inspector', activityId] })
  }, [qc, activityId])

  const confirmAllMutation = useMutation({
    mutationFn: () =>
      throwOnApiResponse(apiFetch(`/api/v1/activities/${activityId}/lifts/confirm-all`, { method: 'POST' })),
    onSuccess: () => {
      invalidateAfterBulk()
      setBulkActionDone(true)
      onClose()
    },
  })

  const rejectAllMutation = useMutation({
    mutationFn: () =>
      throwOnApiResponse(apiFetch(`/api/v1/activities/${activityId}/lifts/reject-all`, { method: 'POST' })),
    onSuccess: () => {
      invalidateAfterBulk()
      setBulkActionDone(true)
      onClose()
    },
  })

  const acknowledgeMutation = useMutation({
    mutationFn: () =>
      throwOnApiResponse(apiFetch(`/api/v1/activities/${activityId}/lifts/acknowledge`, { method: 'POST' })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['activity', activityId] })
    },
  })

  const confirmMutation = useMutation({
    mutationFn: (cid: string) =>
      throwOnApiResponse(apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${cid}/confirm`, { method: 'POST' })),
    onSuccess: () => invalidateAfterBulk(),
  })

  const unconfirmMutation = useMutation({
    mutationFn: (cid: string) =>
      throwOnApiResponse(apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${cid}/unconfirm`, { method: 'PATCH' })),
    onSuccess: () => invalidateAfterBulk(),
    onError: (err: unknown) => {
      const msg = apiErrorCode(err) === 'ALREADY_REJECTED'
        ? 'Bereits abgelehnt — Liste aktualisiert.'
        : apiErrorMessage(err)
      addToast({ type: 'error', title: msg })
      // Re-fetch so the modal reflects the actual server-side state.
      invalidateAfterBulk()
    },
  })

  const rejectMutation = useMutation({
    mutationFn: (cid: string) =>
      throwOnApiResponse(apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${cid}`, { method: 'DELETE' })),
    onSuccess: () => invalidateAfterBulk(),
  })

  const restoreMutation = useMutation({
    mutationFn: (cid: string) =>
      throwOnApiResponse(apiFetch(`/api/v1/lift-candidates/${cid}/restore`, { method: 'POST' })),
    onSuccess: () => invalidateAfterBulk(),
  })

  const recalcMutation = useMutation({
    mutationFn: () =>
      throwOnApiResponse(apiFetch(`/api/v1/activities/${activityId}/lifts/recalculate-all`, { method: 'POST' })),
    onSuccess: () => {
      invalidateAfterBulk()
      setShowRecalcConfirm(false)
      onClose()
    },
    onError: (err: unknown) => {
      if (apiErrorStatus(err) === 409 || apiErrorCode(err) === 'LIFT_RECOMPUTE_RUNNING') {
        addToast({ type: 'warning', title: 'Job läuft bereits — bitte warten' })
      } else {
        addToast({ type: 'error', title: 'Fehler beim Neuberechnen', duration: 0 })
      }
    },
  })

  const handleClose = () => {
    if (!bulkActionDone) {
      acknowledgeMutation.mutate()
    }
    onClose()
  }

  const handleCandidateClick = useCallback((id: string) => {
    setHighlightedId(id)
    // Switch to list tab on mobile when a map polyline is clicked
    setMobileTab('list')
    const el = listItemRefs.current.get(id)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }
  }, [])

  const handleListItemClick = useCallback((id: string) => {
    setHighlightedId(id)
  }, [])

  const toggleMultiSelect = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) { next.delete(id) } else { next.add(id) }
      return next
    })
  }

  const bulkConfirmIds = [...selectedIds].filter(id => {
    const c = displayedCandidates.find(x => x.id === id)
    return c && !c.confirmed && !c.rejected_by_user
  })
  const bulkRejectIds = [...selectedIds].filter(id => {
    const c = displayedCandidates.find(x => x.id === id)
    return c && !c.rejected_by_user
  })

  const handleBulkConfirm = async () => {
    const results = await Promise.allSettled(
      bulkConfirmIds.map(id => confirmMutation.mutateAsync(id)),
    )
    const failed = results.filter(r => r.status === 'rejected').length
    const succeeded = results.length - failed
    if (failed === 0) {
      // All succeeded — exit multi-select mode and show brief confirmation.
      setSelectedIds(new Set())
      setMultiSelectMode(false)
      if (succeeded > 1) {
        addToast({
          type: 'success',
          title: succeeded === 1 ? '1 Lift bestätigt' : `${succeeded} Lifte bestätigt`,
        })
      }
    } else {
      // Partial failure — keep selection open so the user can retry failed items.
      addToast({
        type: 'error',
        title: `${succeeded} von ${results.length} bestätigt — ${failed} fehlgeschlagen`,
        duration: 0,
      })
    }
  }

  const handleBulkReject = async () => {
    const results = await Promise.allSettled(
      bulkRejectIds.map(id => rejectMutation.mutateAsync(id)),
    )
    const failed = results.filter(r => r.status === 'rejected').length
    const succeeded = results.length - failed
    if (failed === 0) {
      setSelectedIds(new Set())
      setMultiSelectMode(false)
      if (succeeded > 1) {
        addToast({
          type: 'success',
          title: succeeded === 1 ? '1 Lift abgelehnt' : `${succeeded} Lifte abgelehnt`,
        })
      }
    } else {
      addToast({
        type: 'error',
        title: `${succeeded} von ${results.length} abgelehnt — ${failed} fehlgeschlagen`,
        duration: 0,
      })
    }
  }

  if (!open) return null

  // ── Compact mode ─────────────────────────────────────────────────────────────
  if (mode === 'compact') {
    const isSingleLift = n === 1
    const compactTitle = isSingleLift ? 'Liftfahrt erkannt' : 'Liftfahrten erkannt'
    const summaryLabel = isSingleLift ? '1 offen' : `${n} offen`
    const confirmLabel = isSingleLift ? 'Bestätigen' : 'Alle bestätigen'
    const rejectLabel = isSingleLift ? 'Ablehnen' : 'Alle ablehnen'

    return (
      <ModalShell onClose={handleClose} maxWidth="max-w-md">
        <div className="space-y-5">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
                {compactTitle}
              </h2>
              <p className="mt-1 text-sm text-neutral-600 dark:text-neutral-400">
                Bitte prüfe den erkannten Lift und wähle die passende Aktion.
              </p>
            </div>
            <button
              onClick={handleClose}
              aria-label="Schließen"
              className="cursor-pointer rounded-md p-1 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-neutral-800 dark:hover:text-neutral-200"
            >
              <XIcon className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>

          <dl className="grid grid-cols-3 gap-2 text-sm">
            <div className="rounded-lg border border-neutral-200 bg-neutral-50 px-3 py-2 dark:border-neutral-800 dark:bg-neutral-950/50">
              <dt className="text-xs text-neutral-500 dark:text-neutral-500">Offen</dt>
              <dd className="mt-0.5 font-medium text-neutral-900 dark:text-neutral-100">{summaryLabel}</dd>
            </div>
            <div className="rounded-lg border border-neutral-200 bg-neutral-50 px-3 py-2 dark:border-neutral-800 dark:bg-neutral-950/50">
              <dt className="text-xs text-neutral-500 dark:text-neutral-500">Aufstieg</dt>
              <dd className="mt-0.5 font-medium text-neutral-900 dark:text-neutral-100">+{totalAscent} m</dd>
            </div>
            <div className="rounded-lg border border-neutral-200 bg-neutral-50 px-3 py-2 dark:border-neutral-800 dark:bg-neutral-950/50">
              <dt className="text-xs text-neutral-500 dark:text-neutral-500">Abstieg</dt>
              <dd className="mt-0.5 font-medium text-neutral-900 dark:text-neutral-100">{totalDescent} m</dd>
            </div>
          </dl>

          <div className="flex flex-col gap-2 sm:grid sm:grid-cols-[1fr_1fr_1.2fr]">
            <button
              onClick={() => confirmAllMutation.mutate()}
              disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
              className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-neutral-100 disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700 dark:text-neutral-200 dark:hover:bg-neutral-800"
            >
              {confirmLabel}
            </button>
            <button
              onClick={() => rejectAllMutation.mutate()}
              disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
              className="cursor-pointer rounded-lg border border-neutral-300 px-4 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-red-50 hover:text-red-700 disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700 dark:text-neutral-200 dark:hover:bg-red-950/30 dark:hover:text-red-300"
            >
              {rejectLabel}
            </button>
            <button
              onClick={() => setMode('expanded')}
              disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
              className="cursor-pointer rounded-lg bg-accent-DEFAULT px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              Prüfen
            </button>
          </div>
        </div>
      </ModalShell>
    )
  }

  // ── Expanded mode ─────────────────────────────────────────────────────────────
  const listPanel = (
    <div className="flex h-full flex-col overflow-hidden">
      <ul className="flex-1 overflow-y-auto divide-y divide-neutral-100 dark:divide-neutral-800">
        {displayedCandidates.map((c, idx) => {
          const isHighlighted = c.id === highlightedId
          const hasMapTrack = candidateHasMapTrack.get(c.id) ?? false

          // OSM-derived display label — empty object for heuristic rows or
          // OSM rows whose aerialway is not whitelisted (defensive: better
          // empty than wrong-category).
          const label = liftDisplayLabel(c)
          const idLabel = `L${String(idx + 1).padStart(2, '0')}`

          // Compact icon-button styling — shared across all action variants
          // so the row stays visually quiet.
          const iconBtn =
            'cursor-pointer inline-flex items-center justify-center w-7 h-7 rounded-md ' +
            'border transition-colors disabled:opacity-50 disabled:cursor-not-allowed'

          return (
            <li
              key={c.id}
              ref={el => {
                if (el) listItemRefs.current.set(c.id, el)
                else listItemRefs.current.delete(c.id)
              }}
              onClick={() => handleListItemClick(c.id)}
              className={`px-3 py-1.5 cursor-pointer transition-colors text-sm
                ${isHighlighted ? 'bg-accent-DEFAULT/10 dark:bg-accent-DEFAULT/15' : 'hover:bg-neutral-50 dark:hover:bg-neutral-800/50'}`}
            >
              {/* Row 1 — identifier + name (when known) + action icons */}
              <div className="flex items-start gap-2">
                {multiSelectMode && !c.rejected_by_user && (
                  <input
                    type="checkbox"
                    checked={selectedIds.has(c.id)}
                    onChange={e => { e.stopPropagation(); toggleMultiSelect(c.id) }}
                    onClick={e => e.stopPropagation()}
                    aria-label={`Lift ${idx + 1} auswählen`}
                    className="mt-0.5 rounded border-neutral-300 dark:border-neutral-600 cursor-pointer"
                  />
                )}
                <div className="flex-1 min-w-0 flex items-baseline gap-1.5 flex-wrap">
                  <span className="font-medium text-neutral-700 dark:text-neutral-300 shrink-0">
                    {idLabel}
                  </span>
                  {label.name && (
                    <>
                      <span className="text-neutral-400 dark:text-neutral-600" aria-hidden="true">·</span>
                      <span className="font-medium text-neutral-900 dark:text-neutral-100 truncate">
                        {label.name}
                      </span>
                    </>
                  )}
                  {label.type && (
                    <>
                      {label.name && (
                        <span className="text-neutral-400 dark:text-neutral-600" aria-hidden="true">·</span>
                      )}
                      <span className="text-neutral-500 dark:text-neutral-400 truncate">
                        {label.type}
                      </span>
                    </>
                  )}
                </div>
                <div className="flex items-center gap-0.5 shrink-0" onClick={e => e.stopPropagation()}>
                  {c.confirmed && !c.rejected_by_user && (
                    <button
                      onClick={() => unconfirmMutation.mutate(c.id)}
                      disabled={unconfirmMutation.isPending}
                      aria-label="Rückgängig machen"
                      title="Rückgängig"
                      className={`${iconBtn} border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800`}
                    >
                      <Undo2 className="w-3.5 h-3.5" aria-hidden="true" />
                    </button>
                  )}
                  {!c.confirmed && !c.rejected_by_user && (
                    <button
                      onClick={() => confirmMutation.mutate(c.id)}
                      disabled={confirmMutation.isPending}
                      aria-label="Bestätigen"
                      title="Bestätigen"
                      className={`${iconBtn} border-green-300 dark:border-green-700 text-green-700 dark:text-green-400 hover:bg-green-50 dark:hover:bg-green-900/20`}
                    >
                      <Check className="w-3.5 h-3.5" aria-hidden="true" />
                    </button>
                  )}
                  {!c.rejected_by_user && (
                    <button
                      onClick={() => rejectMutation.mutate(c.id)}
                      disabled={rejectMutation.isPending}
                      aria-label="Ablehnen"
                      title="Ablehnen"
                      className={`${iconBtn} border-red-300 dark:border-red-700 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20`}
                    >
                      <X className="w-3.5 h-3.5" aria-hidden="true" />
                    </button>
                  )}
                  {c.rejected_by_user && (
                    <button
                      onClick={() => restoreMutation.mutate(c.id)}
                      disabled={restoreMutation.isPending}
                      aria-label="Wiederherstellen"
                      title="Wiederherstellen"
                      className={`${iconBtn} border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800`}
                    >
                      <Undo2 className="w-3.5 h-3.5" aria-hidden="true" />
                    </button>
                  )}
                </div>
              </div>

              {/* Row 2 — detection method + status (compact, single line) */}
              <div className="mt-0.5 flex items-center gap-2 text-xs flex-wrap">
                {sourceConfidenceBadge(c)}
                {!c.rejected_by_user && (
                  <span className={`shrink-0 ${
                    c.confirmed ? 'text-green-600 dark:text-green-400' : 'text-neutral-400 dark:text-neutral-500'
                  }`}>
                    · {c.confirmed ? 'bestätigt' : 'offen'}
                  </span>
                )}
              </div>

              {/* Row 3 — distance / ascent / descent */}
              <div className="mt-0.5 flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400 flex-wrap">
                <span>{formatDistance(c.distance_m)}</span>
                <span>+{Math.round(c.ascent_m ?? 0)} m</span>
                {(c.descent_m ?? 0) > 0 && <span>−{Math.round(c.descent_m!)} m</span>}
                {/* Warn when stream data does not cover this lift's time range — map will not show a track. */}
                {!hasMapTrack && (
                  <span className="text-amber-600 dark:text-amber-400" title="Stream-Daten fehlen für diesen Zeitraum">
                    (keine Karten-Spur)
                  </span>
                )}
              </div>
            </li>
          )
        })}
        {displayedCandidates.length === 0 && (
          <li className="px-3 py-6 text-sm text-neutral-400 dark:text-neutral-500 text-center">
            Keine Liftfahrten gefunden
          </li>
        )}
      </ul>
      <div className="shrink-0 border-t border-neutral-200 dark:border-neutral-700 px-3 py-2">
        {multiSelectMode && selectedIds.size > 0 ? (
          <div className="flex flex-wrap items-center gap-2">
            <span className="mr-auto text-sm font-medium text-neutral-600 dark:text-neutral-300">
              {selectedIds.size === 1 ? '1 gewählt' : `${selectedIds.size} gewählt`}
            </span>
            <button
              onClick={handleBulkConfirm}
              disabled={confirmMutation.isPending || bulkConfirmIds.length === 0}
              className="cursor-pointer rounded-lg border border-green-300 px-3 py-2 text-sm font-medium text-green-700 transition-colors hover:bg-green-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-green-700 dark:text-green-400 dark:hover:bg-green-900/20"
            >
              Bestätigen
            </button>
            <button
              onClick={handleBulkReject}
              disabled={rejectMutation.isPending || bulkRejectIds.length === 0}
              className="cursor-pointer rounded-lg border border-red-300 px-3 py-2 text-sm font-medium text-red-600 transition-colors hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-700 dark:text-red-400 dark:hover:bg-red-900/20"
            >
              Ablehnen
            </button>
          </div>
        ) : (
          <button
            onClick={() => {
              setMultiSelectMode(v => !v)
              if (multiSelectMode) setSelectedIds(new Set())
            }}
            className={`w-full cursor-pointer rounded-lg border px-3 py-2 text-sm font-medium transition-colors
              ${multiSelectMode
                ? 'bg-neutral-200 dark:bg-neutral-700 border-neutral-400 dark:border-neutral-500 text-neutral-800 dark:text-neutral-200'
                : 'border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
              }`}
          >
            {multiSelectMode ? 'Multi-Auswahl beenden' : 'Multi-Auswahl'}
          </button>
        )}
      </div>
    </div>
  )

  const mapPanel = (
    <LiftInspectorMap
      candidates={displayedCandidates}
      streams={streams}
      highlightedCandidateId={highlightedId}
      onCandidateClick={handleCandidateClick}
      onOpenHelp={() => setHelpOpen(true)}
    />
  )

  const bulkFooter = (
    <div className="flex items-center gap-2 flex-wrap shrink-0">
      <button
        onClick={() => setShowRecalcConfirm(true)}
        disabled={recalcMutation.isPending}
        className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Neu erkennen
      </button>
      <button
        onClick={() => rejectAllMutation.mutate()}
        disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
        className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Alle ablehnen
      </button>
      <button
        onClick={() => confirmAllMutation.mutate()}
        disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
        className="cursor-pointer rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Alle bestätigen
      </button>
    </div>
  )

  // ── Recalculate confirm dialog ────────────────────────────────────────────────
  const recalcDialog = showRecalcConfirm ? (
    <div className="absolute inset-0 z-10 flex items-center justify-center bg-black/30 rounded-xl">
      <div className="bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-xl shadow-2xl p-6 max-w-sm mx-4 space-y-4">
        <h3 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          Neu erkennen?
        </h3>
        <p className="text-sm text-neutral-600 dark:text-neutral-400">
          Alle erkannten Liftfahrten dieser Aktivität verwerfen und neu erkennen lassen? Bisher abgelehnte Lifte werden ggf. wieder vorgeschlagen.
        </p>
        <div className="flex gap-2 justify-end">
          <button
            onClick={() => setShowRecalcConfirm(false)}
            className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
          >
            Abbrechen
          </button>
          <button
            onClick={() => recalcMutation.mutate()}
            disabled={recalcMutation.isPending}
            className="cursor-pointer rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            Ja, neu erkennen
          </button>
        </div>
      </div>
    </div>
  ) : null

  // D.2: compute pause-gap count once from the loaded streams.
  const pauseGapCount = (() => {
    if (streams.length === 0) return 0
    const segments = splitTrackAtGaps(streams, PAUSE_GAP_THRESHOLD_S)
    return Math.max(0, segments.length - 1)
  })()

  return (
    <>
    <ModalShell onClose={handleClose} maxWidth="!max-w-[95vw] w-[95vw]">
      <div className="relative flex flex-col" style={{ height: 'calc(90dvh - 48px)', minHeight: '480px' }}>
        {recalcDialog}

        {/* Header — HelpCircle (Position 3) opens LiftHelpModal */}
        <div className="flex items-center justify-between gap-3 mb-3 shrink-0">
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
              Lifte prüfen
            </h2>
            <button
              onClick={() => setHelpOpen(true)}
              aria-label="Hilfe zur Lifterkennung öffnen"
              className="text-neutral-400 dark:text-neutral-500 hover:text-accent-DEFAULT dark:hover:text-accent-DEFAULT cursor-pointer transition-colors"
            >
              <HelpCircle className="w-5 h-5" aria-hidden="true" />
            </button>
          </div>
          <button
            onClick={handleClose}
            aria-label="Schließen"
            className="cursor-pointer rounded-md p-1 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-neutral-800 dark:hover:text-neutral-200"
          >
            <XIcon className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>

        {isDesktop ? (
          // Desktop: Fix 3 — 70/30 split map/list (map gets more, list narrower).
          <>
            <div className="flex flex-1 gap-3 min-h-0">
              {/* Map — gets the lion's share. min-h-0 so flex doesn't refuse to
                  shrink, h-full propagates the column height to the map container. */}
              <div className="relative flex-[8] min-h-0 h-full">
                {/* Pause-gap banner — informative two-line note centred above
                    the map. max-w keeps it from overlapping the legend
                    (top-left) or the layer-picker / heatmap card (top-right).
                    z-20 keeps it above the map's z-10 legend/controls. */}
                {pauseGapCount > 0 && (
                  <div
                    className="absolute top-2 left-1/2 -translate-x-1/2 z-20 max-w-xl w-[min(38rem,calc(100%-18rem))] rounded-lg border border-amber-300 dark:border-amber-700 bg-amber-50/95 dark:bg-amber-900/90 px-3 py-2 text-xs text-amber-800 dark:text-amber-200 shadow-md"
                    role="note"
                    data-testid="pause-gap-banner"
                  >
                    <div className="flex items-start gap-2">
                      <span className="shrink-0 text-base leading-none mt-0.5" aria-hidden="true">⚠</span>
                      <div className="flex-1 space-y-1 leading-snug">
                        <p>
                          <span className="font-semibold">
                            Aufzeichnung enthält {pauseGapCount} Pause{pauseGapCount > 1 ? 'n' : ''} (&gt;{PAUSE_GAP_THRESHOLD_S}s).
                          </span>
                        </p>
                        <p>
                          Die Karte zeichnet die Linie durchgängig, markiert die Teleportations-Abschnitte
                          aber gestrichelt. Solche Lücken werden vom Detektor{' '}
                          <span className="font-semibold">nicht</span> als Lift erkannt.
                        </p>
                      </div>
                      <button
                        type="button"
                        onClick={() => setHelpOpen(true)}
                        aria-label="Hilfe zur Lifterkennung öffnen"
                        className="shrink-0 mt-0.5 text-amber-600 dark:text-amber-400 hover:text-amber-900 dark:hover:text-amber-100 cursor-pointer transition-colors"
                      >
                        <HelpCircle className="w-4 h-4" aria-hidden="true" />
                      </button>
                    </div>
                  </div>
                )}
                {mapPanel}
              </div>
              {/* List — narrower (≈22%); the panel owns its own scrolling and footer. */}
              <div className="flex-[2.5] min-h-0 border border-neutral-200 dark:border-neutral-700 rounded-lg overflow-hidden">
                {listPanel}
              </div>
            </div>
            {/* Footer */}
            <div className="mt-3 shrink-0 flex gap-3">
              <div className="flex-[8] min-w-0 flex items-center gap-2">
                <LiftDetectionHeader
                  activityId={activityId}
                  jobRunning={jobRunning}
                  onJobStarted={() => setJobPhase('starting')}
                  compact
                />
                <button
                  onClick={() => setMode('compact')}
                  className="ml-auto cursor-pointer whitespace-nowrap text-sm text-neutral-500 hover:text-neutral-700 dark:text-neutral-400 dark:hover:text-neutral-200 transition-colors"
                >
                  Zurück zur Übersicht
                </button>
              </div>
              <div className="flex-[2.5] min-w-0 flex items-center justify-end">
                {bulkFooter}
              </div>
            </div>
          </>
        ) : (
          // Mobile: tab layout — Header / Tabs / Content / Footer
          <>
            {/* Tab bar */}
            <div className="flex shrink-0 border-b border-neutral-200 dark:border-neutral-700 mb-2">
              {(['list', 'map'] as const).map(tab => (
                <button
                  key={tab}
                  onClick={() => setMobileTab(tab)}
                  className={`flex-1 py-2 text-sm font-medium cursor-pointer transition-colors border-b-2 -mb-px
                    ${mobileTab === tab
                      ? 'border-accent-DEFAULT text-accent-DEFAULT'
                      : 'border-transparent text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200'
                    }`}
                >
                  {tab === 'list' ? 'Liste' : 'Karte'}
                </button>
              ))}
            </div>
            {/* Active tab content — banner shown above map on mobile when map tab is active */}
            <div className="flex flex-col flex-1 min-h-0 gap-2">
              {mobileTab === 'map' && pauseGapCount > 0 && (
                <button
                  type="button"
                  onClick={() => setHelpOpen(true)}
                  className="animate-fade-in shrink-0 rounded-lg border border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20 px-3 py-2 text-xs text-amber-800 dark:text-amber-200 text-left flex items-center gap-2 hover:bg-amber-100 dark:hover:bg-amber-900/40 cursor-pointer transition-colors"
                  aria-label="Pause-Lücken-Hinweis — Hilfe öffnen"
                  data-testid="pause-gap-banner"
                >
                  <span className="shrink-0 text-base leading-none" aria-hidden="true">⚠</span>
                  <span className="flex-1 leading-tight">
                    <span className="font-semibold">
                      {pauseGapCount} Pause{pauseGapCount > 1 ? 'n' : ''} (&gt;{PAUSE_GAP_THRESHOLD_S}s)
                    </span>{' '}
                    <span className="opacity-90">— über Pausen kann der Detektor keinen Lift erkennen.</span>
                  </span>
                  <HelpCircle className="w-4 h-4 shrink-0 opacity-70" aria-hidden="true" />
                </button>
              )}
              <div className="flex-1 min-h-0">
                {mobileTab === 'list'
                  ? <div className="h-full border border-neutral-200 dark:border-neutral-700 rounded-lg overflow-hidden">{listPanel}</div>
                  : mapPanel
                }
              </div>
            </div>
            {/* Sticky footer */}
            <div className="mt-3 shrink-0 flex flex-wrap items-center gap-2">
              <LiftDetectionHeader
                activityId={activityId}
                jobRunning={jobRunning}
                onJobStarted={() => setJobPhase('starting')}
                compact
              />
              {bulkFooter}
              <button
                onClick={() => setMode('compact')}
                className="ml-auto cursor-pointer text-sm text-neutral-500 hover:text-neutral-700 dark:text-neutral-400 dark:hover:text-neutral-200 transition-colors"
              >
                Zurück zur Übersicht
              </button>
            </div>
          </>
        )}
      </div>
    </ModalShell>
      {/* Phase 11.9 / Fix 6: LiftHelpModal rendered AFTER ModalShell so its z-50
          fixed overlay wins over the review modal's z-50 via DOM order. */}
      <LiftHelpModal open={helpOpen} onClose={() => setHelpOpen(false)} />
    </>
  )
}

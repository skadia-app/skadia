import { useState, useRef, useEffect, useCallback } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ModalShell } from '@/components/ui/ModalShell'
import { apiFetch } from '@/lib/auth'
import { fetchActivityStreams } from '@/api/activities'
import { useToast } from '@/lib/toast'
import type { Activity, LiftCandidate, StreamPoint } from '@/api/types'
import LiftInspectorMap, { splitTrackAtGaps } from './LiftInspectorMap'

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
function sourceConfidenceBadge(source: string, confidence: string, rejectedByUser: boolean) {
  if (rejectedByUser) {
    return (
      <span
        className="inline-flex items-center gap-1 rounded-full text-xs font-medium px-2 py-0.5"
        style={{ backgroundColor: '#fef2f2', color: '#dc2626', border: '1px dashed #ef4444' }}
      >
        <span aria-hidden="true" style={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: '#ef4444', display: 'inline-block' }} />
        Abgelehnt
      </span>
    )
  }
  if (source === 'osm') {
    return (
      <span
        className="inline-flex items-center gap-1 rounded-full text-xs font-medium px-2 py-0.5"
        style={{ backgroundColor: '#eff6ff', color: '#1d4ed8' }}
      >
        <span aria-hidden="true" style={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: '#3b82f6', display: 'inline-block' }} />
        OSM-Treffer
      </span>
    )
  }
  // Heuristic — confidence determines color
  const confMap: Record<string, { label: string; bg: string; text: string; dot: string }> = {
    high:  { label: 'Heuristik (hoch)',          bg: '#f0fdf4', text: '#15803d', dot: '#22c55e' },
    bidir: { label: 'Heuristik (bidirektional)',  bg: '#f5f3ff', text: '#7c3aed', dot: '#8b5cf6' },
    medium:{ label: 'Heuristik (mittel)',         bg: '#fff7ed', text: '#c2410c', dot: '#f97316' },
  }
  const meta = confMap[confidence] ?? { label: `Heuristik (${confidence})`, bg: '#f5f5f5', text: '#525252', dot: '#a3a3a3' }
  return (
    <span
      className="inline-flex items-center gap-1 rounded-full text-xs font-medium px-2 py-0.5"
      style={{ backgroundColor: meta.bg, color: meta.text }}
    >
      <span aria-hidden="true" style={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: meta.dot, display: 'inline-block' }} />
      {meta.label}
    </span>
  )
}

// Legacy helpers kept for compact-mode summary (not used in list rows).
function sourceBadge(source: string) {
  return source === 'osm'
    ? <span className="inline-flex items-center rounded-full bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 text-xs font-medium px-2 py-0.5">OSM</span>
    : <span className="inline-flex items-center rounded-full bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 text-xs font-medium px-2 py-0.5">Heuristik</span>
}

function confidenceBadge(confidence: string) {
  const labels: Record<string, string> = { high: 'Hoch', medium: 'Mittel', bidir: 'Bidirektional' }
  return (
    <span className="inline-flex items-center rounded-full bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 text-xs px-2 py-0.5">
      {labels[confidence] ?? confidence}
    </span>
  )
}

function statusLabel(c: LiftCandidate): string {
  if (c.rejected_by_user) return 'abgelehnt'
  if (c.confirmed) return 'bestätigt'
  return 'offen'
}

function formatDistance(m?: number | null): string {
  if (m == null) return '–'
  if (m >= 1000) return `${(m / 1000).toFixed(1)} km`
  return `${Math.round(m)} m`
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
  const listItemRefs = useRef<Map<string, HTMLElement>>(new Map())
  const isDesktop = useIsDesktop()

  const unreviewedCandidates = candidates.filter(
    c => c.source === 'heuristic' && !c.confirmed && !c.rejected_by_user,
  )
  const n = unreviewedCandidates.length
  const totalAscent = Math.round(unreviewedCandidates.reduce((s, c) => s + (c.ascent_m ?? 0), 0))
  const totalDescent = Math.round(unreviewedCandidates.reduce((s, c) => s + (c.descent_m ?? 0), 0))

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

  const invalidateAfterBulk = useCallback(() => {
    qc.invalidateQueries({ queryKey: ['activity', activityId] })
    qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] })
    qc.invalidateQueries({ queryKey: ['lift-candidates-inspector', activityId] })
  }, [qc, activityId])

  // apiFetch returns Response on every status — including 4xx/5xx. Mutations
  // that participate in Promise.allSettled-based failure counting (F9) must
  // throw on !ok so the caller can distinguish fulfilled vs rejected. This
  // helper also surfaces RFC7807 problem+json bodies (Slot 2's F12 pattern).
  const throwOnError = async (res: Response): Promise<Response> => {
    if (!res.ok) {
      const body = await res.json().catch(() => null) as { code?: string; detail?: string } | null
      throw Object.assign(new Error(body?.detail ?? `HTTP ${res.status}`), {
        status: res.status,
        code: body?.code,
      })
    }
    return res
  }

  const confirmAllMutation = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/activities/${activityId}/lifts/confirm-all`, { method: 'POST' }).then(throwOnError),
    onSuccess: () => {
      invalidateAfterBulk()
      setBulkActionDone(true)
      onClose()
    },
  })

  const rejectAllMutation = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/activities/${activityId}/lifts/reject-all`, { method: 'POST' }).then(throwOnError),
    onSuccess: () => {
      invalidateAfterBulk()
      setBulkActionDone(true)
      onClose()
    },
  })

  const acknowledgeMutation = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/activities/${activityId}/lifts/acknowledge`, { method: 'POST' }).then(throwOnError),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['activity', activityId] })
    },
  })

  const confirmMutation = useMutation({
    mutationFn: (cid: string) =>
      apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${cid}/confirm`, { method: 'POST' }).then(throwOnError),
    onSuccess: () => invalidateAfterBulk(),
  })

  const unconfirmMutation = useMutation({
    mutationFn: (cid: string) =>
      apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${cid}/unconfirm`, { method: 'PATCH' }).then(throwOnError),
    onSuccess: () => invalidateAfterBulk(),
    onError: (err: Error & { code?: string }) => {
      const msg = err.code === 'ALREADY_REJECTED'
        ? 'Bereits abgelehnt — Liste aktualisiert.'
        : err.code === 'WRONG_SOURCE'
        ? 'OSM-Treffer kann nicht zurückgesetzt werden.'
        : err.message
      addToast({ type: 'error', title: msg })
      // Re-fetch so the modal reflects the actual server-side state.
      invalidateAfterBulk()
    },
  })

  const rejectMutation = useMutation({
    mutationFn: (cid: string) =>
      apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${cid}`, { method: 'DELETE' }).then(throwOnError),
    onSuccess: () => invalidateAfterBulk(),
  })

  const restoreMutation = useMutation({
    mutationFn: (cid: string) =>
      apiFetch(`/api/v1/lift-candidates/${cid}/restore`, { method: 'POST' }).then(throwOnError),
    onSuccess: () => invalidateAfterBulk(),
  })

  const recalcMutation = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/activities/${activityId}/lifts/recalculate-all`, { method: 'POST' }).then(throwOnError),
    onSuccess: () => {
      invalidateAfterBulk()
      setShowRecalcConfirm(false)
      onClose()
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

  const bulkHeuristicIds = [...selectedIds].filter(id => {
    const c = displayedCandidates.find(x => x.id === id)
    return c?.source === 'heuristic' && !c.rejected_by_user
  })

  const handleBulkConfirm = async () => {
    const results = await Promise.allSettled(
      bulkHeuristicIds.map(id => confirmMutation.mutateAsync(id)),
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
      bulkHeuristicIds.map(id => rejectMutation.mutateAsync(id)),
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
    return (
      <ModalShell onClose={handleClose} maxWidth="max-w-md">
        <div className="space-y-4">
          <div className="flex items-start justify-between gap-4">
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
              Lifte erkannt
            </h2>
            <button
              onClick={handleClose}
              aria-label="Schließen"
              className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer transition-colors"
            >
              ✕
            </button>
          </div>

          <p className="text-sm text-neutral-600 dark:text-neutral-400">
            {n === 1
              ? '1 Liftfahrt wurde erkannt — ist sie korrekt?'
              : `${n} Liftfahrten wurden erkannt — sind sie korrekt?`}
          </p>

          <ul className="text-sm text-neutral-700 dark:text-neutral-300">
            <li>
              {n === 1 ? '1 Liftfahrt' : `${n} Liftfahrten`}
              {' · '}
              {`+${totalAscent} m / −${totalDescent} m`}
            </li>
          </ul>

          <div className="flex flex-col gap-2 sm:flex-row sm:gap-2 pt-1">
            <button
              onClick={() => confirmAllMutation.mutate()}
              disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
              className="cursor-pointer flex-1 rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50 transition-colors"
            >
              Alle bestätigen
            </button>
            <button
              onClick={() => rejectAllMutation.mutate()}
              disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
              className="cursor-pointer flex-1 rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50 transition-colors"
            >
              Alle ablehnen
            </button>
            <button
              onClick={() => setMode('expanded')}
              disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
              className="cursor-pointer flex-1 rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50 transition-colors"
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
    <div className="flex flex-col h-full overflow-hidden">
      {multiSelectMode && selectedIds.size > 0 && (
        <div className="flex items-center gap-2 px-3 py-2 bg-neutral-50 dark:bg-neutral-800 border-b border-neutral-200 dark:border-neutral-700 text-xs shrink-0">
          <span className="text-neutral-600 dark:text-neutral-400 font-medium">
            {selectedIds.size === 1 ? '1 gewählt' : `${selectedIds.size} gewählt`}
          </span>
          <button
            onClick={handleBulkConfirm}
            disabled={confirmMutation.isPending || bulkHeuristicIds.length === 0}
            className="cursor-pointer rounded px-2 py-1 bg-green-600 text-white text-xs hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Bestätigen
          </button>
          <button
            onClick={handleBulkReject}
            disabled={rejectMutation.isPending || bulkHeuristicIds.length === 0}
            className="cursor-pointer rounded px-2 py-1 bg-red-600 text-white text-xs hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Ablehnen
          </button>
        </div>
      )}
      <ul className="flex-1 overflow-y-auto divide-y divide-neutral-100 dark:divide-neutral-800">
        {displayedCandidates.map((c, idx) => {
          const isHighlighted = c.id === highlightedId
          const isHeuristic = c.source === 'heuristic'
          // Check whether the activity stream covers this lift's time range with
          // at least 2 GPS points — candidates without it cannot be shown on the map.
          const hasMapTrack = streams.filter(s => {
            if (s.lat == null || s.lng == null) return false
            const t = new Date(s.time).getTime()
            return t >= new Date(c.started_at).getTime() && t <= new Date(c.ended_at).getTime()
          }).length >= 2

          return (
            <li
              key={c.id}
              ref={el => {
                if (el) listItemRefs.current.set(c.id, el)
                else listItemRefs.current.delete(c.id)
              }}
              onClick={() => handleListItemClick(c.id)}
              className={`flex flex-col gap-1.5 px-3 py-2.5 cursor-pointer transition-colors text-sm
                ${isHighlighted ? 'bg-fuchsia-50 dark:bg-fuchsia-950/30' : 'hover:bg-neutral-50 dark:hover:bg-neutral-800/50'}`}
            >
              <div className="flex items-center gap-2 flex-wrap">
                {/* Multi-select checkbox — only for heuristic candidates */}
                {multiSelectMode && isHeuristic && (
                  <input
                    type="checkbox"
                    checked={selectedIds.has(c.id)}
                    onChange={e => { e.stopPropagation(); toggleMultiSelect(c.id) }}
                    onClick={e => e.stopPropagation()}
                    aria-label={`Lift ${idx + 1} auswählen`}
                    className="rounded border-neutral-300 dark:border-neutral-600 cursor-pointer"
                  />
                )}
                <span className="font-medium text-neutral-700 dark:text-neutral-300 shrink-0">
                  L{String(idx + 1).padStart(2, '0')}
                </span>
                {sourceConfidenceBadge(c.source, c.confidence, c.rejected_by_user)}
                {!c.rejected_by_user && (
                  <span className={`text-xs font-medium shrink-0 ${
                    c.confirmed ? 'text-green-600 dark:text-green-400' : 'text-neutral-400'
                  }`}>
                    {c.confirmed ? 'bestätigt' : 'offen'}
                  </span>
                )}
              </div>
              <div className="flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400 flex-wrap">
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
              <div className="flex items-center gap-2 flex-wrap mt-0.5" onClick={e => e.stopPropagation()}>
                {/* Method B (heuristic) action buttons */}
                {isHeuristic && c.confirmed && !c.rejected_by_user && (
                  <button
                    onClick={() => unconfirmMutation.mutate(c.id)}
                    disabled={unconfirmMutation.isPending}
                    aria-label="Rückgängig machen"
                    className="cursor-pointer rounded px-2 py-0.5 border border-neutral-300 dark:border-neutral-600 text-xs text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50"
                  >
                    ↶ Rückgängig
                  </button>
                )}
                {isHeuristic && !c.confirmed && !c.rejected_by_user && (
                  <>
                    <button
                      onClick={() => confirmMutation.mutate(c.id)}
                      disabled={confirmMutation.isPending}
                      aria-label="Bestätigen"
                      className="cursor-pointer rounded px-2 py-0.5 border border-green-500 text-xs text-green-700 dark:text-green-400 hover:bg-green-50 dark:hover:bg-green-900/20 disabled:opacity-50"
                    >
                      ✓ Bestätigen
                    </button>
                    <button
                      onClick={() => rejectMutation.mutate(c.id)}
                      disabled={rejectMutation.isPending}
                      aria-label="Ablehnen"
                      className="cursor-pointer rounded px-2 py-0.5 border border-red-400 text-xs text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50"
                    >
                      ✗ Ablehnen
                    </button>
                  </>
                )}
                {isHeuristic && c.rejected_by_user && (
                  <button
                    onClick={() => restoreMutation.mutate(c.id)}
                    disabled={restoreMutation.isPending}
                    aria-label="Wiederherstellen"
                    className="cursor-pointer rounded px-2 py-0.5 border border-neutral-300 dark:border-neutral-600 text-xs text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50"
                  >
                    ↶ Wiederherstellen
                  </button>
                )}
                {/* Method A (osm) action buttons */}
                {c.source === 'osm' && c.confirmed && !c.rejected_by_user && (
                  <button
                    onClick={() => rejectMutation.mutate(c.id)}
                    disabled={rejectMutation.isPending}
                    aria-label="Ablehnen"
                    className="cursor-pointer rounded px-2 py-0.5 border border-red-400 text-xs text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50"
                  >
                    ✗ Ablehnen
                  </button>
                )}
                {c.source === 'osm' && c.rejected_by_user && (
                  <button
                    onClick={() => restoreMutation.mutate(c.id)}
                    disabled={restoreMutation.isPending}
                    aria-label="Wiederherstellen"
                    className="cursor-pointer rounded px-2 py-0.5 border border-neutral-300 dark:border-neutral-600 text-xs text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50"
                  >
                    ↶ Wiederherstellen
                  </button>
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
    </div>
  )

  const mapPanel = (
    <LiftInspectorMap
      candidates={displayedCandidates}
      streams={streams}
      highlightedCandidateId={highlightedId}
      onCandidateClick={handleCandidateClick}
    />
  )

  const bulkFooter = (
    <div className="flex items-center gap-2 flex-wrap shrink-0">
      <button
        onClick={() => confirmAllMutation.mutate()}
        disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
        className="cursor-pointer rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Alle bestätigen
      </button>
      {/* F8: expanded mode parity — compact mode has "Alle ablehnen", expanded mode must too. */}
      <button
        onClick={() => rejectAllMutation.mutate()}
        disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
        className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Alle ablehnen
      </button>
      <button
        onClick={() => {
          setMultiSelectMode(v => !v)
          if (multiSelectMode) setSelectedIds(new Set())
        }}
        className={`cursor-pointer rounded-lg border px-4 py-2 text-sm font-medium transition-colors
          ${multiSelectMode
            ? 'bg-neutral-200 dark:bg-neutral-700 border-neutral-400 dark:border-neutral-500 text-neutral-800 dark:text-neutral-200'
            : 'border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
          }`}
      >
        {multiSelectMode ? 'Multi-Auswahl beenden' : 'Multi-Auswahl'}
      </button>
      <button
        onClick={() => setShowRecalcConfirm(true)}
        disabled={recalcMutation.isPending}
        className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Neu erkennen
      </button>
      <div className="flex-1" />
      <button
        onClick={() => setMode('compact')}
        className="cursor-pointer text-sm text-neutral-500 hover:text-neutral-700 dark:text-neutral-400 dark:hover:text-neutral-200 transition-colors"
      >
        Zurück zur Übersicht
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
    <ModalShell onClose={handleClose} maxWidth="!max-w-[95vw] w-[95vw]">
      <div className="relative flex flex-col" style={{ height: 'calc(90dvh - 48px)', minHeight: '480px' }}>
        {recalcDialog}

        {/* Header */}
        <div className="flex items-center justify-between gap-3 mb-3 shrink-0">
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            Lifte prüfen
          </h2>
          <button
            onClick={handleClose}
            aria-label="Schließen"
            className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer transition-colors text-lg leading-none"
          >
            ✕
          </button>
        </div>

        {/* D.2: Pause-gap banner — visible above both desktop and mobile layouts.
            Only shown when streams are loaded and contain at least one gap > 75s. */}
        {pauseGapCount > 0 && (
          <div
            className="shrink-0 mb-2 rounded-lg border border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20 px-3 py-2 text-xs text-amber-800 dark:text-amber-200"
            role="note"
            data-testid="pause-gap-banner"
          >
            <span className="font-semibold">
              ⚠ Aufzeichnung enthält {pauseGapCount} Pause{pauseGapCount > 1 ? 'n' : ''}.
            </span>{' '}
            Die Karte zeigt die Lücken als Unterbrechung statt als gerade Linie. Lifte, die eine
            Pause-Lücke (&gt; 75 s) überspannen, werden vom Detektor{' '}
            <span className="font-semibold">nicht</span> erkannt — falls dort ein Lift erwartet
            wird, prüfe die Abschnitte vor und nach der Pause separat.
          </div>
        )}

        {isDesktop ? (
          // Desktop: 60/40 split map/list
          <>
            <div className="flex flex-1 gap-3 min-h-0">
              {/* Map — 60% */}
              <div className="flex-[3] min-h-0">
                {mapPanel}
              </div>
              {/* List — 40% */}
              <div className="flex-[2] min-h-0 border border-neutral-200 dark:border-neutral-700 rounded-lg overflow-hidden">
                {listPanel}
              </div>
            </div>
            {/* Footer */}
            <div className="mt-3 shrink-0">
              {bulkFooter}
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
            {/* Active tab content */}
            <div className="flex-1 min-h-0">
              {mobileTab === 'list'
                ? <div className="h-full border border-neutral-200 dark:border-neutral-700 rounded-lg overflow-hidden">{listPanel}</div>
                : mapPanel
              }
            </div>
            {/* Sticky footer */}
            <div className="mt-3 shrink-0">
              {bulkFooter}
            </div>
          </>
        )}
      </div>
    </ModalShell>
  )
}

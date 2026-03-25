import { useState, useMemo, useCallback } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import type { Activity, StreamPoint } from '@/api/types'
import { useToast } from '@/lib/toast'
import { useAccentColor } from '@/lib/accentColor'
import { useNavigate } from 'react-router-dom'

// ── Types ─────────────────────────────────────────────────────────────────────

type Mode = 'trim' | 'split'

interface SplitResponse {
  activity_id: string
  new_activity_id: string
}

interface TrimSplitControlsProps {
  activity: Activity
  streams: StreamPoint[]
  totalPoints: number
  trimStart: number
  trimEnd: number
  onTrimStartChange: (idx: number) => void
  onTrimEndChange: (idx: number) => void
  /** Index of split point in GPS-point array */
  splitIndex: number
  onSplitIndexChange: (idx: number) => void
  onSetFromMap: (which: 'start' | 'end' | 'split') => void
  activeMapSelect: 'start' | 'end' | 'split' | null
  /** Called after saving trim edits */
  onTrimSaved?: () => void
  /** Called when mode changes (trim/split) so parent can sync map visualization */
  onModeChange?: (mode: Mode) => void
  sectionRef?: React.RefObject<HTMLDivElement | null>
  /** initial mode from URL param */
  initialMode?: Mode
}

// ── Helpers ───────────────────────────────────────────────────────────────────

/** Format seconds as MM:SS or H:MM:SS */
function formatMMSS(totalSeconds: number): string {
  if (totalSeconds < 0) totalSeconds = 0
  const h = Math.floor(totalSeconds / 3600)
  const m = Math.floor((totalSeconds % 3600) / 60)
  const s = Math.floor(totalSeconds % 60)
  if (h > 0) {
    return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
  }
  return `${m}:${s.toString().padStart(2, '0')}`
}

/** Convert a GPS-point index to elapsed seconds from activity start */
function indexToSeconds(
  index: number,
  gpsStreams: StreamPoint[],
  startTime: number,
): number {
  if (index < 0 || index >= gpsStreams.length) return 0
  const pointTime = new Date(gpsStreams[index].time).getTime()
  return Math.max(0, (pointTime - startTime) / 1000)
}

/** Find the GPS-point index closest to a given elapsed-seconds offset */
function secondsToIndex(
  targetSeconds: number,
  gpsStreams: StreamPoint[],
  startTime: number,
): number {
  const targetMs = startTime + targetSeconds * 1000
  let bestIdx = 0
  let bestDiff = Infinity
  for (let i = 0; i < gpsStreams.length; i++) {
    const diff = Math.abs(new Date(gpsStreams[i].time).getTime() - targetMs)
    if (diff < bestDiff) {
      bestDiff = diff
      bestIdx = i
    }
  }
  return bestIdx
}

// ── Component ─────────────────────────────────────────────────────────────────

export function TrimSplitControls({
  activity,
  streams,
  totalPoints,
  trimStart,
  trimEnd,
  onTrimStartChange,
  onTrimEndChange,
  splitIndex,
  onSplitIndexChange,
  onSetFromMap,
  activeMapSelect,
  onTrimSaved,
  onModeChange,
  sectionRef,
  initialMode,
}: TrimSplitControlsProps) {
  const [mode, setMode] = useState<Mode>(initialMode ?? 'trim')
  const { accent } = useAccentColor()
  const { addToast } = useToast()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  // GPS-only streams (same filter as trackGeoJSON construction)
  const gpsStreams = useMemo(
    () => streams.filter(s => s.lat != null && s.lng != null),
    [streams],
  )

  const startTime = useMemo(
    () => gpsStreams.length > 0 ? new Date(gpsStreams[0].time).getTime() : 0,
    [gpsStreams],
  )

  const totalElapsed = useMemo(
    () => gpsStreams.length > 1
      ? (new Date(gpsStreams[gpsStreams.length - 1].time).getTime() - startTime) / 1000
      : activity.elapsed_time_s ?? 0,
    [gpsStreams, startTime, activity.elapsed_time_s],
  )

  // ── Trim time values ────────────────────────────────────────────────────────
  const trimStartSeconds = useMemo(
    () => indexToSeconds(trimStart, gpsStreams, startTime),
    [trimStart, gpsStreams, startTime],
  )
  const trimEndSeconds = useMemo(
    () => indexToSeconds(trimEnd, gpsStreams, startTime),
    [trimEnd, gpsStreams, startTime],
  )

  // ── Split time values ───────────────────────────────────────────────────────
  const splitSeconds = useMemo(
    () => indexToSeconds(splitIndex, gpsStreams, startTime),
    [splitIndex, gpsStreams, startTime],
  )

  const part1Seconds = Math.round(splitSeconds)
  const part2Seconds = Math.round(Math.max(0, totalElapsed - splitSeconds))
  const splitValid = splitSeconds > 30 && part2Seconds > 30 && splitIndex > 0 && splitIndex < totalPoints - 1
  const splitPercent = totalElapsed > 0 ? Math.min(100, Math.max(0, (splitSeconds / totalElapsed) * 100)) : 0

  // ── Fine-adjust helpers ─────────────────────────────────────────────────────
  const nudge = useCallback(
    (
      currentIndex: number,
      direction: -1 | 1,
      setter: (idx: number) => void,
      minIdx = 0,
      maxIdx = totalPoints - 1,
    ) => {
      // Move by ~1 second: find the point that is ~1s away
      if (gpsStreams.length === 0) return
      const currentTime = new Date(gpsStreams[currentIndex]?.time ?? 0).getTime()
      const targetTime = currentTime + direction * 1000
      let bestIdx = currentIndex + direction
      if (bestIdx < minIdx) bestIdx = minIdx
      if (bestIdx > maxIdx) bestIdx = maxIdx

      // Search nearby for closest to 1s offset
      let best = bestIdx
      let bestDiff = Infinity
      const searchStart = Math.max(minIdx, currentIndex - 30)
      const searchEnd = Math.min(maxIdx, currentIndex + 30)
      for (let i = searchStart; i <= searchEnd; i++) {
        if (i === currentIndex) continue
        const diff = Math.abs(new Date(gpsStreams[i].time).getTime() - targetTime)
        if (diff < bestDiff && ((direction > 0 && i > currentIndex) || (direction < 0 && i < currentIndex))) {
          bestDiff = diff
          best = i
        }
      }
      setter(best)
    },
    [gpsStreams, totalPoints],
  )

  // ── Split mutation ──────────────────────────────────────────────────────────
  const [confirmSplit, setConfirmSplit] = useState(false)
  const splitMutation = useMutation({
    mutationFn: async () => {
      const splitAt = new Date(startTime + splitSeconds * 1000)
      return apiClient<SplitResponse>(`/activities/${activity.id}/split`, {
        method: 'POST',
        body: { split_at: splitAt.toISOString() },
      })
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      addToast({
        type: 'success',
        title: 'Aktivitat aufgeteilt',
        message: 'Zwei Aktivitaten wurden erstellt.',
      })
      navigate(`/activities/${result.new_activity_id}`)
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Aufteilung fehlgeschlagen', message: String(err), duration: 0 })
      setConfirmSplit(false)
    },
  })

  // ── Trim state ──────────────────────────────────────────────────────────────
  const maxIdx = totalPoints - 1
  const trimActive = trimStart > 0 || trimEnd < maxIdx
  const keptCount = trimEnd - trimStart + 1
  const keptPct = totalPoints > 0 ? ((keptCount / totalPoints) * 100).toFixed(1) : '0.0'

  // ── Render ──────────────────────────────────────────────────────────────────
  return (
    <div ref={sectionRef} className="space-y-4">
      {/* Mode toggle */}
      <div className="flex items-center gap-1 p-0.5 rounded-lg bg-neutral-100 dark:bg-neutral-800 w-fit">
        <button
          onClick={() => { setMode('trim'); onModeChange?.('trim') }}
          className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors cursor-pointer ${
            mode === 'trim'
              ? 'bg-white dark:bg-neutral-700 text-neutral-900 dark:text-neutral-100 shadow-sm'
              : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-300'
          }`}
        >
          <span className="flex items-center gap-1.5">
            <ScissorsIcon className="w-3.5 h-3.5" />
            Trim / Crop
          </span>
        </button>
        <button
          onClick={() => { setMode('split'); onModeChange?.('split') }}
          className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors cursor-pointer ${
            mode === 'split'
              ? 'bg-white dark:bg-neutral-700 text-neutral-900 dark:text-neutral-100 shadow-sm'
              : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-300'
          }`}
        >
          <span className="flex items-center gap-1.5">
            <SplitIcon className="w-3.5 h-3.5" />
            Aufteilen
          </span>
        </button>
      </div>

      {/* ── TRIM MODE ────────────────────────────────────────────────────── */}
      {mode === 'trim' && (
        <div className="space-y-3 p-4 border border-neutral-200 dark:border-neutral-700 rounded-lg bg-neutral-50 dark:bg-neutral-900">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Anfang und Ende zuschneiden
            </h3>
            {trimActive && (
              <span className="text-xs text-amber-600 dark:text-amber-400 font-medium">
                Anfang/Ende abgeschnitten
              </span>
            )}
          </div>

          {/* Start slider */}
          <div className="space-y-1">
            <div className="flex items-center justify-between">
              <label className="text-xs text-neutral-500 dark:text-neutral-400">
                Start: <span className="font-mono font-medium text-neutral-700 dark:text-neutral-300">{formatMMSS(trimStartSeconds)}</span>
              </label>
              <button
                onClick={() => onSetFromMap('start')}
                className={`text-xs px-2 py-0.5 rounded border transition-colors cursor-pointer ${
                  activeMapSelect === 'start'
                    ? 'bg-blue-100 border-blue-400 text-blue-700 dark:bg-blue-900/40 dark:border-blue-500 dark:text-blue-300'
                    : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {activeMapSelect === 'start' ? 'Auf Karte klicken...' : 'Auf Karte setzen'}
              </button>
            </div>
            <div className="flex items-center gap-1.5">
              <NudgeButton
                direction="left"
                onClick={() => nudge(trimStart, -1, onTrimStartChange, 0, trimEnd)}
                disabled={trimStart <= 0}
              />
              <input
                type="range"
                min={0}
                max={maxIdx}
                value={trimStart}
                onChange={e => {
                  const v = Number(e.target.value)
                  if (v <= trimEnd) onTrimStartChange(v)
                }}
                className="flex-1 accent-green-500"
              />
              <NudgeButton
                direction="right"
                onClick={() => nudge(trimStart, 1, onTrimStartChange, 0, trimEnd)}
                disabled={trimStart >= trimEnd}
              />
            </div>
          </div>

          {/* End slider */}
          <div className="space-y-1">
            <div className="flex items-center justify-between">
              <label className="text-xs text-neutral-500 dark:text-neutral-400">
                Ende: <span className="font-mono font-medium text-neutral-700 dark:text-neutral-300">{formatMMSS(trimEndSeconds)}</span>
              </label>
              <button
                onClick={() => onSetFromMap('end')}
                className={`text-xs px-2 py-0.5 rounded border transition-colors cursor-pointer ${
                  activeMapSelect === 'end'
                    ? 'bg-blue-100 border-blue-400 text-blue-700 dark:bg-blue-900/40 dark:border-blue-500 dark:text-blue-300'
                    : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {activeMapSelect === 'end' ? 'Auf Karte klicken...' : 'Auf Karte setzen'}
              </button>
            </div>
            <div className="flex items-center gap-1.5">
              <NudgeButton
                direction="left"
                onClick={() => nudge(trimEnd, -1, onTrimEndChange, trimStart, maxIdx)}
                disabled={trimEnd <= trimStart}
              />
              <input
                type="range"
                min={0}
                max={maxIdx}
                value={trimEnd}
                onChange={e => {
                  const v = Number(e.target.value)
                  if (v >= trimStart) onTrimEndChange(v)
                }}
                className="flex-1 accent-red-500"
              />
              <NudgeButton
                direction="right"
                onClick={() => nudge(trimEnd, 1, onTrimEndChange, trimStart, maxIdx)}
                disabled={trimEnd >= maxIdx}
              />
            </div>
          </div>

          {/* Summary */}
          <div className="flex items-center justify-between text-xs text-neutral-500 dark:text-neutral-400 pt-1 border-t border-neutral-200 dark:border-neutral-700">
            <span>
              {keptCount} / {totalPoints} Punkte behalten ({keptPct}%)
              {trimActive && (
                <span className="ml-2 font-mono text-neutral-600 dark:text-neutral-300">
                  {formatMMSS(trimStartSeconds)} – {formatMMSS(trimEndSeconds)}
                </span>
              )}
            </span>
            {trimActive && (
              <button
                onClick={() => {
                  onTrimStartChange(0)
                  onTrimEndChange(maxIdx)
                }}
                className="text-xs text-neutral-400 dark:text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 transition-colors underline cursor-pointer"
              >
                Zurucksetzen
              </button>
            )}
          </div>

          {/* Save trim */}
          {trimActive && onTrimSaved && (
            <div className="flex justify-end pt-1">
              <SaveTrimButton
                activityId={activity.id}
                trimStart={trimStart}
                trimEnd={trimEnd}
                maxIdx={maxIdx}
                gpsStreams={gpsStreams}
                onSaved={onTrimSaved}
              />
            </div>
          )}
        </div>
      )}

      {/* ── SPLIT MODE ───────────────────────────────────────────────────── */}
      {mode === 'split' && (
        <div className="space-y-3 p-4 border border-neutral-200 dark:border-neutral-700 rounded-lg bg-neutral-50 dark:bg-neutral-900">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Aktivitat an einem Punkt aufteilen
            </h3>
            <span className="text-xs text-neutral-400 dark:text-neutral-500">
              Gesamtdauer: <span className="font-mono text-neutral-600 dark:text-neutral-300">{formatMMSS(totalElapsed)}</span>
            </span>
          </div>

          {/* Split slider */}
          <div className="space-y-1">
            <div className="flex items-center justify-between">
              <label className="text-xs text-neutral-500 dark:text-neutral-400">
                Teilungspunkt: <span className="font-mono font-medium text-neutral-700 dark:text-neutral-300">{formatMMSS(splitSeconds)}</span>
              </label>
              <button
                onClick={() => onSetFromMap('split')}
                className={`text-xs px-2 py-0.5 rounded border transition-colors cursor-pointer ${
                  activeMapSelect === 'split'
                    ? 'bg-blue-100 border-blue-400 text-blue-700 dark:bg-blue-900/40 dark:border-blue-500 dark:text-blue-300'
                    : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {activeMapSelect === 'split' ? 'Auf Karte klicken...' : 'Auf Karte setzen'}
              </button>
            </div>
            <div className="flex items-center gap-1.5">
              <NudgeButton
                direction="left"
                onClick={() => nudge(splitIndex, -1, onSplitIndexChange, 1, totalPoints - 2)}
                disabled={splitIndex <= 1}
              />
              <input
                type="range"
                min={1}
                max={totalPoints - 2}
                value={splitIndex}
                onChange={e => {
                  onSplitIndexChange(Number(e.target.value))
                  setConfirmSplit(false)
                }}
                className="flex-1"
                style={{ accentColor: accent.shade500 }}
              />
              <NudgeButton
                direction="right"
                onClick={() => nudge(splitIndex, 1, onSplitIndexChange, 1, totalPoints - 2)}
                disabled={splitIndex >= totalPoints - 2}
              />
            </div>
          </div>

          {/* Visual split bar */}
          {splitIndex > 0 && (
            <div className="space-y-2">
              <div className="relative h-6 rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700">
                <div
                  className="absolute inset-y-0 left-0 flex items-center justify-center text-[10px] font-medium text-white"
                  style={{ width: `${Math.max(splitPercent, 8)}%`, backgroundColor: accent.shade500 }}
                >
                  {part1Seconds > 0 ? formatMMSS(part1Seconds) : ''}
                </div>
                <div
                  className="absolute inset-y-0 right-0 flex items-center justify-center text-[10px] font-medium bg-neutral-200 dark:bg-neutral-700 text-neutral-700 dark:text-neutral-300"
                  style={{ width: `${Math.max(100 - splitPercent, 8)}%` }}
                >
                  {part2Seconds > 0 ? formatMMSS(part2Seconds) : ''}
                </div>
              </div>

              <div className="grid grid-cols-2 gap-2">
                <div className="rounded-lg bg-white dark:bg-neutral-800/50 px-3 py-2 text-center">
                  <p className="text-[10px] uppercase tracking-wider text-neutral-400 dark:text-neutral-500">Teil 1</p>
                  <p className="text-sm font-semibold text-neutral-900 dark:text-neutral-50 mt-0.5 font-mono">
                    {part1Seconds > 0 ? formatMMSS(part1Seconds) : '\u2014'}
                  </p>
                </div>
                <div className="rounded-lg bg-white dark:bg-neutral-800/50 px-3 py-2 text-center">
                  <p className="text-[10px] uppercase tracking-wider text-neutral-400 dark:text-neutral-500">Teil 2</p>
                  <p className={`text-sm font-semibold mt-0.5 font-mono ${part2Seconds <= 0 ? 'text-red-500' : 'text-neutral-900 dark:text-neutral-50'}`}>
                    {part2Seconds > 0 ? formatMMSS(part2Seconds) : 'Ungultig'}
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* Confirm + Action */}
          <div className="flex items-center gap-3 pt-1">
            {!confirmSplit ? (
              <button
                onClick={() => setConfirmSplit(true)}
                disabled={!splitValid}
                className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                           border border-neutral-200 dark:border-neutral-700
                           text-neutral-700 dark:text-neutral-300
                           hover:bg-neutral-100 dark:hover:bg-neutral-800
                           disabled:opacity-50 disabled:cursor-not-allowed
                           transition-colors cursor-pointer"
              >
                <SplitIcon className="w-4 h-4" />
                Aufteilen
              </button>
            ) : (
              <div className="flex items-center gap-3">
                <div className="flex items-start gap-2 bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 rounded-lg p-2.5">
                  <svg className="w-4 h-4 text-amber-500 flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
                  </svg>
                  <p className="text-xs text-amber-700 dark:text-amber-400 leading-relaxed">
                    Diese Aktion erstellt eine zweite Aktivitat ab dem Teilungspunkt.
                  </p>
                </div>
                <button
                  onClick={() => splitMutation.mutate()}
                  disabled={splitMutation.isPending}
                  className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium text-white
                             disabled:opacity-50 disabled:cursor-not-allowed
                             hover:opacity-90 transition-colors cursor-pointer whitespace-nowrap"
                  style={{ backgroundColor: accent.shade500 }}
                >
                  {splitMutation.isPending ? 'Wird aufgeteilt\u2026' : 'Jetzt aufteilen'}
                </button>
                <button
                  onClick={() => setConfirmSplit(false)}
                  disabled={splitMutation.isPending}
                  className="px-3 py-2 rounded-lg text-sm border border-neutral-200 dark:border-neutral-700
                             text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800
                             disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
                >
                  Abbrechen
                </button>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

// ── SaveTrimButton (isolated to avoid pulling sidecar logic into parent) ─────

function SaveTrimButton({
  activityId,
  trimStart,
  trimEnd,
  maxIdx,
  gpsStreams,
  onSaved,
}: {
  activityId: string
  trimStart: number
  trimEnd: number
  maxIdx: number
  gpsStreams: StreamPoint[]
  onSaved: () => void
}) {
  const { addToast } = useToast()
  const [saving, setSaving] = useState(false)

  const handleSave = async () => {
    setSaving(true)
    try {
      // Read existing sidecar
      const sidecar = await apiClient<{
        version: number
        activity_id: string
        created_at: string
        updated_at: string
        edits: { type: string; timestamp?: string; timestamps?: string[]; method?: string; from_utc?: string; to_utc?: string }[]
      }>(`/activities/${activityId}/edits`)

      const now = new Date().toISOString()
      const newEdits = [
        ...(sidecar?.edits.filter(e => e.type !== 'trim_start' && e.type !== 'trim_end') ?? []),
      ]

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

      await apiClient(`/activities/${activityId}/edits`, {
        method: 'PUT',
        body: {
          version: 1,
          activity_id: activityId,
          created_at: sidecar?.created_at ?? now,
          updated_at: now,
          edits: newEdits,
        },
      })

      addToast({ type: 'success', title: 'Trim gespeichert' })
      onSaved()
    } catch {
      addToast({ type: 'error', title: 'Trim-Speichern fehlgeschlagen', duration: 0 })
    } finally {
      setSaving(false)
    }
  }

  return (
    <button
      onClick={handleSave}
      disabled={saving}
      className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium
                 border border-amber-300 dark:border-amber-700
                 text-amber-700 dark:text-amber-400
                 hover:bg-amber-50 dark:hover:bg-amber-900/20
                 disabled:opacity-50 disabled:cursor-not-allowed
                 transition-colors cursor-pointer"
    >
      <SaveIcon className="w-4 h-4" />
      {saving ? 'Speichere...' : 'Trim speichern'}
    </button>
  )
}

// ── NudgeButton ─────────────────────────────────────────────────────────────

function NudgeButton({
  direction,
  onClick,
  disabled,
}: {
  direction: 'left' | 'right'
  onClick: () => void
  disabled?: boolean
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded-md
                 border border-neutral-200 dark:border-neutral-700
                 text-neutral-500 dark:text-neutral-400
                 hover:bg-neutral-100 dark:hover:bg-neutral-800
                 disabled:opacity-30 disabled:cursor-not-allowed
                 transition-colors cursor-pointer text-xs font-bold"
      title={direction === 'left' ? '-1s' : '+1s'}
    >
      {direction === 'left' ? '\u2039' : '\u203A'}
    </button>
  )
}

// ── Icons ─────────────────────────────────────────────────────────────────────

function ScissorsIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <circle cx="6" cy="6" r="3" />
      <path d="M8.12 8.12 12 12" />
      <path d="M20 4 8.12 15.88" />
      <circle cx="6" cy="18" r="3" />
      <path d="M14.8 14.8 20 20" />
    </svg>
  )
}

function SplitIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M16 3h5v5" />
      <path d="M8 3H3v5" />
      <path d="M12 22v-8.3a4 4 0 0 0-1.172-2.872L3 3" />
      <path d="m15 9 6-6" />
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

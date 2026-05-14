import { useState, useEffect } from 'react'
import { Loader2 } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  useLiftDetectionConfig,
  usePutLiftDetectionConfig,
  useResetLiftDetectionConfig,
} from '@/hooks/useLiftDetectionConfig'
import type { LiftDetectionConfigResponse } from '@/hooks/useLiftDetectionConfig'
import { useToast } from '@/lib/toast'
import { apiFetch } from '@/lib/auth'
import { ApiError, throwOnApiResponse } from '@/api/client'
import { fetchActiveJobs } from '@/api/jobs'

interface LiftDetectionHeaderProps {
  activityId: string
  /** True while a lift_recompute background job is running for this activity. */
  jobRunning: boolean
  /** Called when recalculate-all starts or an existing recompute job is already running. */
  onJobStarted: () => void
  compact?: boolean
  osmSnapAvailable?: boolean
}

interface LocalSettings {
  osm: boolean
  heuristic: boolean
}

type DetectionUpdate =
  | { kind: 'put'; settings: LocalSettings }
  | { kind: 'reset' }

interface RecomputeResult {
  status: number
  startsJob: boolean
  alreadyRunning?: boolean
}

function deriveLocalSettings(data: LiftDetectionConfigResponse): LocalSettings {
  return {
    osm: data.override.osm ?? data.user_setting.osm,
    heuristic: data.override.heuristic ?? data.user_setting.heuristic,
  }
}

function hasActiveOverride(data: LiftDetectionConfigResponse): boolean {
  return data.override.osm !== null || data.override.heuristic !== null
}

function startsBackgroundRecompute(data: LiftDetectionConfigResponse, settings: LocalSettings): boolean {
  return data.master_enabled && (settings.osm || settings.heuristic)
}

function apiErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    const detail = err.json?.detail ?? err.json?.message
    return typeof detail === 'string' ? detail : (err.body || `Fehler ${err.status}`)
  }
  return err instanceof Error ? err.message : 'Unbekannter Fehler'
}

interface ToggleRowProps {
  label: string
  checked: boolean
  disabled: boolean
  onChange: (v: boolean) => void
  testId?: string
}

function ToggleRow({ label, checked, disabled, onChange, testId }: ToggleRowProps) {
  return (
    <label
      className={`flex items-center gap-2 select-none ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
      data-testid={testId}
    >
      <button
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={`relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors
          focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT
          disabled:cursor-not-allowed
          ${checked ? 'bg-accent-DEFAULT' : 'bg-neutral-300 dark:bg-neutral-600'}`}
        type="button"
      >
        <span
          className={`pointer-events-none inline-block h-4 w-4 rounded-full bg-white shadow-sm ring-0 transition-transform
            ${checked ? 'translate-x-4' : 'translate-x-0'}`}
          aria-hidden="true"
        />
      </button>
      <span className="text-xs text-neutral-700 dark:text-neutral-300">{label}</span>
    </label>
  )
}

async function hasRunningLiftRecompute(activityId: string): Promise<boolean> {
  const data = await fetchActiveJobs()
  return data.jobs.some(
    j => j.job_type === 'lift_recompute' && j.entity_id === activityId && j.status === 'running',
  )
}

export default function LiftDetectionHeader({ activityId, jobRunning, onJobStarted, compact = false, osmSnapAvailable = true }: LiftDetectionHeaderProps) {
  const { addToast } = useToast()
  const qc = useQueryClient()
  const { data, isLoading } = useLiftDetectionConfig(activityId)
  const putMutation = usePutLiftDetectionConfig(activityId)
  const resetMutation = useResetLiftDetectionConfig(activityId)

  const [local, setLocal] = useState<LocalSettings | null>(null)

  // Sync local state when query data arrives or changes (e.g. after reset).
  useEffect(() => {
    if (data) {
      const next = deriveLocalSettings(data)
      if (!osmSnapAvailable) next.osm = false
      setLocal(next)
    }
  }, [data, osmSnapAvailable])

  const recomputeMutation = useMutation({
    mutationFn: async (update: DetectionUpdate): Promise<RecomputeResult> => {
      if (!data) throw new Error('Lift-Erkennungskonfiguration fehlt')
      if (await hasRunningLiftRecompute(activityId)) {
        return { status: 409, startsJob: true, alreadyRunning: true }
      }
      const startsJob = update.kind === 'reset'
        ? startsBackgroundRecompute(data, { osm: data.user_setting.osm, heuristic: data.user_setting.heuristic })
        : startsBackgroundRecompute(data, update.settings)
      if (update.kind === 'reset') {
        await resetMutation.mutateAsync()
      } else {
        await putMutation.mutateAsync({ osm: update.settings.osm, heuristic: update.settings.heuristic })
      }
      const res = await apiFetch(`/api/v1/activities/${activityId}/lifts/recalculate-all`, {
        method: 'POST',
      })
      await throwOnApiResponse(res)
      return { status: res.status, startsJob }
    },
    onSuccess: async ({ status, startsJob, alreadyRunning }) => {
      if (alreadyRunning) {
        addToast({ type: 'info', title: 'Job läuft bereits — bitte warten' })
        onJobStarted()
        return
      }
      // An accepted recompute can change lift candidates and activity totals.
      qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] })
      qc.invalidateQueries({ queryKey: ['lift-candidates-inspector', activityId] })
      qc.invalidateQueries({ queryKey: ['activity', activityId] })
      if (status >= 200 && status < 300) {
        if (startsJob) onJobStarted()
      }
    },
    onError: (err) => {
      if (err instanceof ApiError && err.status === 409) {
        addToast({ type: 'info', title: 'Job läuft bereits — bitte warten' })
        onJobStarted()
        return
      }
      addToast({ type: 'error', title: apiErrorMessage(err) })
    },
  })

  if (isLoading || !data || !local) {
    return (
      <div className={`${compact ? '' : 'border-b border-neutral-200 dark:border-neutral-800 px-4 py-2'} flex items-center gap-2 text-sm text-neutral-500 dark:text-neutral-400 shrink-0`}>
        <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" />
        Erkennungsmodus wird geladen …
      </div>
    )
  }

  const masterOff = !data.master_enabled
  const override = hasActiveOverride(data)
  const isApplying = recomputeMutation.isPending
  const disablingAll = isApplying && !jobRunning && local && !local.osm && !local.heuristic
  const allDisabled = masterOff || jobRunning || isApplying

  const handleToggle = (patch: Partial<LocalSettings>) => {
    if (!local) return
    const next = { ...local, ...patch }
    setLocal(next)
    recomputeMutation.mutate({ kind: 'put', settings: next })
  }

  const handleReset = () => {
    recomputeMutation.mutate({ kind: 'reset' })
  }

  return (
    <div
      className={[
        compact
          ? 'flex flex-wrap items-center gap-x-3 gap-y-1 shrink-0'
          : 'flex flex-col gap-2 border-b border-neutral-200 dark:border-neutral-800 px-4 py-2.5 shrink-0',
      ].join(' ')}
      data-testid="lift-detection-header"
    >
      <div className={`${compact ? 'contents' : 'flex items-center justify-between gap-2'}`}>
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">
          {compact ? 'Erkennung' : 'Erkennungsmodus'}
        </span>
        {compact && (
          <div className="flex flex-wrap items-center gap-3">
            {osmSnapAvailable && (
              <ToggleRow
                label="OSM-Snap"
                checked={local.osm}
                disabled={allDisabled}
                onChange={osm => handleToggle({ osm })}
                testId="toggle-osm"
              />
            )}
            <ToggleRow
              label="Heuristik"
              checked={local.heuristic}
              disabled={allDisabled}
              onChange={heuristic => handleToggle({ heuristic })}
              testId="toggle-heuristic"
            />
          </div>
        )}
        {override && (
          <button
            type="button"
            onClick={handleReset}
            disabled={allDisabled}
            className="inline-flex items-center rounded-full bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 text-xs font-medium px-2 py-0.5 disabled:opacity-50 disabled:cursor-not-allowed"
            data-testid="override-badge"
            title="Aktivitätsspezifische Einstellung zurücksetzen"
          >
            Override gesetzt
          </button>
        )}
        {(jobRunning || isApplying) && (
          <span className="inline-flex items-center gap-1 text-xs text-neutral-500 dark:text-neutral-400">
            <Loader2 className="w-3.5 h-3.5 animate-spin" aria-hidden="true" />
            {disablingAll ? 'Wird deaktiviert …' : 'Wird berechnet …'}
          </span>
        )}
      </div>

      {!compact && (
        <div className="flex flex-wrap items-center gap-4">
          {osmSnapAvailable && (
            <ToggleRow
              label="OSM-Snap"
              checked={local.osm}
              disabled={allDisabled}
              onChange={osm => handleToggle({ osm })}
              testId="toggle-osm"
            />
          )}
          <ToggleRow
            label="Heuristik"
            checked={local.heuristic}
            disabled={allDisabled}
            onChange={heuristic => handleToggle({ heuristic })}
            testId="toggle-heuristic"
          />
        </div>
      )}

      {masterOff && (
        <p className="text-xs text-amber-700 dark:text-amber-400" data-testid="master-off-notice">
          Globale Lift-Erkennung deaktiviert, siehe Einstellungen
        </p>
      )}

    </div>
  )
}

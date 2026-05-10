import { useState, useEffect } from 'react'
import { Loader2 } from 'lucide-react'
import {
  useLiftDetectionConfig,
  usePutLiftDetectionConfig,
  useResetLiftDetectionConfig,
} from '@/hooks/useLiftDetectionConfig'
import type { LiftDetectionConfigResponse } from '@/hooks/useLiftDetectionConfig'
import { useToast } from '@/lib/toast'
import { apiFetch } from '@/lib/auth'
import { ApiError } from '@/api/client'

interface LiftDetectionHeaderProps {
  activityId: string
  /** True while a lift_recompute background job is running for this activity. */
  jobRunning: boolean
  /** Called after Apply triggers recalculate-all successfully (202). */
  onJobStarted: () => void
}

interface LocalSettings {
  osm: boolean
  heuristic: boolean
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
      <span className="text-sm text-neutral-700 dark:text-neutral-300">{label}</span>
    </label>
  )
}

export default function LiftDetectionHeader({ activityId, jobRunning, onJobStarted }: LiftDetectionHeaderProps) {
  const { addToast } = useToast()
  const { data, isLoading } = useLiftDetectionConfig(activityId)
  const putMutation = usePutLiftDetectionConfig(activityId)
  const resetMutation = useResetLiftDetectionConfig(activityId)

  const [local, setLocal] = useState<LocalSettings | null>(null)
  const [isApplying, setIsApplying] = useState(false)

  // Sync local state when query data arrives or changes (e.g. after reset).
  useEffect(() => {
    if (data) {
      setLocal(deriveLocalSettings(data))
    }
  }, [data])

  if (isLoading || !data || !local) {
    return (
      <div className="flex items-center gap-2 px-4 py-2 border-b border-neutral-200 dark:border-neutral-800 text-sm text-neutral-500 dark:text-neutral-400 shrink-0">
        <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" />
        Erkennungsmodus wird geladen …
      </div>
    )
  }

  const masterOff = !data.master_enabled
  const override = hasActiveOverride(data)
  const allDisabled = masterOff || jobRunning || isApplying

  const handleApply = async () => {
    setIsApplying(true)
    try {
      await putMutation.mutateAsync({ osm: local.osm, heuristic: local.heuristic })
      // Now trigger recalculate-all. Use apiFetch directly to inspect the response
      // status — recalcMutation in the parent already uses throwOnError, but here
      // we need to distinguish 409 from 202 without closing the modal.
      const res = await apiFetch(`/api/v1/activities/${activityId}/lifts/recalculate-all`, {
        method: 'POST',
      })
      if (res.status === 409) {
        addToast({
          type: 'info',
          title: 'Job läuft bereits — bitte warten',
        })
        // The job is running — activate polling state via onJobStarted so the
        // parent can show a spinner and poll for completion.
        onJobStarted()
      } else if (res.ok) {
        onJobStarted()
      } else {
        const body = await res.json().catch(() => ({})) as Record<string, unknown>
        const msg = typeof body.message === 'string' ? body.message : `Fehler ${res.status}`
        addToast({ type: 'error', title: msg })
      }
    } catch (err) {
      const msg = err instanceof ApiError ? err.body : (err instanceof Error ? err.message : 'Unbekannter Fehler')
      addToast({ type: 'error', title: msg })
    } finally {
      setIsApplying(false)
    }
  }

  const handleReset = async () => {
    try {
      await resetMutation.mutateAsync()
      addToast({ type: 'success', title: 'Aktivitätsspezifische Einstellung zurückgesetzt' })
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Fehler beim Zurücksetzen'
      addToast({ type: 'error', title: msg })
    }
  }

  return (
    <div
      className="flex flex-col gap-2 border-b border-neutral-200 dark:border-neutral-800 px-4 py-2.5 shrink-0"
      data-testid="lift-detection-header"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          Erkennungsmodus
        </span>
        {override && (
          <span
            className="inline-flex items-center rounded-full bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 text-xs font-medium px-2 py-0.5"
            data-testid="override-badge"
          >
            Aktivität nutzt eigene Einstellungen
          </span>
        )}
        {jobRunning && (
          <span className="inline-flex items-center gap-1 text-xs text-neutral-500 dark:text-neutral-400">
            <Loader2 className="w-3.5 h-3.5 animate-spin" aria-hidden="true" />
            Lift-Detection läuft …
          </span>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-4">
        <ToggleRow
          label="OSM-Snap"
          checked={local.osm}
          disabled={allDisabled}
          onChange={v => setLocal(prev => prev ? { ...prev, osm: v } : prev)}
          testId="toggle-osm"
        />
        <ToggleRow
          label="Heuristik"
          checked={local.heuristic}
          disabled={allDisabled}
          onChange={v => setLocal(prev => prev ? { ...prev, heuristic: v } : prev)}
          testId="toggle-heuristic"
        />
      </div>

      {masterOff && (
        <p className="text-xs text-amber-700 dark:text-amber-400" data-testid="master-off-notice">
          Globale Lift-Erkennung deaktiviert — siehe Einstellungen
        </p>
      )}

      <div className="flex gap-2">
        <button
          onClick={handleApply}
          disabled={allDisabled}
          className="cursor-pointer rounded-md bg-accent-DEFAULT px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          data-testid="apply-button"
        >
          {isApplying ? 'Wird angewendet …' : 'Anwenden'}
        </button>
        <button
          onClick={handleReset}
          disabled={!override || allDisabled}
          className="cursor-pointer rounded-md border border-neutral-300 dark:border-neutral-600 px-3 py-1.5 text-xs font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          data-testid="reset-button"
        >
          Zurücksetzen
        </button>
      </div>
    </div>
  )
}

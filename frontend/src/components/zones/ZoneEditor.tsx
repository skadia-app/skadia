/**
 * ZoneEditor — per-sport card with sections for HR / Power / Speed zones.
 *
 * Shows source_type badge, threshold input, editable ZoneTable, and Save button.
 * Full-state semantics: fetches current state, user edits, sends complete state on save.
 */

import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/lib/toast'
import { updateTrainingZones, ZONE_TYPES } from '@/api/trainingZones'
import type { SportZones, ZoneDetail, ZoneType, TrainingZoneUpdate } from '@/api/trainingZones'
import ZoneTable from './ZoneTable'
import type { ZoneRow } from './ZoneTable'

// ── Source badge ──────────────────────────────────────────────────────────────

function sourceLabel(sourceType: string): string {
  const lower = sourceType.toLowerCase()
  if (lower.includes('fit') || lower.includes('garmin')) return 'Device Import'
  if (lower.includes('manual')) return 'Manual'
  return sourceType
}

function SourceBadge({ sourceType }: { sourceType: string }) {
  const label = sourceLabel(sourceType)
  const isDevice = label === 'Device Import'
  return (
    <span
      className={[
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
        isDevice
          ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
          : 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
      ].join(' ')}
    >
      {label}
    </span>
  )
}

// ── Zone section helpers ──────────────────────────────────────────────────────

function zoneDetailToRows(detail: ZoneDetail): ZoneRow[] {
  return detail.zone_names.map((name, i) => ({
    name,
    high: detail.zone_highs[i] ?? 0,
  }))
}

function rowsToNamesHighs(rows: ZoneRow[]): { zone_names: string[]; zone_highs: number[] } {
  // Last zone has an implicit infinite upper bound; store it as a large sentinel
  const highs = rows.map((r, i) => (i === rows.length - 1 ? 9999 : r.high))
  return { zone_names: rows.map(r => r.name), zone_highs: highs }
}

// ── Shared state type ─────────────────────────────────────────────────────────

interface ZoneState {
  threshold: number | null
  rows: ZoneRow[]
}

// ── Shared save hook ──────────────────────────────────────────────────────────

function useZoneSave(buildPayload: (state: ZoneState) => TrainingZoneUpdate, successTitle: string, errorTitle: string) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  return useMutation({
    mutationFn: (state: ZoneState) => updateTrainingZones(buildPayload(state)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['training-zones'] })
      addToast({ type: 'success', title: successTitle })
    },
    onError: () => {
      addToast({ type: 'error', title: errorTitle, duration: 0 })
    },
  })
}

// ── ZoneSection config ────────────────────────────────────────────────────────

interface ZoneSectionConfig {
  zoneType: ZoneType
  heading: string
  thresholdLabel: string
  thresholdPlaceholder: string
  thresholdMin: number
  thresholdMax: number
  thresholdStep?: number
  unit: string
  convertMps?: boolean
  saveLabel: string
  successTitle: string
  errorTitle: string
  /** Read threshold from ZoneDetail into display units */
  readThreshold: (d: ZoneDetail) => number | null
  /** Build the API payload from sport + display-unit state */
  buildPayload: (sport: number, state: ZoneState) => TrainingZoneUpdate
  /** True when save should be disabled (beyond isPending) */
  isSaveDisabled: (state: ZoneState) => boolean
}

// ── ZoneSection ───────────────────────────────────────────────────────────────

function ZoneSection({
  sport,
  detail,
  disabled,
  config,
}: {
  sport: number
  detail: ZoneDetail
  disabled: boolean
  config: ZoneSectionConfig
}) {
  const [state, setState] = useState<ZoneState>({
    threshold: config.readThreshold(detail),
    rows: zoneDetailToRows(detail),
  })

  useEffect(() => {
    setState({
      threshold: config.readThreshold(detail),
      rows: zoneDetailToRows(detail),
    })
  }, [detail]) // eslint-disable-line react-hooks/exhaustive-deps

  const mutation = useZoneSave(
    s => config.buildPayload(sport, s),
    config.successTitle,
    config.errorTitle,
  )

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{config.heading}</h4>
        <SourceBadge sourceType={detail.source_type} />
      </div>

      <div className="flex items-center gap-3">
        <label className="text-sm text-neutral-600 dark:text-neutral-400 shrink-0">
          {config.thresholdLabel} ({config.unit})
        </label>
        <input
          type="number"
          min={config.thresholdMin}
          max={config.thresholdMax}
          step={config.thresholdStep}
          value={state.threshold ?? ''}
          onChange={e =>
            setState(s => ({
              ...s,
              threshold: e.target.value
                ? config.thresholdStep != null && config.thresholdStep < 1
                  ? parseFloat(e.target.value)
                  : parseInt(e.target.value)
                : null,
            }))
          }
          disabled={disabled}
          placeholder={config.thresholdPlaceholder}
          className="w-28 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-1.5 text-sm disabled:opacity-50"
        />
      </div>

      <ZoneTable
        rows={state.rows}
        onChange={rows => setState(s => ({ ...s, rows }))}
        unit={config.unit}
        convertMpsToKmh={config.convertMps}
        disabled={disabled}
      />

      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => mutation.mutate(state)}
          disabled={disabled || mutation.isPending || config.isSaveDisabled(state)}
          className="cursor-pointer px-3 py-1.5 text-sm font-medium rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {mutation.isPending ? 'Saving...' : config.saveLabel}
        </button>
      </div>
    </div>
  )
}

// ── Per-type configs ──────────────────────────────────────────────────────────

const HR_CONFIG: ZoneSectionConfig = {
  zoneType: ZONE_TYPES.HR,
  heading: 'Heart Rate Zones',
  thresholdLabel: 'LTHR',
  thresholdPlaceholder: 'e.g. 165',
  thresholdMin: 40,
  thresholdMax: 250,
  unit: 'bpm',
  saveLabel: 'Save HR zones',
  successTitle: 'Heart rate zones saved',
  errorTitle: 'Failed to save HR zones',
  readThreshold: d => d.threshold_heart_rate ?? 0,
  buildPayload: (sport, state) => {
    const { zone_names, zone_highs } = rowsToNamesHighs(state.rows)
    return {
      sport,
      sub_sport: 0,
      zone_type: ZONE_TYPES.HR,
      threshold_heart_rate: (state.threshold ?? 0) > 0 ? (state.threshold as number) : undefined,
      zone_names,
      zone_highs,
    }
  },
  isSaveDisabled: state => (state.threshold ?? 0) <= 0,
}

const POWER_CONFIG: ZoneSectionConfig = {
  zoneType: ZONE_TYPES.POWER,
  heading: 'Power Zones',
  thresholdLabel: 'FTP',
  thresholdPlaceholder: 'e.g. 280',
  thresholdMin: 0,
  thresholdMax: 1000,
  unit: 'W',
  saveLabel: 'Save power zones',
  successTitle: 'Power zones saved',
  errorTitle: 'Failed to save power zones',
  readThreshold: d => d.functional_threshold_power ?? 0,
  buildPayload: (sport, state) => {
    const { zone_names, zone_highs } = rowsToNamesHighs(state.rows)
    return {
      sport,
      sub_sport: 0,
      zone_type: ZONE_TYPES.POWER,
      functional_threshold_power: (state.threshold ?? 0) > 0 ? (state.threshold as number) : undefined,
      zone_names,
      zone_highs,
    }
  },
  isSaveDisabled: state => (state.threshold ?? 0) <= 0,
}

const SPEED_CONFIG: ZoneSectionConfig = {
  zoneType: ZONE_TYPES.SPEED,
  heading: 'Speed Zones',
  thresholdLabel: 'CSS',
  thresholdPlaceholder: 'Optional',
  thresholdMin: 0,
  thresholdMax: 100,
  thresholdStep: 0.1,
  unit: 'km/h',
  convertMps: true,
  saveLabel: 'Save speed zones',
  successTitle: 'Speed zones saved',
  errorTitle: 'Failed to save speed zones',
  readThreshold: d =>
    d.threshold_speed_mps != null ? Math.round(d.threshold_speed_mps * 3.6 * 10) / 10 : null,
  buildPayload: (sport, state) => {
    const { zone_names, zone_highs } = rowsToNamesHighs(state.rows)
    return {
      sport,
      sub_sport: 0,
      zone_type: ZONE_TYPES.SPEED,
      threshold_speed_mps: state.threshold != null ? state.threshold / 3.6 : undefined,
      zone_names,
      zone_highs,
    }
  },
  isSaveDisabled: () => false,
}

// ── ZoneEditorCard ────────────────────────────────────────────────────────────

interface ZoneEditorCardProps {
  sportZones: SportZones
  disabled?: boolean
}

export default function ZoneEditorCard({ sportZones, disabled = false }: ZoneEditorCardProps) {
  const hr = sportZones.zones[ZONE_TYPES.HR] ?? null
  const power = sportZones.zones[ZONE_TYPES.POWER] ?? null
  const speed = sportZones.zones[ZONE_TYPES.SPEED] ?? null

  const hasAny = hr != null || power != null || speed != null

  if (!hasAny) {
    return (
      <div className="text-sm text-neutral-500 dark:text-neutral-400 py-4">
        No training zones found for this sport. Import a FIT file with zones to populate this section.
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {hr && (
        <ZoneSection sport={sportZones.sport} detail={hr} disabled={disabled} config={HR_CONFIG} />
      )}

      {power && (
        <>
          {hr && <div className="border-t border-neutral-200 dark:border-neutral-700" />}
          <ZoneSection sport={sportZones.sport} detail={power} disabled={disabled} config={POWER_CONFIG} />
        </>
      )}

      {speed && (
        <>
          {(hr || power) && <div className="border-t border-neutral-200 dark:border-neutral-700" />}
          <ZoneSection sport={sportZones.sport} detail={speed} disabled={disabled} config={SPEED_CONFIG} />
        </>
      )}
    </div>
  )
}

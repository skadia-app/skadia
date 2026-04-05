/**
 * ZoneEditor — per-sport card with sections for HR / Power / Speed zones.
 *
 * Shows source_type badge, threshold input, editable ZoneTable, and Save button.
 * Full-state semantics: fetches current state, user edits, sends complete state on save.
 */

import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/lib/toast'
import { updateTrainingZones } from '@/api/trainingZones'
import type { SportZones, ZoneDetail } from '@/api/trainingZones'
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

// ── Zone section state types ──────────────────────────────────────────────────

interface HrState {
  threshold: number
  rows: ZoneRow[]
}

interface PowerState {
  threshold: number
  rows: ZoneRow[]
}

interface SpeedState {
  threshold: number | null
  rows: ZoneRow[]
}

// ── ZoneSectionHR ─────────────────────────────────────────────────────────────

function ZoneSectionHR({
  detail,
  sport,
  disabled,
}: {
  detail: ZoneDetail
  sport: number
  disabled: boolean
}) {
  const [state, setState] = useState<HrState>({
    threshold: detail.threshold_heart_rate ?? 0,
    rows: zoneDetailToRows(detail),
  })
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  useEffect(() => {
    setState({
      threshold: detail.threshold_heart_rate ?? 0,
      rows: zoneDetailToRows(detail),
    })
  }, [detail])

  const mutation = useMutation({
    mutationFn: () => {
      const { zone_names, zone_highs } = rowsToNamesHighs(state.rows)
      return updateTrainingZones({
        sport,
        sub_sport: 0,
        zone_type: 'hr',
        threshold_heart_rate: state.threshold > 0 ? state.threshold : undefined,
        zone_names,
        zone_highs,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['training-zones'] })
      addToast({ type: 'success', title: 'Heart rate zones saved' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Failed to save HR zones', duration: 0 })
    },
  })

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Heart Rate Zones</h4>
        <SourceBadge sourceType={detail.source_type} />
      </div>

      <div className="flex items-center gap-3">
        <label className="text-sm text-neutral-600 dark:text-neutral-400 shrink-0">
          LTHR (bpm)
        </label>
        <input
          type="number"
          min={40}
          max={250}
          value={state.threshold || ''}
          onChange={e => setState(s => ({ ...s, threshold: e.target.value ? parseInt(e.target.value) : 0 }))}
          disabled={disabled}
          placeholder="e.g. 165"
          className="w-28 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-1.5 text-sm disabled:opacity-50"
        />
      </div>

      <ZoneTable
        rows={state.rows}
        onChange={rows => setState(s => ({ ...s, rows }))}
        unit="bpm"
        disabled={disabled}
      />

      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => mutation.mutate()}
          disabled={disabled || mutation.isPending || state.threshold <= 0}
          className="cursor-pointer px-3 py-1.5 text-sm font-medium rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {mutation.isPending ? 'Saving...' : 'Save HR zones'}
        </button>
      </div>
    </div>
  )
}

// ── ZoneSectionPower ──────────────────────────────────────────────────────────

function ZoneSectionPower({
  detail,
  sport,
  disabled,
}: {
  detail: ZoneDetail
  sport: number
  disabled: boolean
}) {
  const [state, setState] = useState<PowerState>({
    threshold: detail.functional_threshold_power ?? 0,
    rows: zoneDetailToRows(detail),
  })
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  useEffect(() => {
    setState({
      threshold: detail.functional_threshold_power ?? 0,
      rows: zoneDetailToRows(detail),
    })
  }, [detail])

  const mutation = useMutation({
    mutationFn: () => {
      const { zone_names, zone_highs } = rowsToNamesHighs(state.rows)
      return updateTrainingZones({
        sport,
        sub_sport: 0,
        zone_type: 'power',
        functional_threshold_power: state.threshold > 0 ? state.threshold : undefined,
        zone_names,
        zone_highs,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['training-zones'] })
      addToast({ type: 'success', title: 'Power zones saved' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Failed to save power zones', duration: 0 })
    },
  })

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Power Zones</h4>
        <SourceBadge sourceType={detail.source_type} />
      </div>

      <div className="flex items-center gap-3">
        <label className="text-sm text-neutral-600 dark:text-neutral-400 shrink-0">
          FTP (W)
        </label>
        <input
          type="number"
          min={0}
          max={1000}
          value={state.threshold || ''}
          onChange={e => setState(s => ({ ...s, threshold: e.target.value ? parseInt(e.target.value) : 0 }))}
          disabled={disabled}
          placeholder="e.g. 280"
          className="w-28 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-1.5 text-sm disabled:opacity-50"
        />
      </div>

      <ZoneTable
        rows={state.rows}
        onChange={rows => setState(s => ({ ...s, rows }))}
        unit="W"
        disabled={disabled}
      />

      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => mutation.mutate()}
          disabled={disabled || mutation.isPending || state.threshold <= 0}
          className="cursor-pointer px-3 py-1.5 text-sm font-medium rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {mutation.isPending ? 'Saving...' : 'Save power zones'}
        </button>
      </div>
    </div>
  )
}

// ── ZoneSectionSpeed ──────────────────────────────────────────────────────────

function ZoneSectionSpeed({
  detail,
  sport,
  disabled,
}: {
  detail: ZoneDetail
  sport: number
  disabled: boolean
}) {
  const [state, setState] = useState<SpeedState>({
    threshold: detail.threshold_speed_mps != null ? Math.round(detail.threshold_speed_mps * 3.6 * 10) / 10 : null,
    rows: zoneDetailToRows(detail),
  })
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  useEffect(() => {
    setState({
      threshold: detail.threshold_speed_mps != null ? Math.round(detail.threshold_speed_mps * 3.6 * 10) / 10 : null,
      rows: zoneDetailToRows(detail),
    })
  }, [detail])

  const mutation = useMutation({
    mutationFn: () => {
      const { zone_names, zone_highs } = rowsToNamesHighs(state.rows)
      const thresholdMps = state.threshold != null ? state.threshold / 3.6 : undefined
      return updateTrainingZones({
        sport,
        sub_sport: 0,
        zone_type: 'speed',
        threshold_speed_mps: thresholdMps,
        zone_names,
        zone_highs,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['training-zones'] })
      addToast({ type: 'success', title: 'Speed zones saved' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Failed to save speed zones', duration: 0 })
    },
  })

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Speed Zones</h4>
        <SourceBadge sourceType={detail.source_type} />
      </div>

      <div className="flex items-center gap-3">
        <label className="text-sm text-neutral-600 dark:text-neutral-400 shrink-0">
          CSS (km/h)
        </label>
        <input
          type="number"
          min={0}
          max={100}
          step={0.1}
          value={state.threshold ?? ''}
          onChange={e =>
            setState(s => ({
              ...s,
              threshold: e.target.value ? parseFloat(e.target.value) : null,
            }))
          }
          disabled={disabled}
          placeholder="Optional"
          className="w-28 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-1.5 text-sm disabled:opacity-50"
        />
      </div>

      <ZoneTable
        rows={state.rows}
        onChange={rows => setState(s => ({ ...s, rows }))}
        unit="km/h"
        convertMpsToKmh
        disabled={disabled}
      />

      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => mutation.mutate()}
          disabled={disabled || mutation.isPending}
          className="cursor-pointer px-3 py-1.5 text-sm font-medium rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {mutation.isPending ? 'Saving...' : 'Save speed zones'}
        </button>
      </div>
    </div>
  )
}

// ── ZoneEditorCard ────────────────────────────────────────────────────────────

interface ZoneEditorCardProps {
  sportZones: SportZones
  disabled?: boolean
}

export default function ZoneEditorCard({ sportZones, disabled = false }: ZoneEditorCardProps) {
  const { hr, power, speed } = {
    hr: sportZones.zones['hr'] ?? null,
    power: sportZones.zones['power'] ?? null,
    speed: sportZones.zones['speed'] ?? null,
  }

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
        <ZoneSectionHR detail={hr} sport={sportZones.sport} disabled={disabled} />
      )}

      {power && (
        <>
          {hr && <div className="border-t border-neutral-200 dark:border-neutral-700" />}
          <ZoneSectionPower detail={power} sport={sportZones.sport} disabled={disabled} />
        </>
      )}

      {speed && (
        <>
          {(hr || power) && <div className="border-t border-neutral-200 dark:border-neutral-700" />}
          <ZoneSectionSpeed detail={speed} sport={sportZones.sport} disabled={disabled} />
        </>
      )}
    </div>
  )
}

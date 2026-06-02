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
import { speedDisplayUnit, speedFromDisplayValue, speedToDisplayValue, type UnitSystem } from '@/lib/units'
import { useUser } from '@/providers/UserProvider'
import ZoneTable from './ZoneTable'
import type { ZoneRow } from './ZoneTable'
import { useTranslation } from 'react-i18next'

// ── Source badge ──────────────────────────────────────────────────────────────

function sourceLabel(sourceType: string, strings: { deviceImport: string; manual: string }): string {
  const lower = sourceType.toLowerCase()
  if (lower.includes('fit') || lower.includes('garmin')) return strings.deviceImport
  if (lower.includes('manual')) return strings.manual
  return sourceType
}

function SourceBadge({ sourceType, strings }: { sourceType: string; strings: { deviceImport: string; manual: string } }) {
  const label = sourceLabel(sourceType, strings)
  const lower = sourceType.toLowerCase()
  const isDevice = lower.includes('fit') || lower.includes('garmin')
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
  thresholdLabel: string
  thresholdMin: number
  thresholdMax: number
  thresholdStep?: number
  unit: string
  convertMps?: boolean
  copyKey: 'heartRate' | 'power' | 'speed'
  /** Read threshold from ZoneDetail into display units */
  readThreshold: (d: ZoneDetail, units: UnitSystem) => number | null
  /** Build the API payload from sport + display-unit state */
  buildPayload: (sport: number, state: ZoneState, units: UnitSystem) => TrainingZoneUpdate
  /** True when save should be disabled (beyond isPending) */
  isSaveDisabled: (state: ZoneState) => boolean
}

type ZoneSectionKey = ZoneSectionConfig['copyKey']

interface ZoneEditorStrings {
  saving: string
  empty: string
  source: {
    deviceImport: string
    manual: string
  }
  sections: Record<ZoneSectionKey, {
    heading: string
    thresholdPlaceholder: string
    saveLabel: string
    successTitle: string
    errorTitle: string
  }>
}

// ── ZoneSection ───────────────────────────────────────────────────────────────

function ZoneSection({
  sport,
  detail,
  disabled,
  config,
  strings,
  units,
}: {
  sport: number
  detail: ZoneDetail
  disabled: boolean
  config: ZoneSectionConfig
  strings: ZoneEditorStrings
  units: UnitSystem
}) {
  const displayUnit = config.copyKey === 'speed' ? speedDisplayUnit(units) : config.unit
  const thresholdMin = config.convertMps ? roundDisplaySpeed(speedToDisplayValue(config.thresholdMin, units)) : config.thresholdMin
  const thresholdMax = config.convertMps ? roundDisplaySpeed(speedToDisplayValue(config.thresholdMax, units)) : config.thresholdMax
  const [state, setState] = useState<ZoneState>({
    threshold: config.readThreshold(detail, units),
    rows: zoneDetailToRows(detail),
  })

  useEffect(() => {
    setState({
      threshold: config.readThreshold(detail, units),
      rows: zoneDetailToRows(detail),
    })
  }, [detail, units]) // eslint-disable-line react-hooks/exhaustive-deps

  const mutation = useZoneSave(
    s => config.buildPayload(sport, s, units),
    strings.sections[config.copyKey].successTitle,
    strings.sections[config.copyKey].errorTitle,
  )

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{strings.sections[config.copyKey].heading}</h4>
        <SourceBadge sourceType={detail.source_type} strings={strings.source} />
      </div>

      <div className="flex items-center gap-3">
        <label className="text-sm text-neutral-600 dark:text-neutral-400 shrink-0">
          {config.thresholdLabel} ({displayUnit})
        </label>
        <input
          type="number"
          min={thresholdMin}
          max={thresholdMax}
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
          placeholder={strings.sections[config.copyKey].thresholdPlaceholder}
          className="w-28 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-1.5 text-sm disabled:opacity-50"
        />
      </div>

      <ZoneTable
        rows={state.rows}
        onChange={rows => setState(s => ({ ...s, rows }))}
        unit={displayUnit}
        unitSystem={units}
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
          {mutation.isPending ? strings.saving : strings.sections[config.copyKey].saveLabel}
        </button>
      </div>
    </div>
  )
}

// ── Per-type configs ──────────────────────────────────────────────────────────

const HR_CONFIG: ZoneSectionConfig = {
  zoneType: ZONE_TYPES.HR,
  thresholdLabel: 'LTHR',
  thresholdMin: 40,
  thresholdMax: 250,
  unit: 'bpm',
  copyKey: 'heartRate',
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
  thresholdLabel: 'FTP',
  thresholdMin: 0,
  thresholdMax: 1000,
  unit: 'W',
  copyKey: 'power',
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
  thresholdLabel: 'CSS',
  thresholdMin: 0,
  thresholdMax: 100 / 3.6,
  thresholdStep: 0.1,
  unit: 'km/h',
  convertMps: true,
  copyKey: 'speed',
  readThreshold: (d, units) =>
    d.threshold_speed_mps != null ? roundDisplaySpeed(speedToDisplayValue(d.threshold_speed_mps, units)) : null,
  buildPayload: (sport, state, units) => {
    const { zone_names, zone_highs } = rowsToNamesHighs(state.rows)
    return {
      sport,
      sub_sport: 0,
      zone_type: ZONE_TYPES.SPEED,
      threshold_speed_mps: state.threshold != null ? speedFromDisplayValue(state.threshold, units) : undefined,
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

function roundDisplaySpeed(value: number): number {
  return Math.round(value * 10) / 10
}

export default function ZoneEditorCard({ sportZones, disabled = false }: ZoneEditorCardProps) {
  const { t } = useTranslation()
  const { units } = useUser()
  const strings: ZoneEditorStrings = {
    saving: t('gearZones.zoneEditor.saving'),
    empty: t('gearZones.zoneEditor.empty'),
    source: {
      deviceImport: t('gearZones.zoneEditor.source.deviceImport'),
      manual: t('gearZones.zoneEditor.source.manual'),
    },
    sections: {
      heartRate: {
        heading: t('gearZones.zoneEditor.sections.heartRate.heading'),
        thresholdPlaceholder: t('gearZones.zoneEditor.sections.heartRate.thresholdPlaceholder'),
        saveLabel: t('gearZones.zoneEditor.sections.heartRate.saveLabel'),
        successTitle: t('gearZones.zoneEditor.sections.heartRate.successTitle'),
        errorTitle: t('gearZones.zoneEditor.sections.heartRate.errorTitle'),
      },
      power: {
        heading: t('gearZones.zoneEditor.sections.power.heading'),
        thresholdPlaceholder: t('gearZones.zoneEditor.sections.power.thresholdPlaceholder'),
        saveLabel: t('gearZones.zoneEditor.sections.power.saveLabel'),
        successTitle: t('gearZones.zoneEditor.sections.power.successTitle'),
        errorTitle: t('gearZones.zoneEditor.sections.power.errorTitle'),
      },
      speed: {
        heading: t('gearZones.zoneEditor.sections.speed.heading'),
        thresholdPlaceholder: t('gearZones.zoneEditor.sections.speed.thresholdPlaceholder'),
        saveLabel: t('gearZones.zoneEditor.sections.speed.saveLabel'),
        successTitle: t('gearZones.zoneEditor.sections.speed.successTitle'),
        errorTitle: t('gearZones.zoneEditor.sections.speed.errorTitle'),
      },
    },
  }
  const hr = sportZones.zones[ZONE_TYPES.HR] ?? null
  const power = sportZones.zones[ZONE_TYPES.POWER] ?? null
  const speed = sportZones.zones[ZONE_TYPES.SPEED] ?? null

  const hasAny = hr != null || power != null || speed != null

  if (!hasAny) {
    return (
      <div className="text-sm text-neutral-500 dark:text-neutral-400 py-4">
        {strings.empty}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {hr && (
        <ZoneSection sport={sportZones.sport} detail={hr} disabled={disabled} config={HR_CONFIG} strings={strings} units={units} />
      )}

      {power && (
        <>
          {hr && <div className="border-t border-neutral-200 dark:border-neutral-700" />}
          <ZoneSection sport={sportZones.sport} detail={power} disabled={disabled} config={POWER_CONFIG} strings={strings} units={units} />
        </>
      )}

      {speed && (
        <>
          {(hr || power) && <div className="border-t border-neutral-200 dark:border-neutral-700" />}
          <ZoneSection sport={sportZones.sport} detail={speed} disabled={disabled} config={SPEED_CONFIG} strings={strings} units={units} />
        </>
      )}
    </div>
  )
}

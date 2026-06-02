import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { ZONE_TYPES, type SportZones } from '@/api/trainingZones'
import { ToastProvider } from '@/lib/toast'
import ZoneEditorCard from './ZoneEditor'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      const labels: Record<string, string> = {
        'gearZones.zoneEditor.saving': 'Saving...',
        'gearZones.zoneEditor.empty': 'No training zones',
        'gearZones.zoneEditor.source.deviceImport': 'Device import',
        'gearZones.zoneEditor.source.manual': 'Manual',
        'gearZones.zoneEditor.sections.heartRate.heading': 'Heart Rate zones',
        'gearZones.zoneEditor.sections.heartRate.thresholdPlaceholder': 'e.g. 165',
        'gearZones.zoneEditor.sections.heartRate.saveLabel': 'Save Heart Rate zones',
        'gearZones.zoneEditor.sections.heartRate.successTitle': 'Heart Rate zones saved',
        'gearZones.zoneEditor.sections.heartRate.errorTitle': 'Failed to save Heart Rate zones',
        'gearZones.zoneEditor.sections.power.heading': 'Power zones',
        'gearZones.zoneEditor.sections.power.thresholdPlaceholder': 'e.g. 280',
        'gearZones.zoneEditor.sections.power.saveLabel': 'Save Power zones',
        'gearZones.zoneEditor.sections.power.successTitle': 'Power zones saved',
        'gearZones.zoneEditor.sections.power.errorTitle': 'Failed to save Power zones',
        'gearZones.zoneEditor.sections.speed.heading': 'Speed zones',
        'gearZones.zoneEditor.sections.speed.thresholdPlaceholder': 'Optional',
        'gearZones.zoneEditor.sections.speed.saveLabel': 'Save Speed zones',
        'gearZones.zoneEditor.sections.speed.successTitle': 'Speed zones saved',
        'gearZones.zoneEditor.sections.speed.errorTitle': 'Failed to save Speed zones',
        'activityTables.zoneTable.name': 'Name',
        'activityTables.zoneTable.low': 'Low',
        'activityTables.zoneTable.high': 'High',
        'activityTables.zoneTable.max': 'max',
        'activityTables.zoneTable.add': '+ Add zone',
        'activityTables.zoneTable.removeTitle': 'Remove zone',
      }
      if (key === 'activityTables.zoneTable.defaultName') return `Zone ${params?.number}`
      if (key === 'activityTables.zoneTable.nameAria') return `Name for zone ${params?.number}`
      if (key === 'activityTables.zoneTable.upperBoundAria') return `Upper bound for zone ${params?.number}`
      if (key === 'activityTables.zoneTable.removeAria') return `Remove zone ${params?.number}`
      return labels[key] ?? key
    },
  }),
}))

describe('ZoneEditorCard unit preferences', () => {
  it('uses user units for speed zones while keeping heart-rate and power units fixed', () => {
    renderWithProviders(<ZoneEditorCard sportZones={sportZones()} />)

    expect(screen.getByText('LTHR (bpm)')).toBeInTheDocument()
    expect(screen.getByText('FTP (W)')).toBeInTheDocument()
    expect(screen.getByText('CSS (mph)')).toBeInTheDocument()
    const speedThreshold = screen.getByDisplayValue('22.4')
    expect(speedThreshold).toBeInTheDocument()
    expect(speedThreshold).toHaveAttribute('min', '0')
    expect(speedThreshold).toHaveAttribute('max', '62.1')
    expect(screen.queryByText('CSS (km/h)')).not.toBeInTheDocument()
  })
})

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>{ui}</ToastProvider>
    </QueryClientProvider>,
  )
}

function sportZones(): SportZones {
  return {
    sport: 1,
    sport_label: 'Running',
    zones: {
      [ZONE_TYPES.HR]: {
        source_type: 'manual',
        threshold_heart_rate: 165,
        zone_names: ['Z1', 'Z2'],
        zone_highs: [140, 9999],
      },
      [ZONE_TYPES.POWER]: {
        source_type: 'manual',
        functional_threshold_power: 280,
        zone_names: ['Z1', 'Z2'],
        zone_highs: [220, 9999],
      },
      [ZONE_TYPES.SPEED]: {
        source_type: 'manual',
        threshold_speed_mps: 10,
        zone_names: ['Z1', 'Z2'],
        zone_highs: [5, 9999],
      },
    },
  }
}

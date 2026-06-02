import { beforeEach, describe, expect, it } from 'vitest'
import { i18n } from '@/i18n/i18n'
import {
  getJobMessageLabel,
  getJobPhaseLabel,
  getJobStatusLabel,
  getJobTypeLabel,
  getJobNotificationTitle,
  JOB_TYPE_LABEL_KEYS,
} from './jobLabels'
import type { ActiveJob } from '@/api/jobs'

const baseJob: ActiveJob = {
  id: 'job-1',
  job_type: 'garmin_import',
  status: 'completed',
  progress_current: 0,
  progress_total: 0,
  started_at: '2026-05-31T00:00:00Z',
}

describe('jobLabels', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('returns localized labels for known job types', async () => {
    expect(getJobTypeLabel('garmin_import')).toBe('Garmin-Import')
    expect(getJobTypeLabel('lift_recompute')).toBe('Lift-Erkennung neu berechnen')

    await i18n.changeLanguage('en')

    expect(getJobTypeLabel('garmin_import')).toBe('Garmin import')
    expect(getJobTypeLabel('lift_recompute')).toBe('Recompute lift detection')
  })

  it('has catalog entries for every mapped job type', async () => {
    for (const locale of ['de', 'en', 'fr', 'it', 'es']) {
      await i18n.changeLanguage(locale)
      for (const labelKey of Object.values(JOB_TYPE_LABEL_KEYS)) {
        expect(i18n.exists(`jobs.type.${labelKey}`), `${locale}: jobs.type.${labelKey}`).toBe(true)
      }
    }
  })

  it('labels unknown job types without exposing backend codes', async () => {
    expect(getJobTypeLabel('unknown_private_job')).toBe('Hintergrundjob')

    await i18n.changeLanguage('en')

    expect(getJobTypeLabel('unknown_private_job')).toBe('Background job')
  })

  it('labels unknown job status and phase values without exposing backend codes', () => {
    expect(getJobStatusLabel('paused')).toBe('Unbekannter Status')
    expect(getJobPhaseLabel('private_backend_phase')).toBe('Unbekannte Phase')
  })

  it('labels known backend phases instead of treating them as unknown', () => {
    expect(getJobPhaseLabel('tiles')).toBe('Tiles erstellen')
    expect(getJobPhaseLabel('coverage')).toBe('Coverage erstellen')
    expect(getJobPhaseLabel('dem_download')).toBe('DEM herunterladen')
    expect(getJobPhaseLabel('Multisport')).toBe('Multisport wird verarbeitet')
    expect(getJobPhaseLabel('Zusammenfassen')).toBe('Zusammenfassen')
    expect(getJobPhaseLabel('region_1_europe_alps')).toBe('Region wird verarbeitet')
    expect(getJobPhaseLabel('brouter_OsmFastCutter')).toBe('BRouter wird verarbeitet')
    expect(getJobPhaseLabel('landmarks: distance valleys')).toBe('Landmark-Vorverarbeitung')
    expect(getJobPhaseLabel('data_import_files')).toBe('Importiere Dateien')
    expect(getJobPhaseLabel('data_import_phase_3')).toBe('Importiere Phase 3')
    expect(getJobPhaseLabel('Disziplin 1/3')).toBe('Disziplin wird verarbeitet')
  })

  it('formats completion notifications without exposing unknown job type codes', async () => {
    const failedUnknownJob = { ...baseJob, job_type: 'unknown_private_job', status: 'failed' as const }

    expect(getJobNotificationTitle(baseJob)).toBe('Garmin-Import abgeschlossen')
    expect(getJobNotificationTitle(failedUnknownJob)).toBe('Hintergrundjob fehlgeschlagen')

    await i18n.changeLanguage('en')

    expect(getJobNotificationTitle({ ...baseJob, entity_name: 'Morning Ride' })).toBe('Garmin import - Morning Ride completed')
    expect(getJobNotificationTitle(failedUnknownJob)).toBe('Background job failed')
    expect(getJobStatusLabel('failed')).toBe('Failed')
  })

  it('formats structured admin job messages without exposing unknown backend codes', async () => {
    expect(getJobMessageLabel({
      ...baseJob,
      message_code: 'admin.osm.update.patch_failed',
      params: { region_id: 'europe_alps' },
    })).toBe('OSM-Update-Patch für europe_alps fehlgeschlagen')
    expect(getJobMessageLabel({ ...baseJob, message_code: 'admin.private.raw_code' })).toBeNull()

    await i18n.changeLanguage('en')

    expect(getJobMessageLabel({
      ...baseJob,
      message_code: 'admin.osm.update.completed',
      params: { region_id: 'europe_alps', sequence_number: 123 },
    })).toBe('OSM update for europe_alps completed')
    expect(getJobNotificationTitle({
      ...baseJob,
      status: 'failed',
      message_code: 'admin.osm.update.region_not_in_catalog',
      params: { region_id: 'europe_alps' },
    })).toBe('OSM update region europe_alps is not in the catalog')
  })
})

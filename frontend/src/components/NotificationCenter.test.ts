import { beforeEach, describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { i18n } from '@/i18n/i18n'
import { localizeNotification, type NotificationItem } from './NotificationCenter'

describe('localizeNotification', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('uses structured notification keys when present', async () => {
    const item: NotificationItem = {
      id: 'n1',
      category: 'job',
      title: 'Legacy title',
      body: 'Legacy body',
      read: false,
      created_at: '2026-05-31T00:00:00Z',
      data: {
        key: 'notifications.jobs.completed',
        params: { jobType: 'garmin_import', entityName: 'Morning Ride' },
      },
    }

    expect(localizeNotification(item)).toEqual({
      title: 'Garmin-Import - Morning Ride abgeschlossen',
      body: null,
    })

    await i18n.changeLanguage('en')

    expect(localizeNotification(item).title).toBe('Garmin import - Morning Ride completed')
  })

  it('keeps legacy rendered title and body as fallback', () => {
    const item: NotificationItem = {
      id: 'n2',
      category: 'custom',
      title: 'Custom rule',
      body: 'User-authored text',
      read: true,
      created_at: '2026-05-31T00:00:00Z',
    }

    expect(localizeNotification(item)).toEqual({
      title: 'Custom rule',
      body: 'User-authored text',
    })
  })

  it('does not use empty defaultValue while resolving structured notification keys', async () => {
    const source = readFileSync(resolve(process.cwd(), 'src/components/NotificationCenter.tsx'), 'utf8')
    const item: NotificationItem = {
      id: 'n-missing-key',
      category: 'custom',
      title: 'Legacy fallback title',
      body: 'Legacy fallback body',
      read: true,
      created_at: '2026-05-31T00:00:00Z',
      data: {
        key: 'notifications.unknownBackendKey',
        params: { code: 'raw_backend_code' },
      },
    }

    expect(source).not.toContain('defaultValue')
    expect(localizeNotification(item)).toEqual({
      title: 'Legacy fallback title',
      body: 'Legacy fallback body',
    })
    await i18n.changeLanguage('en')
    expect(localizeNotification(item).title).toBe('Legacy fallback title')
  })

  it('localizes structured OSM import notifications', async () => {
    const partialFailure: NotificationItem = {
      id: 'n3',
      category: 'warning',
      title: 'Legacy partial failure title',
      body: 'Legacy partial failure body',
      read: false,
      created_at: '2026-05-31T00:00:00Z',
      data: {
        key: 'notifications.osmImport.partialFailure',
        params: { succeeded: 2, total: 3, failedRegions: 'europe_alps' },
      },
    }
    const databaseIssue: NotificationItem = {
      id: 'n4',
      category: 'error',
      title: 'Legacy database issue title',
      body: 'Legacy database issue body',
      read: false,
      created_at: '2026-05-31T00:00:00Z',
      data: {
        key: 'notifications.osmImport.databaseIssue',
        params: { region: 'Alpen', error: 'XX0' },
      },
    }

    expect(localizeNotification(partialFailure)).toEqual({
      title: 'OSM-Import teilweise fehlgeschlagen',
      body: '2/3 Regionen importiert. Fehlgeschlagen: europe_alps',
    })
    expect(localizeNotification(databaseIssue)).toEqual({
      title: 'Datenbankproblem beim OSM-Import',
      body: 'Region Alpen: XX0',
    })

    await i18n.changeLanguage('en')

    expect(localizeNotification(partialFailure)).toEqual({
      title: 'OSM import partially failed',
      body: 'Imported 2/3 regions. Failed: europe_alps',
    })
    expect(localizeNotification(databaseIssue)).toEqual({
      title: 'Database issue during OSM import',
      body: 'Region Alpen: XX0',
    })
  })

  it('localizes automation rule notification frames while preserving rule text', async () => {
    const item: NotificationItem = {
      id: 'n5',
      category: 'rule',
      title: 'Legacy rule title',
      body: 'Legacy rule body',
      read: false,
      created_at: '2026-05-31T00:00:00Z',
      data: {
        key: 'notifications.automation.ruleNotification',
        params: { ruleName: 'Pendeln', message: 'Markiert als Arbeitsweg' },
      },
    }

    expect(localizeNotification(item)).toEqual({
      title: 'Automatisierungsregel: Pendeln',
      body: 'Markiert als Arbeitsweg',
    })

    await i18n.changeLanguage('en')

    expect(localizeNotification(item)).toEqual({
      title: 'Automation rule: Pendeln',
      body: 'Markiert als Arbeitsweg',
    })
  })
})

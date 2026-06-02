import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '@/i18n/I18nProvider'
import { i18n } from '@/i18n/i18n'

const apiMock = vi.hoisted(() => ({
  getLatestDataExport: vi.fn(),
  createDataExportDownloadSession: vi.fn(),
  triggerNativeDownload: vi.fn(),
  deleteAccount: vi.fn(),
  createMultipartDataImport: vi.fn(),
  uploadMultipartDataImportPart: vi.fn(),
  startMultipartDataImport: vi.fn(),
  setUnits: vi.fn(),
  setLocale: vi.fn(),
  hasPassword: true,
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({
    user: { username: 'test', display_name: 'Test', email: 't@example.com', has_password: apiMock.hasPassword, locale: 'de', units: 'metric' },
    locale: 'de',
    units: 'metric',
    hasPassword: apiMock.hasPassword,
    setUnits: apiMock.setUnits,
    setLocale: apiMock.setLocale,
  }),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getSessions: vi.fn().mockResolvedValue([]),
    getDataImportStatus: vi.fn().mockResolvedValue({ job_id: 'import-1', status: 'running' }),
    getLatestDataExport: apiMock.getLatestDataExport,
    createDataExportDownloadSession: apiMock.createDataExportDownloadSession,
    createMultipartDataImport: apiMock.createMultipartDataImport,
    uploadMultipartDataImportPart: apiMock.uploadMultipartDataImportPart,
    startMultipartDataImport: apiMock.startMultipartDataImport,
    cancelMultipartDataImport: vi.fn(),
    deleteAccount: apiMock.deleteAccount,
  }
})

vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client')
  return {
    ...actual,
    triggerNativeDownload: apiMock.triggerNativeDownload,
  }
})

import AccountSettingsTab, { formatDataImportProgressMessage } from './AccountSettingsTab'

function renderSettings() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nProvider>
      <MemoryRouter>
        <QueryClientProvider client={queryClient}>
          <AccountSettingsTab />
        </QueryClientProvider>
      </MemoryRouter>
    </I18nProvider>,
  )
}

describe('AccountSettingsTab GDPR export downloads', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.hasPassword = true
  })

  it('requests a download session and triggers a native download', async () => {
    apiMock.getLatestDataExport.mockResolvedValue({
      job_id: 'job-1',
      status: 'completed',
      artifact_format: 'single_zip',
      download_session_url: '/api/v1/auth/data-export/job-1/download-session',
      expires_at: '2099-05-12T12:00:00Z',
    })
    apiMock.createDataExportDownloadSession.mockResolvedValue({
      url: '/api/v1/downloads/gdpr/data-export/job-1/parts/export.zip?session_id=s1',
      expires_at: '2099-05-12T12:05:00Z',
    })

    renderSettings()
    const button = await screen.findByRole('button', { name: 'Herunterladen' })
    await userEvent.click(button)

    await waitFor(() => {
      expect(apiMock.createDataExportDownloadSession).toHaveBeenCalledWith('job-1', { single: true })
      expect(apiMock.triggerNativeDownload).toHaveBeenCalled()
    })
  })

  it('downloads multipart export parts sequentially from one all-objects session', async () => {
    apiMock.getLatestDataExport.mockResolvedValue({
      job_id: 'job-2',
      status: 'completed',
      artifact_format: 'multipart',
      parts: [
        { name: 'manifest.json', role: 'manifest', size_bytes: 12 },
        { name: 'metadata.zip', role: 'metadata_zip', size_bytes: 34 },
        { name: 'files-0001.zip', role: 'files_zip', size_bytes: 56 },
      ],
      expires_at: '2099-05-12T12:00:00Z',
    })
    apiMock.createDataExportDownloadSession.mockResolvedValue({
      url: '/api/v1/downloads/gdpr/data-export/job-2/parts/manifest.json?session_id=s2',
      expires_at: '2099-05-12T12:05:00Z',
      objects: [
        { name: 'manifest.json', url: '/api/v1/downloads/gdpr/data-export/job-2/parts/manifest.json?session_id=s2' },
        { name: 'metadata.zip', url: '/api/v1/downloads/gdpr/data-export/job-2/parts/metadata.zip?session_id=s2' },
        { name: 'files-0001.zip', url: '/api/v1/downloads/gdpr/data-export/job-2/parts/files-0001.zip?session_id=s2' },
      ],
    })

    renderSettings()
    const button = await screen.findByRole('button', { name: 'Alle Teile herunterladen' })
    await userEvent.click(button)

    await waitFor(() => {
      expect(apiMock.createDataExportDownloadSession).toHaveBeenCalledWith('job-2', { all: true })
      expect(apiMock.triggerNativeDownload).toHaveBeenCalledTimes(3)
      expect(apiMock.triggerNativeDownload).toHaveBeenNthCalledWith(
        2,
        '/api/v1/downloads/gdpr/data-export/job-2/parts/metadata.zip?session_id=s2',
        'metadata.zip',
      )
    })
  })
})

describe('AccountSettingsTab preferences', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.hasPassword = true
    apiMock.getLatestDataExport.mockResolvedValue(null)
    apiMock.setUnits.mockResolvedValue(undefined)
    apiMock.setLocale.mockResolvedValue(undefined)
  })

  it('persists the selected language from account preferences', async () => {
    renderSettings()

    await userEvent.click(await screen.findByRole('button', { name: /English/ }))

    await waitFor(() => {
      expect(apiMock.setLocale).toHaveBeenCalledWith('en')
    })
  })

  it('persists the selected unit system', async () => {
    renderSettings()

    await userEvent.click(await screen.findByRole('button', { name: /Imperial/ }))

    await waitFor(() => {
      expect(apiMock.setUnits).toHaveBeenCalledWith('imperial')
    })
  })

  it('shows localized unit details for distance, weight, temperature, speed, and pace', async () => {
    renderSettings()

    expect(await screen.findByText('Distanz/Höhe: km, m')).toBeTruthy()
    expect(screen.getByText('Gewicht: kg')).toBeTruthy()
    expect(screen.getByText('Temperatur: °C')).toBeTruthy()
    expect(screen.getByText('Geschwindigkeit: km/h')).toBeTruthy()
    expect(screen.getByText('Pace: min/km')).toBeTruthy()

    expect(screen.getByText('Distanz/Höhe: mi, ft')).toBeTruthy()
    expect(screen.getByText('Gewicht: lb')).toBeTruthy()
    expect(screen.getByText('Temperatur: °F')).toBeTruthy()
    expect(screen.getByText('Geschwindigkeit: mph')).toBeTruthy()
    expect(screen.getByText('Pace: min/mi')).toBeTruthy()
  })
})

describe('AccountSettingsTab multipart GDPR import', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.hasPassword = true
    apiMock.getLatestDataExport.mockResolvedValue(null)
  })

  it('uploads manifest parts and starts the multipart import session', async () => {
    apiMock.createMultipartDataImport.mockResolvedValue({
      job_id: 'import-1',
      status: 'uploading',
      expires_at: '2099-05-12T12:00:00Z',
    })
    apiMock.uploadMultipartDataImportPart.mockResolvedValue({ status: 'received' })
    apiMock.startMultipartDataImport.mockResolvedValue({ job_id: 'import-1', status: 'running' })
    const manifest = new File([JSON.stringify({
      format_version: 1,
      multipart: true,
      parts: [
        { name: 'metadata.zip', role: 'metadata', sha256: 'a'.repeat(64), size_bytes: 4 },
        { name: 'files-0001.zip', role: 'files', sha256: 'b'.repeat(64), size_bytes: 5 },
      ],
    })], 'manifest.json', { type: 'application/json' })
    const metadata = new File(['meta'], 'metadata.zip', { type: 'application/zip' })
    const files = new File(['files'], 'files-0001.zip', { type: 'application/zip' })

    renderSettings()
    await userEvent.click(screen.getByRole('button', { name: 'Mehrteiliger Export' }))
    await userEvent.upload(screen.getByLabelText('Multipart-Import-Dateien'), [manifest, metadata, files])
    await userEvent.click(await screen.findByRole('button', { name: 'Import starten' }))

    await waitFor(() => {
      expect(apiMock.createMultipartDataImport).toHaveBeenCalled()
      expect(apiMock.uploadMultipartDataImportPart).toHaveBeenNthCalledWith(1, 'import-1', metadata)
      expect(apiMock.uploadMultipartDataImportPart).toHaveBeenNthCalledWith(2, 'import-1', files)
      expect(apiMock.startMultipartDataImport).toHaveBeenCalledWith('import-1')
    })
  })

  it('localizes import progress without exposing backend message text', () => {
    const message = formatDataImportProgressMessage({
      job_id: 'import-1',
      status: 'running',
      phase_name: 'files',
      message: 'Importing files...',
    }, i18n.t)

    expect(message).toBe('Importiere Dateien...')
    expect(message).not.toBe('Importing files...')
    expect(formatDataImportProgressMessage({
      job_id: 'import-1',
      status: 'running',
      message: 'backend raw message',
    }, i18n.t)).toBeNull()
  })
})

describe('AccountSettingsTab account deletion confirmation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.hasPassword = false
    apiMock.getLatestDataExport.mockResolvedValue(null)
    apiMock.deleteAccount.mockResolvedValue({ status: 'deletion_scheduled' })
  })

  it('accepts the localized confirmation phrase but sends the backend confirmation token', async () => {
    const originalPhrase = i18n.t('account.delete.confirmationPhrase')
    const originalPlaceholder = i18n.t('account.delete.confirmationPlaceholder')
    i18n.addResource('de', 'translation', 'account.delete.confirmationPhrase', 'DELETE MY ACCOUNT')
    i18n.addResource('de', 'translation', 'account.delete.confirmationPlaceholder', 'DELETE MY ACCOUNT')

    try {
      renderSettings()

      await userEvent.click(await screen.findByRole('button', { name: 'Account löschen' }))
      await userEvent.type(screen.getByPlaceholderText('DELETE MY ACCOUNT'), 'DELETE MY ACCOUNT')
      await userEvent.click(screen.getByRole('button', { name: 'Unwiderruflich löschen' }))

      await waitFor(() => {
        expect(apiMock.deleteAccount.mock.calls[0]?.[0]).toEqual({ confirmation: 'ACCOUNT LÖSCHEN' })
      })
    } finally {
      i18n.addResource('de', 'translation', 'account.delete.confirmationPhrase', originalPhrase)
      i18n.addResource('de', 'translation', 'account.delete.confirmationPlaceholder', originalPlaceholder)
    }
  })
})

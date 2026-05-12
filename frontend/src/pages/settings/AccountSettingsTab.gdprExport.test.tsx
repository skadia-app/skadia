import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMock = vi.hoisted(() => ({
  getLatestDataExport: vi.fn(),
  createDataExportDownloadSession: vi.fn(),
  triggerNativeDownload: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ user: { username: 'test', display_name: 'Test', email: 't@example.com', has_password: true } }),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getSessions: vi.fn().mockResolvedValue([]),
    getLatestDataExport: apiMock.getLatestDataExport,
    createDataExportDownloadSession: apiMock.createDataExportDownloadSession,
  }
})

vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client')
  return {
    ...actual,
    triggerNativeDownload: apiMock.triggerNativeDownload,
  }
})

import AccountSettingsTab from './AccountSettingsTab'

function renderSettings() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <AccountSettingsTab />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('AccountSettingsTab GDPR export downloads', () => {
  beforeEach(() => {
    vi.clearAllMocks()
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

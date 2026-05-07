import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiFetchMock } = vi.hoisted(() => ({
  apiFetchMock: vi.fn(),
}))

vi.mock('@/lib/auth', () => ({
  apiFetch: apiFetchMock,
}))

import {
  cancelMultipartDataImport,
  createDataExportDownloadSession,
  createMultipartDataImport,
  getLatestDataExport,
  startMultipartDataImport,
  updateMe,
  uploadMultipartDataImportPart,
} from '@/api/auth'

beforeEach(() => {
  vi.clearAllMocks()
})

describe('data export API', () => {
  it('loads the latest data export status from the stable lookup endpoint', async () => {
    apiFetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      job_id: 'job-1',
      status: 'completed',
      download_session_url: '/api/v1/auth/data-export/job-1/download-session',
      artifact_format: 'single_zip',
      expires_at: '2026-05-08T12:00:00Z',
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }))

    const result = await getLatestDataExport()

    expect(apiFetchMock).toHaveBeenCalledWith('/api/v1/auth/data-export/latest')
    expect(result?.job_id).toBe('job-1')
    expect(result?.download_session_url).toBe('/api/v1/auth/data-export/job-1/download-session')
  })

  it('returns null when no prior data export exists', async () => {
    apiFetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ error: 'not_found' }), {
      status: 404,
      headers: { 'Content-Type': 'application/json' },
    }))

    await expect(getLatestDataExport()).resolves.toBeNull()
  })

  it('creates a same-origin data export download session', async () => {
    apiFetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      url: '/api/v1/downloads/gdpr/data-export/job-1/parts/export.zip?session_id=s1',
      expires_at: '2026-05-08T12:05:00Z',
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }))

    const result = await createDataExportDownloadSession('job-1', { single: true })

    expect(apiFetchMock).toHaveBeenCalledWith('/api/v1/auth/data-export/job-1/download-session', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ single: true }),
    })
    expect(result.url).toContain('session_id=s1')
  })
})

describe('multipart data import API', () => {
  it('creates a multipart import session from manifest JSON', async () => {
    apiFetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      job_id: 'job-2',
      status: 'uploading',
      expires_at: '2026-05-08T12:00:00Z',
    }), {
      status: 202,
      headers: { 'Content-Type': 'application/json' },
    }))

    const manifest = { format_version: 1, multipart: true, parts: [] }
    const result = await createMultipartDataImport(manifest)

    expect(apiFetchMock).toHaveBeenCalledWith('/api/v1/auth/data-import/multipart', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(manifest),
    })
    expect(result.job_id).toBe('job-2')
  })

  it('uploads a multipart import part as the raw ZIP body', async () => {
    apiFetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ status: 'received' }), {
      status: 202,
      headers: { 'Content-Type': 'application/json' },
    }))

    const file = new File(['zip'], 'metadata.zip', { type: 'application/zip' })
    await uploadMultipartDataImportPart('job-2', file)

    expect(apiFetchMock).toHaveBeenCalledWith('/api/v1/auth/data-import/job-2/parts/metadata.zip', {
      method: 'POST',
      body: file,
    })
  })

  it('starts and cancels multipart import sessions', async () => {
    apiFetchMock
      .mockResolvedValueOnce(new Response(JSON.stringify({ job_id: 'job-2', status: 'running' }), {
        status: 202,
        headers: { 'Content-Type': 'application/json' },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: 'cancelled' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }))

    await startMultipartDataImport('job-2')
    await cancelMultipartDataImport('job-2')

    expect(apiFetchMock).toHaveBeenNthCalledWith(1, '/api/v1/auth/data-import/job-2/start', { method: 'POST' })
    expect(apiFetchMock).toHaveBeenNthCalledWith(2, '/api/v1/auth/data-import/job-2/cancel', { method: 'POST' })
  })
})

describe('account preference API', () => {
  it('patches locale and unit preferences and returns the updated user', async () => {
    apiFetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      user_id: 'user-1',
      username: 'demo',
      display_name: 'Demo User',
      email: 'demo@example.test',
      is_admin: false,
      email_verified: true,
      has_password: true,
      must_change_password: false,
      registration_source: 'local',
      locale: 'en',
      units: 'imperial',
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }))

    const result = await updateMe({ locale: 'en', units: 'imperial' })

    expect(apiFetchMock).toHaveBeenCalledWith('/api/v1/me', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ locale: 'en', units: 'imperial' }),
    })
    expect(result.locale).toBe('en')
    expect(result.units).toBe('imperial')
  })
})

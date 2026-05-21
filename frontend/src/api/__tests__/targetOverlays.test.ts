import { beforeEach, describe, expect, it, vi } from 'vitest'

const { apiClientMock } = vi.hoisted(() => ({
  apiClientMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: apiClientMock,
}))

import {
  addTargetReportStatus,
  createTargetReport,
  createTargetUserContent,
  deleteTargetUserContent,
  getTargetOverlaySummary,
  getTargetReport,
  listTargetObservations,
  listTargetReports,
  listTargetUserContent,
} from '@/api/targetOverlays'

describe('target overlay api', () => {
  beforeEach(() => {
    apiClientMock.mockReset()
  })

  it('serializes summary and observation query parameters', async () => {
    apiClientMock.mockResolvedValueOnce({ summary: null }).mockResolvedValueOnce({ observations: [] })

    await getTargetOverlaySummary({ subject_kind: 'course', subject_id: 'seg-1', target_version_id: 'v1' })
    await listTargetObservations({ subject_kind: 'course', subject_id: 'seg-1', target_version_id: 'v1', limit: 10, offset: 20 })

    expect(apiClientMock).toHaveBeenNthCalledWith(1, '/target-overlays/summary?subject_kind=course&subject_id=seg-1&target_version_id=v1')
    expect(apiClientMock).toHaveBeenNthCalledWith(2, '/target-overlays/observations?subject_kind=course&subject_id=seg-1&target_version_id=v1&limit=10&offset=20')
  })

  it('serializes report list filters and report writes', async () => {
    apiClientMock.mockResolvedValueOnce({ reports: [] }).mockResolvedValueOnce({ report: { id: 'report-1' } })

    await listTargetReports({ subject_kind: 'place', subject_id: 'place-1', type: 'condition', limit: 25 })
    await createTargetReport({
      subject_kind: 'place',
      subject_id: 'place-1',
      report_type: 'condition',
      visibility: 'public',
      attribution_mode: 'named',
      description: 'Nass',
    })

    expect(apiClientMock).toHaveBeenNthCalledWith(1, '/target-overlays/reports?subject_kind=place&subject_id=place-1&limit=25&type=condition')
    expect(apiClientMock).toHaveBeenNthCalledWith(2, '/target-overlays/reports', {
      method: 'POST',
      body: {
        subject_kind: 'place',
        subject_id: 'place-1',
        report_type: 'condition',
        visibility: 'public',
        attribution_mode: 'named',
        description: 'Nass',
      },
    })
  })

  it('gets reports, appends status, and writes content', async () => {
    apiClientMock
      .mockResolvedValueOnce({ report: { id: 'report-1' } })
      .mockResolvedValueOnce({ status_event: { id: 'status-1' } })
      .mockResolvedValueOnce({ content: [] })
      .mockResolvedValueOnce({ content: { id: 'content-1' } })
      .mockResolvedValueOnce({ ok: true })

    await getTargetReport('report-1')
    await addTargetReportStatus('report-1', { status: 'resolved', reason: 'erledigt' })
    await listTargetUserContent({ subject_kind: 'collection', subject_id: 'collection-1', kind: 'comment' })
    await createTargetUserContent({ subject_kind: 'collection', subject_id: 'collection-1', content_kind: 'comment', visibility: 'public', payload: { text: 'Passt' } })
    await deleteTargetUserContent('content-1')

    expect(apiClientMock).toHaveBeenNthCalledWith(1, '/target-overlays/reports/report-1')
    expect(apiClientMock).toHaveBeenNthCalledWith(2, '/target-overlays/reports/report-1/status', {
      method: 'POST',
      body: { status: 'resolved', reason: 'erledigt' },
    })
    expect(apiClientMock).toHaveBeenNthCalledWith(3, '/target-overlays/content?subject_kind=collection&subject_id=collection-1&kind=comment')
    expect(apiClientMock).toHaveBeenNthCalledWith(4, '/target-overlays/content', {
      method: 'POST',
      body: { subject_kind: 'collection', subject_id: 'collection-1', content_kind: 'comment', visibility: 'public', payload: { text: 'Passt' } },
    })
    expect(apiClientMock).toHaveBeenNthCalledWith(5, '/target-overlays/content/content-1', { method: 'DELETE' })
  })
})

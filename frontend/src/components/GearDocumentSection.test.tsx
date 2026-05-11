import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'

import GearDocumentSection from './GearDocumentSection'
import { downloadGearDocumentBlob, fetchGearDocuments } from '@/api/gear'
import type { GearDocument } from '@/api/types'

vi.mock('@/api/gear', () => ({
  fetchGearDocuments: vi.fn(() => Promise.resolve([])),
  uploadGearDocument: vi.fn(),
  deleteGearDocument: vi.fn(),
  downloadGearDocumentBlob: vi.fn(() => Promise.resolve('blob:pdf')),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

vi.mock('react-pdf', () => ({
  pdfjs: { version: 'test', GlobalWorkerOptions: {} },
  Document: ({ children, onLoadSuccess }: { children: React.ReactNode; onLoadSuccess?: (data: { numPages: number }) => void }) => {
    onLoadSuccess?.({ numPages: 2 })
    return <div data-testid="pdf-document">{children}</div>
  },
  Page: ({ width, renderTextLayer, renderAnnotationLayer }: { width: number; renderTextLayer: boolean; renderAnnotationLayer: boolean }) => (
    <div
      data-testid="pdf-page"
      data-width={String(width)}
      data-text-layer={String(renderTextLayer)}
      data-annotation-layer={String(renderAnnotationLayer)}
    />
  ),
}))

function renderSection() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <GearDocumentSection gearId="gear-1" />
    </QueryClientProvider>,
  )
}

describe('GearDocumentSection', () => {
  it('TestPdfPreviewHasZoomPrintDownloadTextLayerAndResizeHandle', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchGearDocuments).mockResolvedValue([
      gearDocumentFixture({
        id: 'pdf-1',
        content_type: 'application/pdf',
        original_filename: 'manual.pdf',
      }),
    ])
    vi.mocked(downloadGearDocumentBlob).mockResolvedValue('blob:pdf-1')

    renderSection()

    await user.click(await screen.findByText('manual.pdf'))

    expect(await screen.findByRole('separator', { name: 'Vorschau-Breite ändern' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '-' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '+' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Drucken' })).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Herunterladen' }).length).toBeGreaterThan(0)

    await waitFor(() => {
      expect(screen.getByTestId('pdf-page')).toHaveAttribute('data-text-layer', 'true')
      expect(screen.getByTestId('pdf-page')).toHaveAttribute('data-annotation-layer', 'true')
    })
  })
})

function gearDocumentFixture(overrides: Partial<GearDocument> = {}): GearDocument {
  return {
    id: 'doc-1',
    gear_id: 'gear-1',
    component_id: null,
    service_event_id: null,
    doc_type: 'manual',
    title: null,
    original_filename: 'doc.pdf',
    content_type: 'application/pdf',
    file_size_bytes: 1024,
    file_hash: 'hash',
    processing_status: 'processed',
    created_at: '2026-05-11T00:00:00Z',
    ...overrides,
  }
}

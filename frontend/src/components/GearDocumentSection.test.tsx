import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import GearDocumentSection from './GearDocumentSection'
import { downloadGearDocumentBlob, fetchGearDocuments } from '@/api/gear'
import type { GearDocument } from '@/api/types'
import { i18n } from '@/i18n/i18n'

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
  beforeEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('uses catalog-only generic placeholders for document upload fields', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchGearDocuments).mockResolvedValue([])

    const { container } = renderSection()

    await user.click(await screen.findByRole('button', { name: 'Dokumente' }))
    await user.click(await screen.findByRole('button', { name: '+ Dokument hinzufügen' }))

    expect(container.querySelector('input[type="text"]')).toHaveAttribute('placeholder', 'z. B. Kaufbeleg')
    expect(container.querySelector('input[type="number"]')).toHaveAttribute('placeholder', '0.00')

    const source = readFileSync(resolve(process.cwd(), 'src/components/GearDocumentSection.tsx'), 'utf8')
    expect(source).not.toMatch(/gearPages\.documents\.placeholders\.(?:title|amount)['"`],\s*\{\s*defaultValue:/)
  })

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

  it('uses generic labels for unknown document type and processing status values', async () => {
    vi.mocked(fetchGearDocuments).mockResolvedValue([
      gearDocumentFixture({
        doc_type: 'backend_private_doc_type' as unknown as GearDocument['doc_type'],
        processing_status: 'backend_private_status' as unknown as GearDocument['processing_status'],
      }),
    ])

    renderSection()

    expect(await screen.findByText('doc.pdf')).toBeInTheDocument()
    expect(screen.getByText('Unbekannter Dokumenttyp')).toBeInTheDocument()
    expect(screen.getByText('Unbekannter Status')).toBeInTheDocument()
    expect(screen.queryByText('backend_private_doc_type')).not.toBeInTheDocument()
    expect(screen.queryByText('backend_private_status')).not.toBeInTheDocument()
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

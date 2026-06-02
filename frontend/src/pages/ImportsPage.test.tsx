import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ImportsPage from './ImportsPage'
import type { ImportLogEntry, ImportLogParams, ImportLogResponse } from '@/api/imports'
import type { ImportQueueItem } from '@/api/importQueue'
import { i18n } from '@/i18n/i18n'

const mocks = vi.hoisted(() => ({
  getImports: vi.fn<(params?: ImportLogParams) => Promise<ImportLogResponse>>(),
  fetchImportQueue: vi.fn<(status?: string) => Promise<ImportQueueItem[]>>(),
  fetchImportQueueCount: vi.fn<() => Promise<number>>(),
  fetchGear: vi.fn<() => Promise<Array<{ id: string; name: string; is_retired?: boolean }>>>(),
  getCapabilities: vi.fn<() => Promise<{ import_path_enabled: boolean }>>(),
}))

vi.mock('@/api/imports', () => ({
  getImports: mocks.getImports,
}))

vi.mock('@/api/importQueue', async () => {
  const actual = await vi.importActual<typeof import('@/api/importQueue')>('@/api/importQueue')
  return {
    ...actual,
    fetchImportQueue: mocks.fetchImportQueue,
    fetchImportQueueCount: mocks.fetchImportQueueCount,
    assignQueueItem: vi.fn(),
    patchQueueItem: vi.fn(),
    resolvePdfQueueItem: vi.fn(),
  }
})

vi.mock('@/api/gear', () => ({
  fetchGear: mocks.fetchGear,
}))

vi.mock('@/api/routes', () => ({
  getCapabilities: mocks.getCapabilities,
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

vi.mock('@/components/AuthenticatedImage', () => ({
  default: ({ src, alt, className }: { src: string; alt?: string; className?: string }) => (
    <img src={src} alt={alt} className={className} />
  ),
}))

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location">{location.pathname + location.search}</div>
}

function renderImportsPage(initialEntry = '/imports') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <ImportsPage />
        <LocationProbe />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function makeLogEntry(overrides: Partial<ImportLogEntry>): ImportLogEntry {
  return {
    id: 'log-1',
    user_id: 'user-1',
    source_platform: 'upload',
    filename: 'activity.fit',
    status: 'completed',
    created_at: '2026-05-13T10:00:00Z',
    updated_at: '2026-05-13T10:00:00Z',
    ...overrides,
  }
}

const reviewItem: ImportQueueItem = {
  id: 'queue-1',
  item_type: 'image_no_exif',
  status: 'pending',
  filename: 'photo-without-date.jpg',
  file_size: 2048,
  created_at: '2026-05-13T09:00:00Z',
}

const sportConflictItem: ImportQueueItem = {
  id: 'queue-sport-1',
  item_type: 'sport_conflict',
  status: 'pending',
  filename: 'activity.fit',
  created_at: '2026-04-13T09:00:00Z',
  metadata: {
    activity_id: 'activity-1',
    platform_sport: 'training',
    file_sport: 'activity',
    platform_source: 'strava',
    file_source: 'fit',
  },
}

const pdfItem: ImportQueueItem = {
  id: 'queue-pdf-1',
  item_type: 'pdf_untyped',
  status: 'pending',
  filename: 'rechnung.pdf',
  created_at: '2026-05-13T09:00:00Z',
}

describe('ImportsPage', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('de')
    vi.clearAllMocks()
    mocks.fetchImportQueue.mockResolvedValue([])
    mocks.fetchImportQueueCount.mockResolvedValue(0)
    mocks.fetchGear.mockResolvedValue([{ id: 'gear-1', name: 'Gravel Bike', is_retired: false }])
    mocks.getCapabilities.mockResolvedValue({ import_path_enabled: false })
    mocks.getImports.mockImplementation(async (params = {}) => {
      if (params.status === 'pending,processing') {
        return { entries: [], total: 0 }
      }
      return { entries: [], total: 0 }
    })
  })

  it('opens Zu prüfen by default when manual review items are pending', async () => {
    mocks.fetchImportQueueCount.mockResolvedValue(1)
    mocks.fetchImportQueue.mockResolvedValue([reviewItem])

    renderImportsPage('/imports')

    await waitFor(() => {
      expect(screen.getByRole('tab', { name: /Zu prüfen/i })).toHaveAttribute('aria-selected', 'true')
    })
    expect(await screen.findByText('photo-without-date.jpg')).toBeInTheDocument()
  })

  it('opens Importieren by default when no manual review item is pending', async () => {
    renderImportsPage('/imports')

    expect(await screen.findByRole('tab', { name: /Importieren/i })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByText('Dateien importieren')).toBeInTheDocument()
  })

  it.each([
    ['/imports?source=garmin', 'Garmin-Export-Verzeichnis'],
    ['/imports?source=strava', 'Strava-Export-Verzeichnis'],
    ['/imports?source=komoot', 'Anmelden & Scannen'],
  ])('opens the requested source wizard from %s', async (route, visibleText) => {
    mocks.getCapabilities.mockResolvedValue({ import_path_enabled: true })

    renderImportsPage(route)

    expect(await screen.findByRole('tab', { name: /Importieren/i })).toHaveAttribute('aria-selected', 'true')
    expect(await screen.findByText(visibleText)).toBeVisible()
  })

  it('updates the URL when source cards and view tabs are clicked', async () => {
    const user = userEvent.setup()

    renderImportsPage('/imports?view=import')

    await user.click(await screen.findByRole('button', { name: /^Komoot/i }))
    expect(screen.getByTestId('location')).toHaveTextContent('/imports?view=import&source=komoot')

    await user.click(screen.getByRole('tab', { name: /Warteschlange/i }))
    expect(screen.getByTestId('location')).toHaveTextContent('/imports?view=queue')

    await user.click(screen.getByRole('tab', { name: /Verlauf/i }))
    expect(screen.getByTestId('location')).toHaveTextContent('/imports?view=history')
  })

  it('renders the active queue from ?view=queue', async () => {
    mocks.getImports.mockImplementation(async (params = {}) => {
      if (params.status === 'pending,processing') {
        return {
          entries: [
            makeLogEntry({
              id: 'active-log',
              filename: 'active-import.fit',
              status: 'processing',
              metadata: {
                steps: [
                  { key: 'upload', label: 'Upload', status: 'completed' },
                  { key: 'text_extraction', label: 'Texterkennung', status: 'pending' },
                  { key: 'markdown', label: 'Markdown', status: 'pending' },
                ],
              },
            }),
          ],
          total: 1,
        }
      }
      return { entries: [], total: 0 }
    })

    renderImportsPage('/imports?view=queue')

    expect(await screen.findByText('Import-Warteschlange')).toBeInTheDocument()
    expect(await screen.findByText('active-import.fit')).toBeInTheDocument()
    expect(await screen.findByText('Schritt 2/3: Texterkennung')).toBeInTheDocument()
  })

  it('localizes active queue step labels instead of rendering backend labels', async () => {
    await i18n.changeLanguage('en')
    mocks.getImports.mockImplementation(async (params = {}) => {
      if (params.status === 'pending,processing') {
        return {
          entries: [
            makeLogEntry({
              id: 'active-log',
              filename: 'active-import.fit',
              status: 'processing',
              metadata: {
                steps: [
                  { key: 'upload', label: 'Upload', status: 'completed' },
                  { key: 'text_extraction', label: 'Texterkennung', status: 'pending' },
                  { key: 'markdown', label: 'Markdown', status: 'pending' },
                ],
              },
            }),
          ],
          total: 1,
        }
      }
      return { entries: [], total: 0 }
    })

    renderImportsPage('/imports?view=queue')

    expect(await screen.findByText('Import queue')).toBeInTheDocument()
    expect(await screen.findByText('active-import.fit')).toBeInTheDocument()
    expect(await screen.findByText('Step 2/3: Text extraction')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('Texterkennung')
  })

  it('preserves source wizard state when switching sources', async () => {
    mocks.getCapabilities.mockResolvedValue({ import_path_enabled: true })
    const user = userEvent.setup()

    renderImportsPage('/imports?source=garmin')

    const input = await screen.findByPlaceholderText('/pfad/zum/garmin/export')
    await user.type(input, '/tmp/garmin-export')
    await user.click(screen.getByRole('button', { name: /^Strava/i }))
    await user.click(screen.getByRole('button', { name: /^Garmin/i }))

    expect(screen.getByPlaceholderText('/pfad/zum/garmin/export')).toHaveValue('/tmp/garmin-export')
  })

  it('renders image_no_exif as missing capture date with thumbnail', async () => {
    mocks.fetchImportQueueCount.mockResolvedValue(1)
    mocks.fetchImportQueue.mockResolvedValue([reviewItem])

    renderImportsPage('/imports?view=review')

    expect(await screen.findByText('Kein Aufnahmedatum')).toBeInTheDocument()
    expect(screen.getByText(/EXIF-Aufnahmedatum fehlt/i)).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'photo-without-date.jpg' })).toHaveAttribute(
      'src',
      '/api/v1/import-queue/queue-1/thumb',
    )
  })

  it('localizes manual review queue labels in English', async () => {
    await i18n.changeLanguage('en')
    mocks.fetchImportQueueCount.mockResolvedValue(1)
    mocks.fetchImportQueue.mockResolvedValue([reviewItem])

    renderImportsPage('/imports?view=review')

    expect(await screen.findByText('Missing capture date')).toBeInTheDocument()
    expect(screen.getByText(/EXIF capture date is missing/i)).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Select activity...')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Assign' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Discard' })).toBeInTheDocument()
    expect(screen.queryByText('Kein Aufnahmedatum')).not.toBeInTheDocument()
  })

  it('opens a larger preview for image queue items', async () => {
    mocks.fetchImportQueueCount.mockResolvedValue(1)
    mocks.fetchImportQueue.mockResolvedValue([reviewItem])
    const user = userEvent.setup()

    renderImportsPage('/imports?view=review')

    await user.click(await screen.findByRole('button', { name: /Bildvorschau von photo-without-date\.jpg vergrößern/i }))

    expect(screen.getByRole('dialog', { name: 'Bildvorschau' })).toBeInTheDocument()
    expect(
      screen.getAllByRole('img', { name: 'photo-without-date.jpg' }).some(img =>
        img.getAttribute('src') === '/api/v1/import-queue/queue-1/preview',
      ),
    ).toBe(true)
  })

  it('keeps sport conflict resolution controls in the central review tab', async () => {
    mocks.fetchImportQueueCount.mockResolvedValue(1)
    mocks.fetchImportQueue.mockResolvedValue([sportConflictItem])

    renderImportsPage('/imports?view=review')

    expect(await screen.findByText('Sport-Typ-Konflikt')).toBeInTheDocument()
    expect(screen.getByText(/Strava/i)).toBeInTheDocument()
    expect(screen.getByText(/FIT-Datei/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Zur Aktivität/i })).toHaveAttribute('href', '/activities/activity-1')
    expect(screen.queryByRole('button', { name: /Aktivität zuordnen/i })).not.toBeInTheDocument()
  })

  it('requires a gear selection before resolving queued PDFs', async () => {
    mocks.fetchImportQueueCount.mockResolvedValue(1)
    mocks.fetchImportQueue.mockResolvedValue([pdfItem])

    renderImportsPage('/imports?view=review')

    expect(await screen.findByText('PDF ohne Kategorie')).toBeInTheDocument()
    expect(screen.getByText(/Wähle die Dokumentart und die Ausrüstung/i)).toBeInTheDocument()
    expect(await screen.findByRole('option', { name: 'Gravel Bike' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Kategorisieren' })).toBeDisabled()
  })

  it('renders unknown manual review item types as generic localized labels', async () => {
    mocks.fetchImportQueueCount.mockResolvedValue(1)
    mocks.fetchImportQueue.mockResolvedValue([
      {
        id: 'queue-future-1',
        item_type: 'future_item' as ImportQueueItem['item_type'],
        status: 'pending',
        filename: 'future-file.bin',
        created_at: '2026-05-13T09:00:00Z',
      },
    ])

    renderImportsPage('/imports?view=review')

    expect(await screen.findByText('Unbekannter Eintragstyp')).toBeInTheDocument()
    expect(screen.getByText('future-file.bin')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('future_item')
  })

  it('renders queued history rows as Zu prüfen with review link', async () => {
    mocks.getImports.mockImplementation(async (params = {}) => {
      if (params.status === 'pending,processing') {
        return { entries: [], total: 0 }
      }
      return {
        entries: [
          makeLogEntry({
            id: 'log-queued',
            filename: 'queued-photo.jpg',
            status: 'queued',
            import_type: 'photo',
          }),
        ],
        total: 1,
      }
    })

    renderImportsPage('/imports?view=history')

    expect(await screen.findByText('queued-photo.jpg')).toBeInTheDocument()
    const reviewLinks = screen.getAllByRole('link', { name: /Zu prüfen/i })
    expect(reviewLinks.some(link => link.getAttribute('href') === '/imports?view=review')).toBe(true)
  })

  it('renders unknown import history codes as generic localized labels', async () => {
    mocks.getImports.mockImplementation(async (params = {}) => {
      if (params.status === 'pending,processing') {
        return { entries: [], total: 0 }
      }
      return {
        entries: [
          makeLogEntry({
            id: 'log-unknown',
            filename: 'future-format.bin',
            source_platform: 'future_sync',
            status: 'paused_backend_state',
            import_type: 'future_type',
          }),
        ],
        total: 1,
      }
    })

    renderImportsPage('/imports?view=history')

    expect(await screen.findByText('future-format.bin')).toBeInTheDocument()
    expect(screen.getByText('Unbekannte Quelle')).toBeInTheDocument()
    expect(screen.getByText('Unbekannter Status')).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('future_sync')
    expect(document.body).not.toHaveTextContent('paused_backend_state')
  })

  it('keeps explicit view query parameters ahead of the automatic default', async () => {
    mocks.fetchImportQueueCount.mockResolvedValue(2)
    mocks.fetchImportQueue.mockResolvedValue([reviewItem])

    renderImportsPage('/imports?view=import')

    await waitFor(() => {
      expect(screen.getByRole('tab', { name: /Importieren/i })).toHaveAttribute('aria-selected', 'true')
    })
    expect(screen.queryByText('photo-without-date.jpg')).not.toBeInTheDocument()
  })
})

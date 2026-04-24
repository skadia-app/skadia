import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'

// ── Mock @tanstack/react-query ────────────────────────────────────────────────

type QueryOptions = {
  queryKey: unknown[]
  queryFn?: () => unknown
  staleTime?: number
}

// Default: both queries return undefined (loading / no data)
let mockSchemas: unknown = undefined
let mockContext: unknown = undefined

vi.mock('@tanstack/react-query', () => ({
  useQuery: ({ queryKey }: QueryOptions) => {
    if (Array.isArray(queryKey) && queryKey[0] === 'file-schemas-context') {
      return { data: mockContext, isLoading: false, isError: false }
    }
    // file-schemas query
    return { data: mockSchemas, isLoading: false, isError: false }
  },
  useMutation: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
  useQueryClient: () => ({
    invalidateQueries: vi.fn(() => Promise.resolve()),
  }),
}))

// ── Mock api/shadowfs ─────────────────────────────────────────────────────────

vi.mock('@/api/shadowfs', () => ({
  getFileSchemas: vi.fn(),
  getFileSchemasContext: vi.fn(),
  updateFileSchema: vi.fn(),
  getLibraryDir: vi.fn(),
  updateLibraryDir: vi.fn(),
  FILE_TYPE_LABELS: {
    activity: 'Aktivitäten',
    media: 'Medien',
    document: 'Dokumente',
    health: 'Gesundheit',
    route: 'Routen',
  },
}))

// ── Mock @/lib/toast ──────────────────────────────────────────────────────────

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

// ── Mock @/components/ui/SettingsSection ──────────────────────────────────────

vi.mock('@/components/ui/SettingsSection', () => ({
  SettingsSection: ({ children }: { title: string; description?: string; children: React.ReactNode }) => (
    <div data-testid="settings-section">{children}</div>
  ),
}))

// ── Mock @/lib/libraryLabels ──────────────────────────────────────────────────
// Use importOriginal so FILE_TYPE_ORDER, TYPE_VARIABLES, EXAMPLE_VALUES etc.
// are all included — we only need to override LIB_LABELS partially.

vi.mock('@/lib/libraryLabels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/libraryLabels')>()
  return {
    ...actual,
    LIB_LABELS: {
      ...actual.LIB_LABELS,
      schemaAdminManaged: 'Diese Einstellung wird vom Administrator verwaltet.',
      schemaS3Inactive: 'Pfad-Einstellungen sind bei S3-Storage inaktiv.',
      libraryDirPrefix: 'Pfad-Präfix:',
      libraryDirManagedByAdmin: 'Wird vom Administrator verwaltet.',
      libraryDirNotApplicableS3: 'Nicht anwendbar bei S3-Storage.',
    },
  }
})

// ── Component under test ──────────────────────────────────────────────────────

import FileSchemaSettings from '../FileSchemaSettings'

// ── Helpers ───────────────────────────────────────────────────────────────────

function makeSchema(fileType: string, schema: string) {
  return { file_type: fileType, schema, effective_schema: schema, is_user_override: false }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('FileSchemaSettings — editable state from context', () => {
  beforeEach(() => {
    // Reset to sane defaults
    mockSchemas = [
      makeSchema('activity', '{year}/{month}/{sport}/{title}'),
      makeSchema('media', '{year}/{month}/{title}'),
      makeSchema('document', '{year}/{category}/{title}'),
      makeSchema('health', '{year}/{month}/{type}'),
      makeSchema('route', '{sport}/{title}'),
    ]
    mockContext = undefined
  })

  it('renders schema sections in editable state by default (no context)', () => {
    render(<FileSchemaSettings />)
    // All collapsible headers should be present
    expect(screen.getByText('Aktivitäten')).toBeInTheDocument()
    expect(screen.getByText('Medien')).toBeInTheDocument()
  })

  it('disables schema inputs when editable=false (admin_locked reason)', () => {
    mockContext = { editable: false, backend: 'local', admin_managed_reason: 'admin_locked' }
    render(<FileSchemaSettings />)

    // Open a section by clicking its header via fireEvent (triggers React event system)
    const activityHeader = screen.getByText('Aktivitäten').closest('button')!
    fireEvent.click(activityHeader)

    // The input inside the expanded panel should be disabled
    const inputs = document.querySelectorAll('input[type="text"]')
    const disabledInputs = Array.from(inputs).filter(
      (el) => (el as HTMLInputElement).disabled,
    )
    expect(disabledInputs.length).toBeGreaterThan(0)

    // The admin-managed hint should be rendered
    expect(screen.getAllByText('Diese Einstellung wird vom Administrator verwaltet.').length).toBeGreaterThan(0)
  })

  it('shows s3_backend hint when editable=false and reason=s3_backend', () => {
    mockContext = { editable: false, backend: 's3', admin_managed_reason: 's3_backend' }
    render(<FileSchemaSettings />)

    // Open a section via fireEvent
    const activityHeader = screen.getByText('Aktivitäten').closest('button')!
    fireEvent.click(activityHeader)

    expect(screen.getAllByText('Pfad-Einstellungen sind bei S3-Storage inaktiv.').length).toBeGreaterThan(0)
  })

  it('does not disable inputs when editable=true', () => {
    mockContext = { editable: true, backend: 'local', admin_managed_reason: null }
    render(<FileSchemaSettings />)

    // Open a section via fireEvent
    const activityHeader = screen.getByText('Aktivitäten').closest('button')!
    fireEvent.click(activityHeader)

    const inputs = document.querySelectorAll('input[type="text"]')
    const disabledInputs = Array.from(inputs).filter(
      (el) => (el as HTMLInputElement).disabled,
    )
    expect(disabledInputs.length).toBe(0)

    // No hint text rendered
    expect(screen.queryByText('Diese Einstellung wird vom Administrator verwaltet.')).not.toBeInTheDocument()
  })
})

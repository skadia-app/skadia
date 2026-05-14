// frontend/src/components/AdminLibrarySettingsTab.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ToastProvider } from '@/lib/toast'
import { AdminLibrarySettingsTab } from './AdminLibrarySettingsTab'
import type { AdminLibrarySettings, BulkRenameResponse } from '@/api/shadowfs'

// ── Mock API ─────────────────────────────────────────────────────────────────

const mockGetAdminLibrarySettings = vi.fn()
const mockAdminRenameBulk = vi.fn()

vi.mock('@/api/shadowfs', async () => {
  const actual = await vi.importActual<typeof import('@/api/shadowfs')>('@/api/shadowfs')
  return {
    ...actual,
    getAdminLibrarySettings: (...args: unknown[]) => mockGetAdminLibrarySettings(...args),
    updateAdminLibrarySettings: vi.fn().mockResolvedValue({ status: 'ok' }),
    adminRenameBulk: (...args: unknown[]) => mockAdminRenameBulk(...args),
  }
})

// ── Fixtures ──────────────────────────────────────────────────────────────────

const MOCK_SETTINGS: AdminLibrarySettings = {
  user_can_edit_dir_name: true,
  user_can_edit_schemas: false,
  default_schemas: {
    activity: '{user_id}/activity',
    media: '{user_id}/media',
    document: '{user_id}/document',
    health: '{user_id}/health',
    route: '{user_id}/route',
  },
  backend: 'local',
  local_actions_available: true,
  admin_managed_reason: null,
}

// ── Wrapper ──────────────────────────────────────────────────────────────────

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ToastProvider>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </ToastProvider>
    )
  }
  return Wrapper
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe('AdminLibrarySettingsTab', () => {
  beforeEach(() => {
    mockGetAdminLibrarySettings.mockReset()
    mockAdminRenameBulk.mockReset()
    mockGetAdminLibrarySettings.mockResolvedValue(MOCK_SETTINGS)
  })

  it('renders 5 accordion rows for file types', async () => {
    render(<AdminLibrarySettingsTab />, { wrapper: makeWrapper() })

    // Wait for data to load — accordion headers appear as buttons with German labels
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Aktivitäten/i })).toBeInTheDocument()
    })

    // All 5 file type accordion headers should be present
    expect(screen.getByRole('button', { name: /Aktivitäten/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Medien/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Dokumente/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Gesundheit/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Routen/i })).toBeInTheDocument()
  })

  it('shows schema input when accordion row is expanded', async () => {
    render(<AdminLibrarySettingsTab />, { wrapper: makeWrapper() })

    // Wait for data to load
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Aktivitäten/i })).toBeInTheDocument()
    })

    // Click to expand the activity row
    fireEvent.click(screen.getByRole('button', { name: /Aktivitäten/i }))

    // Input should now be visible with the saved value
    await waitFor(() => {
      expect(screen.getByDisplayValue('{user_id}/activity')).toBeInTheDocument()
    })

    // Only 1 textbox visible (other rows are collapsed)
    expect(screen.getAllByRole('textbox')).toHaveLength(1)
  })

  it('hides local schema and rename controls in s3 mode', async () => {
    mockGetAdminLibrarySettings.mockResolvedValue({
      ...MOCK_SETTINGS,
      backend: 's3',
      local_actions_available: false,
      admin_managed_reason: 's3_backend',
    } as AdminLibrarySettings)

    render(<AdminLibrarySettingsTab />, { wrapper: makeWrapper() })

    expect(await screen.findByText(/S3-Storage/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/User dürfen Verzeichnisname bearbeiten/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/User dürfen Schema-Templates bearbeiten/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Aktivitäten/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Rename auf alle User anwenden/i })).not.toBeInTheDocument()
  })

  it('bulk-rename confirm-dialog prevents double-click', async () => {
    // Bulk rename returns in-progress on second call
    mockAdminRenameBulk.mockResolvedValue({
      status: 'bulk_rename_in_progress',
    } satisfies BulkRenameResponse)

    render(<AdminLibrarySettingsTab />, { wrapper: makeWrapper() })

    // Wait until data loads and expand the activity row
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Aktivitäten/i })).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: /Aktivitäten/i }))

    // Wait for the bulk-rename trigger button to appear inside the expanded row
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Rename auf alle User anwenden/i })).toBeInTheDocument(),
    )

    // Click the bulk-rename trigger button
    const triggerBtn = screen.getByRole('button', { name: /Rename auf alle User anwenden/i })
    fireEvent.click(triggerBtn)

    // Confirm dialog should open
    await waitFor(() =>
      expect(screen.getByText(/Bulk-Rename bestätigen/i)).toBeInTheDocument(),
    )

    // The confirm button in the modal has a distinct label — no ambiguity with the trigger
    const confirmBtn = screen.getByRole('button', { name: /Bulk-Rename starten/i })
    fireEvent.click(confirmBtn)

    // After the mutation resolves (status = bulk_rename_in_progress),
    // the modal should close (setBulkConfirm(false) in onSuccess)
    await waitFor(() =>
      expect(screen.queryByText(/Bulk-Rename bestätigen/i)).not.toBeInTheDocument(),
    )

    // Should have called the API exactly once
    expect(mockAdminRenameBulk).toHaveBeenCalledWith('activity')
    expect(mockAdminRenameBulk).toHaveBeenCalledTimes(1)
  })
})

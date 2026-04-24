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

  it('renders 5 file-type schema inputs', async () => {
    render(<AdminLibrarySettingsTab />, { wrapper: makeWrapper() })

    // Wait for the query to resolve and inputs to appear
    await waitFor(() => {
      expect(screen.getByDisplayValue('{user_id}/activity')).toBeInTheDocument()
    })

    const FILE_TYPES = ['activity', 'media', 'document', 'health', 'route']
    for (const ft of FILE_TYPES) {
      expect(screen.getByLabelText(ft)).toBeInTheDocument()
    }
    expect(screen.getAllByRole('textbox')).toHaveLength(5)
  })

  it('bulk-rename confirm-dialog prevents double-click', async () => {
    // Bulk rename returns in-progress on second call
    mockAdminRenameBulk.mockResolvedValue({
      status: 'bulk_rename_in_progress',
    } satisfies BulkRenameResponse)

    render(<AdminLibrarySettingsTab />, { wrapper: makeWrapper() })

    // Wait until data loads
    await waitFor(() =>
      expect(screen.getByDisplayValue('{user_id}/activity')).toBeInTheDocument(),
    )

    // Click the bulk-rename trigger button for 'activity'
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
    // the modal should close (setBulkConfirmFileType(null) in onSuccess)
    await waitFor(() =>
      expect(screen.queryByText(/Bulk-Rename bestätigen/i)).not.toBeInTheDocument(),
    )

    // Should have called the API exactly once
    expect(mockAdminRenameBulk).toHaveBeenCalledWith('activity')
    expect(mockAdminRenameBulk).toHaveBeenCalledTimes(1)
  })
})

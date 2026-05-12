import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AdminStorageLimitsSection } from './AdminStorageLimitsSection'
import { getAdminSettings, putAdminSettings } from '@/api/adminSettings'

vi.mock('@/api/adminSettings', () => ({
  getAdminSettings: vi.fn(),
  putAdminSettings: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

function renderSection() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <AdminStorageLimitsSection />
    </QueryClientProvider>,
  )
}

describe('AdminStorageLimitsSection', () => {
  beforeEach(() => {
    vi.mocked(putAdminSettings).mockResolvedValue({ status: 'ok', changed: [] })
  })

  it('keeps persisted zero distinct from empty default fields', async () => {
    vi.mocked(getAdminSettings).mockResolvedValue({
      limits: {
        max_upload_bytes: { value: '52428800', source: 'default' },
        max_user_storage_bytes: { value: '0', source: 'db' },
        max_activity_photos_per_activity: { value: '100', source: 'default' },
        max_route_photos_per_route: { value: '0', source: 'db' },
      },
      library: {
        keep_versions: { value: '1', source: 'default' },
      },
    })

    renderSection()

    await screen.findByText('Max. Benutzer-Speicher')
    const inputs = screen.getAllByRole('spinbutton') as HTMLInputElement[]
    expect(inputs[0].value).toBe('')
    expect(inputs[1].value).toBe('0')
    expect(inputs[2].value).toBe('')
    expect(inputs[3].value).toBe('0')

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => {
      expect(putAdminSettings).toHaveBeenCalledWith(expect.objectContaining({
        'limits.max_upload_bytes': null,
        'limits.max_user_storage_bytes': '0',
        'limits.max_activity_photos_per_activity': null,
        'limits.max_route_photos_per_route': '0',
      }))
    })
  })
})


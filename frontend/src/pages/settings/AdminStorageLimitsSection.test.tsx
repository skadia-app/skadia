import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AdminStorageLimitsSection } from './AdminStorageLimitsSection'
import { getAdminSettings, putAdminSettings } from '@/api/adminSettings'

const { addToastMock } = vi.hoisted(() => ({
  addToastMock: vi.fn(),
}))

vi.mock('@/api/adminSettings', () => ({
  getAdminSettings: vi.fn(),
  putAdminSettings: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: addToastMock }),
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
    vi.clearAllMocks()
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
  it('rejects decimal storage limits instead of truncating them to unlimited', async () => {
    vi.mocked(getAdminSettings).mockResolvedValue({
      limits: {
        max_upload_bytes: { value: '52428800', source: 'default' },
        max_user_storage_bytes: { value: '10737418240', source: 'db' },
        max_activity_photos_per_activity: { value: '100', source: 'default' },
        max_route_photos_per_route: { value: '10', source: 'default' },
      },
      library: {
        keep_versions: { value: '1', source: 'default' },
      },
    })

    renderSection()

    await screen.findByText('Max. Benutzer-Speicher')
    const inputs = screen.getAllByRole('spinbutton') as HTMLInputElement[]
    await waitFor(() => expect(inputs[1].value).toBe('10'))
    fireEvent.change(inputs[1], { target: { value: '0.5' } })
    expect(inputs[1].value).toBe('0.5')
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(putAdminSettings).not.toHaveBeenCalled()
    expect(addToastMock).toHaveBeenCalledWith(expect.objectContaining({
      type: 'error',
      title: 'Ungültiger Max Benutzer-Speicher',
    }))
  })
})

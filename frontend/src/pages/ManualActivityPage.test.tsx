import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import '@testing-library/jest-dom'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { createManualActivity } from '@/api/manualActivity'
import { i18n } from '@/i18n/i18n'
import { ToastProvider } from '@/lib/toast'
import ManualActivityPage from './ManualActivityPage'

const mockUserPrefs = vi.hoisted((): { locale: 'de' | 'en'; units: 'metric' | 'imperial' } => ({
  locale: 'en',
  units: 'imperial',
}))

const mockNavigate = vi.hoisted(() => vi.fn())

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

vi.mock('@/api/manualActivity', () => ({
  createManualActivity: vi.fn(async () => ({ id: 'activity-1', status: 'ok' })),
}))

vi.mock('@/api/gear', () => ({
  fetchGear: vi.fn(async () => []),
}))

vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn(async () => []),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

function renderManualActivityPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <MemoryRouter>
          <ManualActivityPage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

function numericField(label: string): HTMLInputElement {
  const field = screen.getByText(label).closest('div')
  if (!field) throw new Error(`Field not found: ${label}`)
  return within(field).getByRole('spinbutton') as HTMLInputElement
}

describe('ManualActivityPage units', () => {
  beforeEach(async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    mockNavigate.mockReset()
    vi.mocked(createManualActivity).mockClear()
    await i18n.changeLanguage('en')
  })

  it('uses imperial distance and elevation inputs while preserving the meter API contract', async () => {
    renderManualActivityPage()
    const user = userEvent.setup()

    const distanceField = screen.getByText('Distance').closest('div')
    const ascentField = screen.getByText('Ascent').closest('div')
    const descentField = screen.getByText('Descent').closest('div')

    expect(distanceField).not.toBeNull()
    expect(ascentField).not.toBeNull()
    expect(descentField).not.toBeNull()
    expect(within(distanceField!).getByText('mi')).toBeInTheDocument()
    expect(within(ascentField!).getByText('ft')).toBeInTheDocument()
    expect(within(descentField!).getByText('ft')).toBeInTheDocument()

    await user.type(numericField('Distance'), '2')
    await user.type(numericField('Ascent'), '328.084')
    await user.type(numericField('Descent'), '164.042')
    await user.click(screen.getByRole('button', { name: 'Create activity' }))

    await waitFor(() => expect(createManualActivity).toHaveBeenCalledTimes(1))
    const payload = vi.mocked(createManualActivity).mock.calls[0]?.[0]
    expect(payload?.distance_m).toBeCloseTo(3218.688, 3)
    expect(payload?.elevation_gain_m).toBeCloseTo(100, 3)
    expect(payload?.elevation_loss_m).toBeCloseTo(50, 3)
  })

  it('keeps metric distance and elevation inputs on kilometer and meter units', async () => {
    mockUserPrefs.units = 'metric'
    renderManualActivityPage()
    const user = userEvent.setup()

    const distanceField = screen.getByText('Distance').closest('div')
    const ascentField = screen.getByText('Ascent').closest('div')
    const descentField = screen.getByText('Descent').closest('div')

    expect(distanceField).not.toBeNull()
    expect(ascentField).not.toBeNull()
    expect(descentField).not.toBeNull()
    expect(within(distanceField!).getByText('km')).toBeInTheDocument()
    expect(within(ascentField!).getByText('m')).toBeInTheDocument()
    expect(within(descentField!).getByText('m')).toBeInTheDocument()

    await user.type(numericField('Distance'), '3.5')
    await user.type(numericField('Ascent'), '120')
    await user.type(numericField('Descent'), '30')
    await user.click(screen.getByRole('button', { name: 'Create activity' }))

    await waitFor(() => expect(createManualActivity).toHaveBeenCalledTimes(1))
    const payload = vi.mocked(createManualActivity).mock.calls[0]?.[0]
    expect(payload?.distance_m).toBeCloseTo(3500, 3)
    expect(payload?.elevation_gain_m).toBeCloseTo(120, 3)
    expect(payload?.elevation_loss_m).toBeCloseTo(30, 3)
  })
})

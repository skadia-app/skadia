import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RouteSuggestionDialog from '../RouteSuggestionDialog'
import { ToastProvider } from '@/lib/toast'
import { suggestRoute } from '@/api/routes'

const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({ locale: 'de', units: 'metric' }))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

vi.mock('@/api/routingProfiles', () => ({
  fetchAllProfiles: vi.fn().mockResolvedValue({
    profiles: [
      {
        id: 'profile-cycling',
        name: 'trekking',
        engine: 'badrouter',
        base_profile: 'trekking',
        sport_types: ['cycling'],
        predicates: [{ sport: 'cycling' }],
        source: 'system',
        config: null,
        editable: false,
      },
    ],
  }),
}))

vi.mock('@/api/routes', () => ({
  suggestRoute: vi.fn().mockResolvedValue({ suggestions: [], partial_search: false }),
}))

const mockSuggestRoute = suggestRoute as ReturnType<typeof vi.fn>

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <RouteSuggestionDialog
          start={[10, 48]}
          onClose={vi.fn()}
          onResult={vi.fn()}
        />
      </ToastProvider>
    </QueryClientProvider>,
  )
}

async function clickSubmit() {
  const button = await screen.findByRole('button', { name: 'Route vorschlagen' })
  await waitFor(() => expect(button).toBeEnabled())
  fireEvent.click(button)
}

describe('RouteSuggestionDialog modifier controls', () => {
  beforeEach(() => {
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
    mockSuggestRoute.mockClear()
  })

  it('hides non-routing modifiers from the normal suggestion form', () => {
    renderDialog()

    expect(screen.getByText('E-Bike')).toBeInTheDocument()
    expect(screen.queryByText('Indoor')).not.toBeInTheDocument()
    expect(screen.queryByText('Virtuell')).not.toBeInTheDocument()
    expect(screen.queryByText('Pendeln')).not.toBeInTheDocument()
    expect(screen.queryByText('Rundtour')).not.toBeInTheDocument()
  })

  it('allows target distances up to 100 km', () => {
    renderDialog()

    expect(screen.getByLabelText('Zieldistanz')).toHaveAttribute('max', '100')
    expect(screen.getByLabelText('Zieldistanz Slider')).toHaveAttribute('max', '100')
    expect(screen.getByText('100 km')).toBeInTheDocument()
  })

  it('shows distance controls in miles for imperial users while preserving the same meter bounds', () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'

    renderDialog()

    expect(screen.getByLabelText('Zieldistanz')).toHaveAttribute('min', '0.621')
    expect(screen.getByLabelText('Zieldistanz')).toHaveAttribute('max', '62.137')
    expect(screen.getByLabelText('Zieldistanz Slider')).toHaveAttribute('max', '62.137')
    expect(screen.getByText('62.137 mi')).toBeInTheDocument()
  })

  it('submits unchanged imperial distance without altering the stored meter target', async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'

    renderDialog()

    await clickSubmit()

    await waitFor(() => expect(mockSuggestRoute).toHaveBeenCalled())
    expect(mockSuggestRoute).toHaveBeenCalledWith(expect.objectContaining({ target_distance_m: 20_000 }))
  })

  it('clamps imperial min and max interactions to the meter contract', async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'

    const { rerender } = renderDialog()
    const slider = screen.getByLabelText('Zieldistanz Slider')
    fireEvent.change(slider, { target: { value: '0.621' } })
    await clickSubmit()

    await waitFor(() => expect(mockSuggestRoute).toHaveBeenCalled())
    expect(mockSuggestRoute).toHaveBeenLastCalledWith(expect.objectContaining({ target_distance_m: 1000 }))

    mockSuggestRoute.mockClear()
    rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
        <ToastProvider>
          <RouteSuggestionDialog
            start={[10, 48]}
            onClose={vi.fn()}
            onResult={vi.fn()}
          />
        </ToastProvider>
      </QueryClientProvider>,
    )
    fireEvent.change(screen.getByLabelText('Zieldistanz Slider'), { target: { value: '62.137' } })
    await clickSubmit()

    await waitFor(() => expect(mockSuggestRoute).toHaveBeenCalled())
    expect(mockSuggestRoute).toHaveBeenLastCalledWith(expect.objectContaining({ target_distance_m: 100_000 }))
  })
})

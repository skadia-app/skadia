import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import RouteSuggestionDialog from '../RouteSuggestionDialog'
import { ToastProvider } from '@/lib/toast'

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
  suggestRoute: vi.fn(),
}))

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

describe('RouteSuggestionDialog modifier controls', () => {
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
})

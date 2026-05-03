import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import AutomationRulesPage from '../AutomationRulesPage'
import type { AutomationRule } from '@/api/types'
import { ToastProvider } from '@/lib/toast'

const rules: AutomationRule[] = [
  rule({ id: 'electric-sync', name: 'E-Bike', is_system: true, system_type: 'electric_sync', priority: -100 }),
  rule({ id: 'indoor-sync', name: 'Indoor', is_system: true, system_type: 'indoor_sync', priority: -100 }),
  rule({ id: 'virtual-sync', name: 'Virtuell', is_system: true, system_type: 'virtual_sync', priority: -100 }),
  rule({
    id: 'roundtrip',
    name: 'Rundtour',
    is_system: true,
    system_type: 'roundtrip',
    priority: -99,
    conditions: [{ field: 'start_end_distance_m', op: 'lte', value: 500 }],
    actions: [{ action: 'add_tag', params: { tag_id: 'tag-roundtrip' } }],
  }),
  rule({
    id: 'tuple-rule',
    name: 'MTB virtuell',
    is_system: false,
    system_type: null,
    condition_groups: [{ logic: 'and', conditions: [
      { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'cycling', sub_type: 'mountain', is_electric: true }] },
    ] }],
    actions: [{ action: 'set_virtual', params: { virtual: true } }],
  }),
]

vi.mock('@/api/rules', () => ({
  fetchRules: vi.fn(async () => rules),
  createRule: vi.fn(),
  updateRule: vi.fn(),
  deleteRule: vi.fn(),
  applyRules: vi.fn(),
  dryRunAllRules: vi.fn(),
  dryRunSingleRule: vi.fn(),
  applySingleRule: vi.fn(),
}))

vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn(async () => [{ id: 'tag-roundtrip', name: 'Rundtour' }]),
}))

function rule(overrides: Partial<AutomationRule>): AutomationRule {
  return {
    id: 'rule',
    user_id: 'user',
    name: 'Regel',
    is_active: true,
    priority: 0,
    trigger_event: 'on_import',
    stop_on_match: false,
    apply_retroactively: false,
    conditions: [],
    condition_groups: null,
    actions: [],
    branches: null,
    times_applied: 0,
    is_system: false,
    system_type: null,
    created_at: '2026-05-03T00:00:00Z',
    ...overrides,
  }
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <MemoryRouter>
          <AutomationRulesPage />
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  )
}

describe('AutomationRulesPage rule summaries', () => {
  it('hides only readonly sync rules until the sync checkbox is enabled', async () => {
    renderPage()

    await waitFor(() => expect(screen.getByText('Rundtour')).toBeInTheDocument())
    expect(screen.getByText('System-Regeln')).toBeInTheDocument()
    expect(screen.queryByText('E-Bike')).not.toBeInTheDocument()

    fireEvent.click(screen.getByLabelText('Sync-Regeln anzeigen'))

    expect(await screen.findByText('E-Bike')).toBeInTheDocument()
    expect(screen.getByText('Indoor')).toBeInTheDocument()
    expect(screen.getByText('Virtuell')).toBeInTheDocument()
    expect(screen.queryByText(/Datenbankspalte/)).not.toBeInTheDocument()
    expect(screen.getByText(/Sportart und Sub-Sportart/)).toBeInTheDocument()
  })

  it('formats sport tuple conditions without object string leakage', async () => {
    renderPage()

    await waitFor(() => expect(screen.getByText('MTB virtuell')).toBeInTheDocument())
    expect(screen.getByText(/Sportauswahl in Radfahren \(MTB\), E-Bike=Ja/)).toBeInTheDocument()
    expect(screen.getByText(/Virtuell\(Ja\)/)).toBeInTheDocument()
    expect(screen.queryByText(/\[object Object\]/)).not.toBeInTheDocument()
  })
})

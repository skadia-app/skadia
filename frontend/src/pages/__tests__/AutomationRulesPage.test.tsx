import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AutomationRulesPage from '../AutomationRulesPage'
import type { AutomationRule } from '@/api/types'
import { ToastProvider } from '@/lib/toast'
import { i18n } from '@/i18n/i18n'

const userPrefs = vi.hoisted(() => ({ locale: 'de', units: 'metric' as 'metric' | 'imperial' }))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

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
const baseRuleCount = rules.length

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
  beforeEach(async () => {
    rules.splice(baseRuleCount)
    userPrefs.locale = 'de'
    userPrefs.units = 'metric'
    await i18n.changeLanguage('de')
  })

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
    expect(screen.getByText(/Sportauswahl in Radfahren \(MTB\), E-Antrieb=Ja/)).toBeInTheDocument()
    expect(screen.getByText(/Virtuell\(Ja\)/)).toBeInTheDocument()
    expect(screen.queryByText(/\[object Object\]/)).not.toBeInTheDocument()
  })

  it('localizes the page shell and primary actions', async () => {
    await i18n.changeLanguage('en')

    renderPage()

    expect(await screen.findByRole('heading', { name: 'Automation rules' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Preview' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Apply rules' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create rule' })).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Search rules...')).toBeInTheDocument()
  })

  it('localizes rule condition and action summaries', async () => {
    await i18n.changeLanguage('en')

    renderPage()

    await waitFor(() => expect(screen.getByText('MTB virtuell')).toBeInTheDocument())
    expect(screen.getByText(/IF Sport selection in Cycling \(Mountainbike\), E-drive=Yes/)).toBeInTheDocument()
    expect(screen.getByText(/THEN Virtual\(Yes\)/)).toBeInTheDocument()
    expect(screen.queryByText(/WENN/)).not.toBeInTheDocument()
    expect(screen.queryByText(/DANN/)).not.toBeInTheDocument()
  })

  it('uses localized unknown labels for unsupported summary fields and actions', async () => {
    rules.push(rule({
      id: 'unknown-summary-labels',
      name: 'Unknown summary labels',
      conditions: [{ field: 'backend_private_token', op: 'eq', value: '42' }],
      actions: [
        { action: 'backend_private_action', params: {} },
        { action: 'set_field', params: { field: 'backend_private_field', value: 'x' } },
      ],
    }))

    renderPage()

    await waitFor(() => expect(screen.getByText('Unknown summary labels')).toBeInTheDocument())
    expect(screen.getByText(/Unbekanntes Feld = 42/)).toBeInTheDocument()
    expect(screen.getByText(/Unbekannte Aktion\(\)/)).toBeInTheDocument()
    expect(screen.getByText(/Feld setzen\(Unbekanntes Feld = x\)/)).toBeInTheDocument()
    expect(screen.queryByText(/backend_private_token|backend_private_action|backend_private_field/)).not.toBeInTheDocument()
  })

  it('formats numeric rule summaries with the user unit preference', async () => {
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'
    await i18n.changeLanguage('en')
    rules.push(rule({
      id: 'imperial-summary',
      name: 'Imperial summary',
      conditions: [
        { field: 'distance_m', op: 'gt', value: 1609.344 },
        { field: 'avg_speed_ms', op: 'gt', value: 10 },
        { field: 'avg_pace_min_km', op: 'lt', value: 5 },
        { field: 'avg_temperature_c', op: 'gt', value: 20 },
      ],
    }))

    renderPage()

    await waitFor(() => expect(screen.getByText('Imperial summary')).toBeInTheDocument())
    expect(screen.getByText(/Distance > 1.0 mi/)).toBeInTheDocument()
    expect(screen.getByText(/Avg speed > 22.4 mph/)).toBeInTheDocument()
    expect(screen.getByText(/Avg pace < 8:03 min\/mi/)).toBeInTheDocument()
    expect(screen.getByText(/Avg temperature > 68 °F/)).toBeInTheDocument()
    expect(screen.queryByText(/km\/h|min\/km|°C/)).not.toBeInTheDocument()
  })
})

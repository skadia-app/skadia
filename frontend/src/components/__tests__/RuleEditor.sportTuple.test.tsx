import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import RuleEditor from '../RuleEditor'
import type { AutomationRule } from '@/api/types'

vi.mock('@/api/gear', () => ({ fetchGear: vi.fn(async () => []) }))
vi.mock('@/api/hardware', () => ({ fetchDevices: vi.fn(async () => []) }))
vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn(async () => [
    { id: 'tag-ebike', name: 'E-Bike', color: '#a855f7' },
    { id: 'tag-other', name: 'Urlaub', color: '#22c55e' },
  ]),
}))
vi.mock('maplibre-gl', () => ({ default: {} }))

function renderEditor(seed: Partial<AutomationRule>, onSave = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RuleEditor
        initialData={{
          name: 'Sportregel',
          priority: 0,
          trigger_event: 'on_import',
          stop_on_match: false,
          condition_groups: [{ logic: 'and', conditions: [{ field: 'sport_type', op: 'eq', value: 'cycling' }] }],
          actions: [{ action: 'set_sport_type', params: { sport_type: 'cycling' } }],
          ...seed,
        }}
        onSave={onSave}
        onCancel={() => {}}
      />
    </QueryClientProvider>,
  )
  return onSave
}

function renderNewEditor(seed: Partial<AutomationRule> = {}, onSave = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RuleEditor
        initialData={{
          name: 'Neue Regel',
          priority: 0,
          trigger_event: 'on_import',
          stop_on_match: false,
          ...seed,
        }}
        onSave={onSave}
        onCancel={() => {}}
      />
    </QueryClientProvider>,
  )
  return onSave
}

describe('RuleEditor sport tuple controls', () => {
  it('starts new rules with the generic condition picker instead of an empty sport tuple', () => {
    renderNewEditor()

    expect(screen.getByDisplayValue('Titel')).toBeInTheDocument()
    expect(screen.queryByLabelText('Sportart')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('E-Bike')).not.toBeInTheDocument()
  })

  it('keeps newly added conditions visible when a sport tuple already exists', () => {
    renderEditor({})

    fireEvent.click(screen.getByRole('button', { name: '+ Bedingung hinzufügen' }))

    expect(screen.getByLabelText('Sportart')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Titel')).toBeInTheDocument()
  })

  it('shows running subtypes and hides E-Bike modifiers for running sport tuples', () => {
    renderEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'sport_type', op: 'eq', value: 'running' }] }],
      actions: [{ action: 'set_sport_type', params: { sport_type: 'running' } }],
    })

    expect(screen.getByLabelText('Sportart')).toHaveValue('running')
    expect(screen.getAllByRole('option', { name: 'Trail' }).length).toBeGreaterThan(0)
    expect(screen.queryByLabelText('E-Bike')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('E-Bike setzen')).not.toBeInTheDocument()
  })

  it('edits activity speed conditions in km/h while saving m/s', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'avg_speed_ms', op: 'gt', value: 10 }] }],
    })

    expect(screen.getByDisplayValue('36')).toBeInTheDocument()
    expect(screen.getByText('km/h')).toBeInTheDocument()

    fireEvent.change(screen.getByDisplayValue('36'), { target: { value: '54' } })
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'avg_speed_ms', op: 'gt', value: 15 },
    ])
  })

  it('edits activity speed ranges in km/h while saving m/s ranges', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'max_speed_ms', op: 'between', value: '5,10' }] }],
    })

    expect(screen.getByDisplayValue('18')).toBeInTheDocument()
    expect(screen.getByDisplayValue('36')).toBeInTheDocument()

    fireEvent.change(screen.getByDisplayValue('18'), { target: { value: '21.6' } })
    fireEvent.change(screen.getByDisplayValue('36'), { target: { value: '43.2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'max_speed_ms', op: 'between', value: '6,12' },
    ])
  })

  it('serializes sport condition and action as canonical tuple plus electric modifier', async () => {
    const onSave = renderEditor({})

    fireEvent.change(screen.getByLabelText('Sub-Sportart'), { target: { value: 'mountain' } })
    fireEvent.change(screen.getByLabelText('E-Bike'), { target: { value: 'true' } })
    fireEvent.change(screen.getByLabelText('Sub-Sportart setzen'), { target: { value: 'mountain' } })
    fireEvent.change(screen.getByLabelText('E-Bike setzen'), { target: { value: 'true' } })
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(onSave).toHaveBeenCalled())
    const saved = onSave.mock.calls[0][0]
    expect(saved.condition_groups[0].conditions).toEqual([
      { field: 'sport_type', op: 'eq', value: 'cycling' },
      { field: 'sub_type', op: 'eq', value: 'mountain' },
      { field: 'is_electric', op: 'eq', value: true },
    ])
    expect(saved.actions).toEqual([
      { action: 'set_sport_type', params: { sport_type: 'cycling', sub_type: 'mountain' } },
      { action: 'set_electric', params: { electric: true } },
    ])
  })

  it('loads legacy e-bike sport actions and re-saves without deprecated sport values', async () => {
    const onSave = renderEditor({
      actions: [{ action: 'set_sport_type', params: { sport_type: 'e_mountain_biking' } }],
    })

    expect(screen.getByLabelText('Sportart setzen')).toHaveValue('cycling')
    expect(screen.getByLabelText('Sub-Sportart setzen')).toHaveValue('mountain')
    expect(screen.getByLabelText('E-Bike setzen')).toHaveValue('true')

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(JSON.stringify(onSave.mock.calls[0][0].actions)).not.toContain('e_mountain_biking')
  })

  it('carries indoor modifiers through compound sport conditions and actions', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'sport_type', op: 'eq', value: 'treadmill_running' }] }],
      actions: [{ action: 'set_sport_type', params: { sport_type: 'indoor_cycling' } }],
    })

    expect(screen.getByLabelText('Sportart')).toHaveValue('running')
    expect(screen.getByLabelText('Sub-Sportart')).toHaveValue('treadmill')
    expect(screen.getByLabelText('Indoor')).toHaveValue('true')
    expect(screen.getByLabelText('Sportart setzen')).toHaveValue('cycling')
    expect(screen.getByLabelText('Sub-Sportart setzen')).toHaveValue('indoor_cycling')
    expect(screen.getByLabelText('Indoor setzen')).toHaveValue('true')

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_type', op: 'eq', value: 'running' },
      { field: 'sub_type', op: 'eq', value: 'treadmill' },
      { field: 'is_indoor', op: 'eq', value: true },
    ])
    expect(onSave.mock.calls[0][0].actions).toEqual([
      { action: 'set_sport_type', params: { sport_type: 'cycling', sub_type: 'indoor_cycling' } },
      { action: 'set_indoor', params: { indoor: true } },
    ])
  })

  it('carries virtual and commute modifiers through compound sport conditions and actions', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'and', conditions: [
        { field: 'sport_type', op: 'eq', value: 'cycling' },
        { field: 'is_virtual', op: 'eq', value: true },
        { field: 'is_commute', op: 'eq', value: false },
      ] }],
      actions: [{ action: 'set_sport_type', params: { sport_type: 'cycling', virtual: true, commute: false } }],
    })

    expect(screen.getByLabelText('Virtuell')).toHaveValue('true')
    expect(screen.getByLabelText('Pendeln')).toHaveValue('false')
    expect(screen.getByLabelText('Virtuell setzen')).toHaveValue('true')
    expect(screen.getByLabelText('Pendeln setzen')).toHaveValue('false')

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_type', op: 'eq', value: 'cycling' },
      { field: 'is_virtual', op: 'eq', value: true },
      { field: 'is_commute', op: 'eq', value: false },
    ])
    expect(onSave.mock.calls[0][0].actions).toEqual([
      { action: 'set_sport_type', params: { sport_type: 'cycling', sub_type: '' } },
      { action: 'set_virtual', params: { virtual: true } },
      { action: 'set_commute', params: { commute: false } },
    ])
  })

  it('keeps standalone indoor conditions when no sport tuple is present', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'is_indoor', op: 'eq', value: true }] }],
      actions: [{ action: 'set_indoor', params: { indoor: true } }],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'is_indoor', op: 'eq', value: true },
    ])
  })

  it('does not collapse canonical non-eq and in sport conditions into a sport tuple control', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'and', conditions: [
        { field: 'sport_type', op: 'neq', value: 'cycling' },
        { field: 'sport_type', op: 'in', value: ['running', 'swimming'] },
      ] }],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_type', op: 'neq', value: 'cycling' },
      { field: 'sport_type', op: 'in', value: ['running', 'swimming'] },
    ])
  })

  it('dedupes set_electric true with explicit E-Bike tag and blocks contradictions', async () => {
    const onSave = renderEditor({
      actions: [
        { action: 'set_sport_type', params: { sport_type: 'cycling', electric: true } },
        { action: 'add_tag', params: { tag_id: 'tag-ebike' } },
      ],
    })

    await waitFor(() => expect(screen.getByDisplayValue('E-Bike')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(JSON.stringify(onSave.mock.calls[0][0].actions)).not.toContain('tag-ebike')

    const blockedSave = vi.fn()
    renderEditor({
      actions: [
        { action: 'set_sport_type', params: { sport_type: 'cycling', electric: false } },
        { action: 'add_tag', params: { tag_id: 'tag-ebike' } },
      ],
    }, blockedSave)
    fireEvent.click(screen.getAllByRole('button', { name: 'Speichern' }).at(-1)!)
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('set_electric(false)'))
    expect(blockedSave).not.toHaveBeenCalled()
  })
})

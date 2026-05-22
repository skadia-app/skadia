import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import RuleEditor from '../RuleEditor'
import type { AutomationRule } from '@/api/types'

vi.mock('@/api/gear', () => ({ fetchGear: vi.fn(async () => [
  { id: 'gear-active', name: 'Gravelbike', is_retired: false },
  { id: 'gear-retired', name: 'Altes Rad', is_retired: true },
]) }))
vi.mock('@/api/hardware', () => ({ fetchDevices: vi.fn(async () => []) }))
vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn(async () => [
    { id: 'tag-ebike', name: 'E-Bike', color: '#a855f7' },
    { id: 'tag-other', name: 'Urlaub', color: '#22c55e' },
  ]),
}))
vi.mock('@/api/segments', () => ({
  fetchSegments: vi.fn(async () => ({
    segments: [
      {
        id: 'segment-visible',
        name: 'Sichtbares Segment',
        sport_group: 'bike',
        segment_type: 'trail',
        segment_type_meta: {},
        distance_m: 1200,
        elevation_gain_m: 85,
        effort_count: 4,
        athlete_count: 2,
        ridden_count: 1,
        is_favorite: false,
        visibility: 'shared',
        created_by: 'user-1',
        created_at: '2026-05-22T00:00:00Z',
        bidirectional: true,
      },
    ],
    has_more: false,
  })),
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
    expect(screen.queryByLabelText('E-Antrieb')).not.toBeInTheDocument()
  })

  it('keeps newly added conditions visible when a sport tuple already exists', () => {
    renderEditor({})

    fireEvent.click(screen.getByRole('button', { name: '+ Bedingung hinzufügen' }))

    expect(screen.getByLabelText('Sportart')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Titel')).toBeInTheDocument()
  })

  it('uses direct condition categories for gear, tags, media, activity and visibility', () => {
    renderNewEditor()

    expect(screen.queryByRole('option', { name: 'Kontext' })).not.toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Ausrüstung' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Tags' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Medien' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Aktivität' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Sichtbarkeit' })).toBeInTheDocument()
  })

  it('offers taxonomy trait and global modifier condition categories', () => {
    renderNewEditor()

    expect(screen.getByRole('option', { name: 'Taxonomie' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Modifier' })).toBeInTheDocument()
  })

  it('serializes taxonomy trait conditions without expanding them to sport tuples', async () => {
    const onSave = renderNewEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'sport_environment', op: 'in', value: ['water'] }] }],
      actions: [{ action: 'set_sport_type', params: { sport_type: 'cycling' } }],
    })

    expect(screen.getByDisplayValue('Taxonomie')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Umgebung')).toBeInTheDocument()
    expect(screen.getByText('Wasser')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_environment', op: 'in', value: ['water'] },
    ])
  })

  it('serializes global modifier conditions separately from sport tuples', async () => {
    const onSave = renderNewEditor({
      condition_groups: [{ logic: 'and', conditions: [
        { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'cycling' }] },
        { field: 'is_electric', op: 'eq', value: true },
      ] }],
      actions: [{ action: 'set_sport_type', params: { sport_type: 'cycling' } }],
    })

    expect(screen.getByDisplayValue('Modifier')).toBeInTheDocument()
    expect(screen.getByDisplayValue('E-Antrieb')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'cycling' }] },
      { field: 'is_electric', op: 'eq', value: true },
    ])
  })

  it('shows active gear options in gear conditions without offering retired gear', async () => {
    renderEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'gear_id', op: 'eq', value: '' }] }],
    })

    await waitFor(() => expect(screen.getByRole('option', { name: 'Gravelbike' })).toBeInTheDocument())
    expect(screen.queryByRole('option', { name: 'Altes Rad' })).not.toBeInTheDocument()
  })

  it('keeps stored missing gear ids visible without adding them as normal choices', async () => {
    renderEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'gear_id', op: 'eq', value: 'missing-gear' }] }],
    })

    await waitFor(() => expect(screen.getByRole('option', { name: 'Nicht gelistete Ausrüstung (missing-gear)' })).toBeInTheDocument())
    expect(screen.getByDisplayValue('Nicht gelistete Ausrüstung (missing-gear)')).toBeInTheDocument()
  })

  it('stores multiple selected sport tuples as one tuple-list condition', async () => {
    const onSave = renderEditor({})

    fireEvent.click(screen.getByRole('button', { name: 'Sportart hinzufügen' }))
    fireEvent.change(screen.getAllByLabelText('Sportart')[1], { target: { value: 'running' } })
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'cycling' }, { sport_type: 'running' }] },
    ])
  })

  it('removes stale tuple modifiers when a sport no longer supports them', async () => {
    const onSave = renderEditor({})

    fireEvent.change(screen.getByLabelText('E-Antrieb'), { target: { value: 'true' } })
    fireEvent.change(screen.getByLabelText('Sportart'), { target: { value: 'running' } })
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'running' }] },
    ])
  })

  it('does not collapse OR sport alternatives into a tuple list', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'or', conditions: [
        { field: 'sport_type', op: 'eq', value: 'running' },
        { field: 'sport_type', op: 'eq', value: 'cycling' },
      ] }],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_type', op: 'eq', value: 'running' },
      { field: 'sport_type', op: 'eq', value: 'cycling' },
    ])
  })

  it('shows running subtypes and hides electric modifiers for running sport tuples', () => {
    renderEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'sport_type', op: 'eq', value: 'running' }] }],
      actions: [{ action: 'set_sport_type', params: { sport_type: 'running' } }],
    })

    expect(screen.getByLabelText('Sportart')).toHaveValue('running')
    expect(screen.getAllByRole('option', { name: 'Trail' }).length).toBeGreaterThan(0)
    expect(screen.queryByLabelText('E-Antrieb')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('E-Antrieb setzen')).not.toBeInTheDocument()
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

  it('uses an access-aware segment picker with preview text for segment conditions', async () => {
    renderEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'has_segment_effort', op: 'eq', value: '' }] }],
    })

    await waitFor(() => expect(screen.getByRole('option', { name: 'Sichtbares Segment' })).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Segment wählen'), { target: { value: 'segment-visible' } })

    expect(screen.getByText('Trail · Radfahren · 1,2 km · 85 hm · Geteilt')).toBeInTheDocument()
  })

  it('serializes sport condition and action as canonical tuple plus electric modifier', async () => {
    const onSave = renderEditor({})

    fireEvent.change(screen.getByLabelText('Sub-Sportart'), { target: { value: 'mountain' } })
    fireEvent.change(screen.getByLabelText('E-Antrieb'), { target: { value: 'true' } })
    fireEvent.change(screen.getByLabelText('Sub-Sportart setzen'), { target: { value: 'mountain' } })
    fireEvent.change(screen.getByLabelText('E-Antrieb setzen'), { target: { value: 'true' } })
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(onSave).toHaveBeenCalled())
    const saved = onSave.mock.calls[0][0]
    expect(saved.condition_groups[0].conditions).toEqual([
      { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'cycling', sub_type: 'mountain', is_electric: true }] },
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
    expect(screen.getByLabelText('E-Antrieb setzen')).toHaveValue('true')

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
      { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'running', sub_type: 'treadmill', is_indoor: true }] },
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
      { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'cycling', is_virtual: true, is_commute: false }] },
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

  it('preserves render-existing tuple modifiers on unrelated saves', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'and', conditions: [
        { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'running', is_electric: true }] },
      ] }],
      actions: [{ action: 'set_sport_type', params: { sport_type: 'running' } }],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'running', is_electric: true }] },
    ])
  })

  it('preserves global modifier conditions next to canonical sport tuples', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'and', conditions: [
        { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'cycling' }] },
        { field: 'is_indoor', op: 'eq', value: true },
      ] }],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'cycling' }] },
      { field: 'is_indoor', op: 'eq', value: true },
    ])
  })

  it('keeps standalone electric actions after non-electric sport actions', async () => {
    const onSave = renderEditor({
      actions: [
        { action: 'set_sport_type', params: { sport_type: 'running' } },
        { action: 'set_electric', params: { electric: true } },
      ],
    })

    expect(screen.getByLabelText('Sportart setzen')).toHaveValue('running')
    expect(screen.queryByLabelText('E-Antrieb setzen')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].actions).toEqual([
      { action: 'set_sport_type', params: { sport_type: 'running', sub_type: '' } },
      { action: 'set_electric', params: { electric: true } },
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

  it('keeps explicit E-Bike tag actions next to generic electric actions without cycling action context', async () => {
    const onSave = renderEditor({
      actions: [
        { action: 'set_electric', params: { electric: true } },
        { action: 'add_tag', params: { tag_id: 'tag-ebike' } },
      ],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].actions).toEqual([
      { action: 'set_electric', params: { electric: true } },
      { action: 'add_tag', params: { tag_id: 'tag-ebike' } },
    ])

    const falseSave = vi.fn()
    renderEditor({
      actions: [
        { action: 'set_electric', params: { electric: false } },
        { action: 'add_tag', params: { tag_id: 'tag-ebike' } },
      ],
    }, falseSave)
    fireEvent.click(screen.getAllByRole('button', { name: 'Speichern' }).at(-1)!)
    await waitFor(() => expect(falseSave).toHaveBeenCalled())
    expect(falseSave.mock.calls[0][0].actions).toEqual([
      { action: 'set_electric', params: { electric: false } },
      { action: 'add_tag', params: { tag_id: 'tag-ebike' } },
    ])
  })

  it('labels parent-only sport actions as clearing the sub-sport, not wildcard matching', () => {
    renderEditor({
      actions: [{ action: 'set_sport_type', params: { sport_type: 'cycling', sub_type: '' } }],
    })

    const subtypeSelect = screen.getByLabelText('Sub-Sportart setzen')
    expect(within(subtypeSelect).queryByRole('option', { name: 'beliebig' })).not.toBeInTheDocument()
    expect(within(subtypeSelect).getByRole('option', { name: 'keine Sub-Sportart' })).toBeInTheDocument()
  })

  it('blocks duplicate sport tuple condition rows before submit', async () => {
    const onSave = renderEditor({
      condition_groups: [{ logic: 'and', conditions: [
        { field: 'sport_tuple', op: 'in', value: [{ sport_type: 'cycling' }, { sport_type: 'cycling' }] },
      ] }],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Sportauswahl enthält Duplikate'))
    expect(onSave).not.toHaveBeenCalled()
  })

  it('keeps the last selected trait value instead of submitting an empty trait condition', async () => {
    const onSave = renderNewEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'sport_environment', op: 'in', value: ['water'] }] }],
      actions: [{ action: 'set_sport_type', params: { sport_type: 'cycling' } }],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Wasser' }))
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0][0].condition_groups[0].conditions).toEqual([
      { field: 'sport_environment', op: 'in', value: ['water'] },
    ])
  })

  it('blocks already-empty trait conditions before submit', async () => {
    const onSave = renderNewEditor({
      condition_groups: [{ logic: 'and', conditions: [{ field: 'sport_environment', op: 'in', value: [] }] }],
      actions: [{ action: 'set_sport_type', params: { sport_type: 'cycling' } }],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Taxonomie-Bedingung braucht mindestens einen Wert'))
    expect(onSave).not.toHaveBeenCalled()
  })

  it('blocks actions with missing required params before submit', async () => {
    const onSave = renderEditor({
      actions: [{ action: 'add_tag', params: {} }],
    })

    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Aktion "Tag hinzufügen" ist unvollständig'))
    expect(onSave).not.toHaveBeenCalled()
  })
})

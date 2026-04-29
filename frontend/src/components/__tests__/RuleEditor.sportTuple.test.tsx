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

describe('RuleEditor sport tuple controls', () => {
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

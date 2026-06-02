import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import '@testing-library/jest-dom'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ActivityQuickEditModal from '../ActivityQuickEditModal'
import { fetchActivity, updateActivity } from '@/api/activities'
import { assignActivitiesToTag, fetchTags, unassignActivitiesFromTag, type Tag } from '@/api/tags'
import type { Activity } from '@/api/types'
import { i18n } from '@/i18n/i18n'

const userPrefs = vi.hoisted(() => ({ locale: 'de', units: 'metric' as 'metric' | 'imperial' }))

vi.mock('@/api/activities', () => ({
  fetchActivity: vi.fn(() => new Promise(() => {})),
  fetchActivityStreams: vi.fn(async () => []),
  updateActivity: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn(async () => []),
  createTag: vi.fn(),
  assignActivitiesToTag: vi.fn(),
  unassignActivitiesFromTag: vi.fn(),
}))

vi.mock('../ActivityQuickEditMiniMap', () => ({
  default: () => <div data-testid="quick-edit-minimap" />,
  trackCoordsFromStreams: () => [],
}))

vi.mock('../SportTuplePicker', () => ({
  default: () => <button type="button">Sport Picker</button>,
}))

function activity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    user_id: 'user-1',
    title: 'Partial row',
    description: null,
    sport_type: 'running',
    sub_type: null,
    is_electric: false,
    is_virtual: false,
    is_commute: false,
    exclude_from_heatmap: false,
    exclude_from_stats: false,
    started_at: '2026-05-01T10:00:00Z',
    elapsed_time_s: 3600,
    distance_m: 10000,
    elevation_gain_m: 100,
    elevation_loss_m: 100,
    is_indoor: false,
    tags: [],
    visibility: 'private',
    enrichment_status: 'complete',
    needs_classification: false,
    has_gps: true,
    created_at: '2026-05-01T10:00:00Z',
    updated_at: '2026-05-01T10:00:00Z',
    ...overrides,
  }
}

function renderModal(activityValue: Activity) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ActivityQuickEditModal
          activity={activityValue}
          open
          onClose={vi.fn()}
          onSaved={vi.fn()}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function tag(overrides: Partial<Tag> = {}): Tag {
  return {
    id: 'tag-1',
    name: 'Custom',
    color: '#F97316',
    text_color: '#ffffff',
    category: 'user',
    description: '',
    is_system: false,
    is_auto: false,
    conditions: null,
    conditions_logic: 'and',
    activity_count: 1,
    created_at: '2026-05-01T10:00:00Z',
    ...overrides,
  }
}

describe('ActivityQuickEditModal', () => {
  beforeEach(async () => {
    userPrefs.locale = 'de'
    userPrefs.units = 'metric'
    await i18n.changeLanguage('de')
    vi.mocked(fetchActivity).mockImplementation(() => new Promise(() => {}))
    vi.mocked(updateActivity).mockReset()
    vi.mocked(fetchTags).mockResolvedValue([])
    vi.mocked(assignActivitiesToTag).mockReset()
    vi.mocked(unassignActivitiesFromTag).mockReset()
  })

  it('does not render editable fields until the full activity is loaded', () => {
    renderModal(activity())

    expect(screen.getByText('Aktivität wird geladen...')).toBeInTheDocument()
    expect(screen.queryByText('Titel')).not.toBeInTheDocument()
    expect(screen.queryByText('Tags')).not.toBeInTheDocument()
  })

  it('localizes visible form labels and actions in English', async () => {
    await i18n.changeLanguage('en')
    userPrefs.locale = 'en'
    const user = userEvent.setup()
    const initial = activity({
      title: 'English row',
      tags: [],
      recording_device_name: 'Garmin Edge',
      recording_device_id: 'device-1',
    } as Partial<Activity>)
    vi.mocked(fetchActivity).mockResolvedValue(initial)

    renderModal(initial)

    expect(await screen.findByRole('heading', { name: 'Edit activity' })).toBeInTheDocument()
    expect(screen.getByText('Changes are applied when you save.')).toBeInTheDocument()
    expect(screen.getByText('Recording device')).toBeInTheDocument()
    expect(screen.getByLabelText('Title')).toHaveValue('English row')
    expect(screen.getByText('Sport type')).toBeInTheDocument()
    expect(screen.getByText('Privacy')).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Private' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Details' }))
    expect(screen.getByLabelText('Hide map')).toBeInTheDocument()
    expect(screen.getByText('No tags')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Search or create tag...')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'More editing' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
    expect(screen.queryByText('Aktivität bearbeiten')).not.toBeInTheDocument()
  })

  it('formats quick stats with user unit preferences', async () => {
    await i18n.changeLanguage('en')
    userPrefs.locale = 'en'
    userPrefs.units = 'imperial'
    const initial = activity({
      title: 'Imperial row',
      distance_m: 1609.344,
      elevation_gain_m: 100,
      elevation_loss_m: 50,
    })
    vi.mocked(fetchActivity).mockResolvedValue(initial)

    renderModal(initial)

    expect(await screen.findByRole('heading', { name: 'Edit activity' })).toBeInTheDocument()
    expect(screen.getByText('1.0 mi')).toBeInTheDocument()
    expect(screen.getByText('+328 ft / −164 ft')).toBeInTheDocument()
  })

  it('uses catalog keys without hardcoded quick-edit visibility or feel fallbacks', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/components/activity/ActivityQuickEditModal.tsx'), 'utf8')

    expect(source).not.toMatch(/label:\s*['"](?:Private|Known people|Instance|Federation|Public)['"]/)
    expect(source).not.toMatch(/defaultValue:\s*VISIBILITY_OPTIONS/)
    expect(source).not.toMatch(/defaultValue:\s*fallback/)
  })

  it('preserves typed draft fields when the full activity refresh resolves mid-edit', async () => {
    const user = userEvent.setup()
    let resolveActivity: (value: Activity) => void = () => {}
    vi.mocked(fetchActivity).mockImplementation(() => new Promise<Activity>((resolve) => {
      resolveActivity = resolve
    }))

    renderModal(activity({ title: 'List row' }))
    resolveActivity(activity({ title: 'Full row' }))

    const titleInput = await screen.findByLabelText('Titel')
    await user.clear(titleInput)
    await user.type(titleInput, 'User typed')

    resolveActivity(activity({ title: 'Later full row' }))

    expect(await screen.findByDisplayValue('User typed')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('Later full row')).not.toBeInTheDocument()
  })

  it('resets the draft to the refreshed activity after a partial save error', async () => {
    const user = userEvent.setup()
    const initial = activity({ title: 'Initial', tags: ['Custom'] })
    const refreshed = activity({ title: 'Server after partial save', tags: [] })
    vi.mocked(fetchActivity)
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(refreshed)
    vi.mocked(fetchTags).mockResolvedValue([tag({ id: 'tag-1', name: 'Custom' })])
    vi.mocked(unassignActivitiesFromTag).mockResolvedValue(undefined)
    vi.mocked(updateActivity).mockRejectedValue(new Error('refresh failed'))

    renderModal(initial)

    const titleInput = await screen.findByLabelText('Titel')
    await user.clear(titleInput)
    await user.type(titleInput, 'Unsaved draft')
    await user.click(screen.getByRole('button', { name: 'Custom entfernen' }))
    await user.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(await screen.findByText('Teilweise gespeichert. Bitte prüfe die Aktivität und speichere fehlende Änderungen erneut.')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Server after partial save')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('Unsaved draft')).not.toBeInTheDocument()
  })

  it('defers tag endpoint path refresh and relies on one final activity refresh', async () => {
    const user = userEvent.setup()
    const initial = activity({ title: 'Initial', tags: ['Custom'] })
    vi.mocked(fetchActivity).mockResolvedValue(initial)
    vi.mocked(fetchTags).mockResolvedValue([tag({ id: 'tag-1', name: 'Custom' })])
    vi.mocked(unassignActivitiesFromTag).mockResolvedValue(undefined)
    vi.mocked(updateActivity).mockResolvedValue(activity({ title: 'Initial', tags: [] }))

    renderModal(initial)

    await screen.findByLabelText('Titel')
    await user.click(screen.getByRole('button', { name: 'Custom entfernen' }))
    await user.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(unassignActivitiesFromTag).toHaveBeenCalledWith('tag-1', ['act-1'], { deferActivityFileRefresh: true })
    expect(updateActivity).toHaveBeenCalledWith('act-1', {})
  })

  it('keeps detailed privacy controls collapsed until Details is opened and saves them through the modal', async () => {
    const user = userEvent.setup()
    const initial = activity({
      hide_map: false,
      data_visibility: {
        heart_rate: true,
        power: true,
      },
    })
    vi.mocked(fetchActivity).mockResolvedValue(initial)
    vi.mocked(updateActivity).mockResolvedValue({ ...initial, hide_map: true })

    renderModal(initial)

    expect(await screen.findByLabelText('Sichtbarkeit')).toBeInTheDocument()
    expect(screen.queryByLabelText('Karte ausblenden')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Details/ }))
    await user.click(screen.getByLabelText('Karte ausblenden'))
    expect(screen.queryByLabelText('Geschwindigkeit')).not.toBeInTheDocument()
    await user.click(screen.getByLabelText('Leistung ausblenden'))
    await user.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(updateActivity).toHaveBeenCalledWith('act-1', expect.objectContaining({
      hide_map: true,
      data_visibility: expect.objectContaining({ power: false }),
    }))
  })
})

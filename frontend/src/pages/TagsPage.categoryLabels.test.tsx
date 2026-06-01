import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Tag } from '@/api/tags'
import { i18n } from '@/i18n/i18n'
import TagsPage from './TagsPage'

vi.mock('@/api/tags', async () => {
  const makeTag = (id: string, name: string, category: string): Tag => ({
    id,
    name,
    category,
    color: '#6366f1',
    text_color: '#ffffff',
    description: '',
    is_system: category === 'system',
    is_auto: false,
    conditions: null,
    conditions_logic: 'and',
    condition_groups: null,
    activity_count: 0,
    created_at: '2026-01-01T00:00:00Z',
  })

  return {
    fetchTags: vi.fn(async () => [
      makeTag('custom-tag', 'Custom tag', 'custom'),
      makeTag('weather-tag', 'Weather tag', 'weather'),
      makeTag('location-tag', 'Location tag', 'standort'),
      makeTag('system-tag', 'System tag', 'system'),
      makeTag('unknown-tag', 'Unknown tag', 'new_backend_category'),
    ]),
    createTag: vi.fn(),
    updateTag: vi.fn(),
    deleteTag: vi.fn(),
    mergeTags: vi.fn(),
    fetchTagActivities: vi.fn(),
    unassignActivitiesFromTag: vi.fn(),
    upsertTagZone: vi.fn(),
    deleteTagZone: vi.fn(),
    deleteOrphanTag: vi.fn(),
    seedDefaultTags: vi.fn(async () => ({ created: 0 })),
  }
})

vi.mock('maplibre-gl', () => ({
  default: {
    Map: vi.fn(),
    Marker: vi.fn(),
  },
}))

vi.mock('@/components/ZoneEditor', () => ({
  default: () => null,
}))

vi.mock('@/lib/modal', () => ({
  useModal: () => ({ openModal: vi.fn() }),
}))

function renderTagsPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <TagsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('TagsPage category labels', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('localizes known tag categories and hides unknown backend tokens in every locale', async () => {
    const expectedByLocale: Record<string, string[]> = {
      de: ['Eigene', 'Wetter', 'Standort', 'System', 'Unbekannt'],
      en: ['Custom', 'Weather', 'Location', 'System', 'Unknown'],
      fr: ['Personnalisé', 'Météo', 'Lieu', 'Système', 'Inconnu'],
      it: ['Personalizzato', 'Meteo', 'Posizione', 'Sistema', 'Sconosciuto'],
      es: ['Personalizado', 'Tiempo', 'Ubicación', 'Sistema', 'Desconocido'],
    }

    for (const [locale, labels] of Object.entries(expectedByLocale)) {
      await i18n.changeLanguage(locale)
      const { unmount } = renderTagsPage()

      await screen.findAllByText('Custom tag')
      for (const label of labels) {
        expect(screen.getAllByText(label).length).toBeGreaterThan(0)
      }
      await waitFor(() => expect(document.body).not.toHaveTextContent('new_backend_category'))
      expect(document.body).not.toHaveTextContent('tagsPage.categories.')

      unmount()
    }
  })
})

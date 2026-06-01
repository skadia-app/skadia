import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import TagsBadges from '../TagsBadges'
import { assignActivitiesToTag, createTag } from '@/api/tags'
import { updateActivity } from '@/api/activities'

vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn(async () => []),
  createTag: vi.fn(async () => ({ id: 'tag-new', name: 'Neu', activity_count: 0 })),
  assignActivitiesToTag: vi.fn(async () => undefined),
  unassignActivitiesFromTag: vi.fn(),
}))

vi.mock('@/api/activities', () => ({
  updateActivity: vi.fn(async () => ({ id: 'act-1', tags: ['Neu'] })),
}))

function renderTags() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <TagsBadges tags={[]} activityId="act-1" />
    </QueryClientProvider>,
  )
}

describe('TagsBadges', () => {
  it('assigns newly created tags through the tag junction endpoint', async () => {
    renderTags()

    fireEvent.click(screen.getByTitle('Tag hinzufügen'))
    fireEvent.change(screen.getByPlaceholderText('Tag suchen oder erstellen...'), { target: { value: 'Neu' } })
    fireEvent.click(screen.getByText('„Neu“ erstellen'))

    await waitFor(() => {
      expect(createTag).toHaveBeenCalledWith({ name: 'Neu' })
      expect(assignActivitiesToTag).toHaveBeenCalledWith('tag-new', ['act-1'])
      expect(updateActivity).toHaveBeenCalledWith('act-1', {})
    })
  })
})

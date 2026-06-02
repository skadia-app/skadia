import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import UserContentPanel from './UserContentPanel'
import type { TargetUserContent } from '@/api/targetOverlays'
import '@/i18n/i18n'

vi.mock('@/api/targetOverlays', async () => {
  const actual = await vi.importActual<typeof import('@/api/targetOverlays')>('@/api/targetOverlays')
  return {
    ...actual,
    createTargetUserContent: vi.fn(),
  }
})

vi.mock('@/components/target-objects/VisibilityAccessField', () => ({
  default: () => <div data-testid="visibility-access-field" />,
}))

function content(kind: TargetUserContent['content_kind'], payload: Record<string, unknown> = {}): TargetUserContent {
  return {
    id: `${kind}-1`,
    subject_kind: 'course',
    subject_id: 'seg-1',
    content_kind: kind,
    payload,
    visibility: 'public',
    moderation_state: 'visible',
    created_at: '2026-05-22T00:00:00Z',
    updated_at: '2026-05-22T00:00:00Z',
  }
}

describe('UserContentPanel', () => {
  it('shows non-comment content kinds as explicit read-only states', () => {
    render(
      <UserContentPanel
        subjectKind="course"
        subjectId="seg-1"
        canWrite
        content={[
          content('rating', { value: 4 }),
          content('save'),
          content('completion'),
          content('check_in'),
        ]}
      />,
    )

    expect(screen.getByText('1 Bewertung')).toBeInTheDocument()
    expect(screen.getByText('1 Merkung')).toBeInTheDocument()
    expect(screen.getByText('1 Abschluss')).toBeInTheDocument()
    expect(screen.getByText('1 Check-in')).toBeInTheDocument()
    expect(screen.getByText('Weitere Inhalte sind derzeit nur lesbar.')).toBeInTheDocument()
  })
})

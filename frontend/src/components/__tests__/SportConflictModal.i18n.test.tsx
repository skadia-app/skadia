import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import type { Activity } from '@/api/types'
import SportConflictModal from '../SportConflictModal'

vi.mock('@/api/sportConflict', () => ({
  resolveSportConflict: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

const activityWithConflict = (source: string): Activity => ({
  id: 'activity-1',
  sport_type: 'ride',
  sub_type: null,
  sport_conflict_pending: true,
  sport_conflict: {
    platform_sport: 'ride',
    file_sport: 'run',
    platform_source: source,
    file_source: 'fit',
  },
} as Activity)

describe('SportConflictModal i18n labels', () => {
  it('does not expose unknown backend source tokens', async () => {
    await i18n.changeLanguage('de')

    render(
      <SportConflictModal
        activity={activityWithConflict('private_backend_source')}
        open
        onClose={vi.fn()}
        onResolved={vi.fn()}
      />,
    )

    expect(screen.getByText('Unbekannte Quelle')).toBeInTheDocument()
    expect(screen.queryByText('private_backend_source')).not.toBeInTheDocument()
  })
})

import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import SportConflictBanner from '../fusion/SportConflictBanner'

describe('SportConflictBanner', () => {
  it('renders all active modifiers instead of collapsing to one suffix', () => {
    render(
      <SportConflictBanner
        choices={[{
          key: 'cycling:mountain:eiv',
          sportType: 'cycling',
          subType: 'mountain',
          isElectric: true,
          isIndoor: true,
          isVirtual: true,
        }]}
        selectedKey={null}
        onSelect={vi.fn()}
      />,
    )

    expect(screen.getByRole('button')).toHaveTextContent('E/I/V')
  })
})

import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it } from 'vitest'

import GearComponentsTab from './GearComponentsTab'
import type { GearComponent } from '@/api/types'

function makeComponent(overrides: Partial<GearComponent> = {}): GearComponent {
  return {
    id: 'c1',
    gear_id: 'g1',
    name: 'Sattel',
    type: 'saddle',
    total_distance_m: 1000,
    total_elevation_gain_m: 100,
    total_elevation_loss_m: 0,
    total_moving_time_s: 3600,
    total_time_s: 3600,
    total_activities: 0,
    is_retired: false,
    status: 'active',
    ...overrides,
  }
}

describe('GearComponentsTab', () => {
  it('TestRendersComponentListWithHierarchyIndent', () => {
    const root = makeComponent({ id: 'root', name: 'Gabel', parent_component_id: null })
    const child = makeComponent({
      id: 'child',
      name: 'Dämpfungspatrone',
      parent_component_id: 'root',
    })

    render(<GearComponentsTab components={[root, child]} />)

    expect(screen.getByText('Gabel')).toBeInTheDocument()
    expect(screen.getByText('Dämpfungspatrone')).toBeInTheDocument()

    // Child should be visually indented via data-depth attribute.
    const childEl = screen.getByTestId('component-child')
    expect(childEl).toHaveAttribute('data-depth', '1')
  })

  it('renders empty state when no components', () => {
    render(<GearComponentsTab components={[]} />)
    expect(screen.getByText(/Keine Komponenten/i)).toBeInTheDocument()
  })

  it('renders compact rounded distance and duration summaries', () => {
    render(<GearComponentsTab components={[
      makeComponent({
        id: 'c1',
        total_distance_m: 12_450,
        total_time_s: 5_460,
      }),
      makeComponent({
        id: 'c2',
        name: 'Ventil',
        total_distance_m: 490,
        total_time_s: 2_700,
      }),
    ]} />)

    expect(screen.getByTestId('component-c1')).toHaveTextContent('12 km')
    expect(screen.getByTestId('component-c1')).toHaveTextContent('2 h')
    expect(screen.getByTestId('component-c2')).toHaveTextContent('0 km')
    expect(screen.getByTestId('component-c2')).toHaveTextContent('45 min')
  })
})

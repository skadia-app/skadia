import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it } from 'vitest'

import ConditionalField from '@/components/gear/inputs/ConditionalField'
import type { Attribute } from '@/lib/generatedGearSchema'

const attrNoShowWhen: Attribute = {
  key: 'bike.wheel_size',
  labelDe: 'Laufradgröße',
  valueType: 'enum',
}

const attrWithShowWhen: Attribute = {
  key: 'bike.wheel_size_front',
  labelDe: 'Laufradgröße vorne',
  valueType: 'enum',
  showWhen: { field: 'bike.wheel_size', value: 'mullet' },
}

describe('ConditionalField', () => {
  it('TestRendersChildrenWhenAttributeHasNoShowWhen', () => {
    render(
      <ConditionalField attribute={attrNoShowWhen} siblingValues={{}}>
        <div data-testid="child">Inhalt</div>
      </ConditionalField>,
    )
    expect(screen.getByTestId('child')).toBeInTheDocument()
  })

  it('TestRendersChildrenWhenShowWhenMatches', () => {
    render(
      <ConditionalField
        attribute={attrWithShowWhen}
        siblingValues={{ 'bike.wheel_size': 'mullet' }}
      >
        <div data-testid="child">Vorne-Größe</div>
      </ConditionalField>,
    )
    expect(screen.getByTestId('child')).toBeInTheDocument()
  })

  it('TestRendersNothingWhenShowWhenDoesNotMatch', () => {
    render(
      <ConditionalField
        attribute={attrWithShowWhen}
        siblingValues={{ 'bike.wheel_size': 's29' }}
      >
        <div data-testid="child">Vorne-Größe</div>
      </ConditionalField>,
    )
    expect(screen.queryByTestId('child')).not.toBeInTheDocument()
  })

  it('TestRendersNothingWhenSiblingFieldMissing', () => {
    render(
      <ConditionalField attribute={attrWithShowWhen} siblingValues={{}}>
        <div data-testid="child">Vorne-Größe</div>
      </ConditionalField>,
    )
    expect(screen.queryByTestId('child')).not.toBeInTheDocument()
  })

  it('TestUpdatesVisibilityOnSiblingChange', () => {
    // Simulate a sibling value change by re-rendering with different props
    const { rerender } = render(
      <ConditionalField
        attribute={attrWithShowWhen}
        siblingValues={{ 'bike.wheel_size': 's29' }}
      >
        <div data-testid="child">Vorne-Größe</div>
      </ConditionalField>,
    )
    expect(screen.queryByTestId('child')).not.toBeInTheDocument()

    rerender(
      <ConditionalField
        attribute={attrWithShowWhen}
        siblingValues={{ 'bike.wheel_size': 'mullet' }}
      >
        <div data-testid="child">Vorne-Größe</div>
      </ConditionalField>,
    )
    expect(screen.getByTestId('child')).toBeInTheDocument()
  })
})

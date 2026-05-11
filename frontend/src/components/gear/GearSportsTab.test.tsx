import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'

import GearSportsTab from './GearSportsTab'
import type { Gear, GearDefaultSportPredicate } from '@/api/types'

function makeGear(overrides: Partial<Gear> = {}): Gear {
  return {
    id: 'g1',
    user_id: 'u1',
    name: 'Testrad',
    type: 'bike',
    total_distance_m: 0,
    total_elevation_gain_m: 0,
    total_elevation_loss_m: 0,
    total_time_s: 0,
    total_activities: 0,
    is_retired: false,
    status: 'active',
    applicable_sport_predicates: [],
    created_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('GearSportsTab', () => {
  it('TestRendersApplicableSportPredicatesAndModifiers', () => {
    const predicates: GearDefaultSportPredicate[] = [
      { sport_type: 'cycling', sub_type: 'mountain' },
    ]
    const onSave = vi.fn()

    render(
      <GearSportsTab
        item={makeGear({ applicable_sport_predicates: predicates })}
        onSave={onSave}
      />,
    )

    // Predicate rows are rendered.
    expect(screen.getByRole('combobox', { name: /Sportart 1/i })).toBeInTheDocument()

    // Modifier toggles are rendered (is_electric, is_indoor, is_virtual).
    expect(screen.getByRole('combobox', { name: /E-Antrieb 1/i })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: /Indoor 1/i })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: /Virtuell 1/i })).toBeInTheDocument()

    // Save button is present.
    expect(screen.getByRole('button', { name: /Speichern/i })).toBeInTheDocument()
  })

  it('renders empty predicate list with add button when predicates are empty', () => {
    render(
      <GearSportsTab
        item={makeGear({ applicable_sport_predicates: [] })}
        onSave={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: /Sportart hinzufügen/i })).toBeInTheDocument()
  })

  it('TestHelmetWithSkiSubcategoryDefaultsToSkiSports', () => {
    // When predicates are empty but the item has helmet.subcategory=ski,
    // the tab should pre-populate rows for skiing and snowboarding.
    render(
      <GearSportsTab
        item={makeGear({
          type: 'helmet',
          applicable_sport_predicates: [],
          attributes: { 'helmet.subcategory': 'ski' },
        })}
        onSave={vi.fn()}
      />,
    )

    // Two sport rows must be rendered (skiing + snowboarding).
    const selects = screen.getAllByRole('combobox', { name: /Sportart \d+/i })
    expect(selects).toHaveLength(2)

    const sportValues = selects.map(s => (s as HTMLSelectElement).value)
    // alpine_skiing is used (selectable=true); the broad-fallback 'skiing' type is not in the select options
    expect(sportValues).toContain('alpine_skiing')
    expect(sportValues).toContain('snowboarding')
  })

  it('TestGogglesWithSwimSubcategoryDefaultsToSwimming', () => {
    render(
      <GearSportsTab
        item={makeGear({
          type: 'goggles',
          applicable_sport_predicates: [],
          attributes: { 'goggles.subcategory': 'swim' },
        })}
        onSave={vi.fn()}
      />,
    )

    const selects = screen.getAllByRole('combobox', { name: /Sportart 1/i })
    const sportValue = (selects[0] as HTMLSelectElement).value
    expect(sportValue).toBe('swimming')
  })

  it('TestApparelJerseyWithCyclingSubcategoryDefaultsToCycling', () => {
    render(
      <GearSportsTab
        item={makeGear({
          type: 'apparel.jersey',
          applicable_sport_predicates: [],
          attributes: { 'apparel.subcategory': 'cycling' },
        })}
        onSave={vi.fn()}
      />,
    )

    const selects = screen.getAllByRole('combobox', { name: /Sportart 1/i })
    const sportValue = (selects[0] as HTMLSelectElement).value
    expect(sportValue).toBe('cycling')
  })

  it('TestExistingPredicatesAreNotOverriddenBySubcategoryDefaults', () => {
    // If the user already has predicates, subcategory defaults must not overwrite them.
    const predicates: GearDefaultSportPredicate[] = [
      { sport_type: 'running' },
    ]
    render(
      <GearSportsTab
        item={makeGear({
          type: 'helmet',
          applicable_sport_predicates: predicates,
          attributes: { 'helmet.subcategory': 'ski' },
        })}
        onSave={vi.fn()}
      />,
    )

    // Only the user's existing row must be present — no ski defaults injected.
    const selects = screen.getAllByRole('combobox', { name: /Sportart \d+/i })
    expect(selects).toHaveLength(1)
    expect((selects[0] as HTMLSelectElement).value).toBe('running')
  })

  // R2-5: modifier hint tests

  it('TestModifierHintShowsEmpfohlenForRecommendedGearType', () => {
    // bike.city.citybike is in is_electric recommendedFor list.
    // When modifier value is 'true', hint "✓ empfohlen" should appear.
    const predicates: GearDefaultSportPredicate[] = [
      { sport_type: 'cycling', modifiers: { is_electric: true } },
    ]
    render(
      <GearSportsTab
        item={makeGear({
          type: 'bike.city.citybike',
          applicable_sport_predicates: predicates,
        })}
        gearType="bike.city.citybike"
        onSave={vi.fn()}
      />,
    )
    expect(screen.getByText('✓ empfohlen')).toBeInTheDocument()
  })

  it('TestModifierHintShowsUngewoehnlichForNotRecommendedGearType', () => {
    // bike.road.track is in is_electric notRecommendedFor list.
    const predicates: GearDefaultSportPredicate[] = [
      { sport_type: 'cycling', modifiers: { is_electric: true } },
    ]
    render(
      <GearSportsTab
        item={makeGear({
          type: 'bike.road.track',
          applicable_sport_predicates: predicates,
        })}
        gearType="bike.road.track"
        onSave={vi.fn()}
      />,
    )
    expect(screen.getByText('⚠ ungewöhnlich')).toBeInTheDocument()
  })

  it('TestModifierHintAbsentWhenModifierNotSet', () => {
    // No hint when modifier is 'any' (not explicitly set to true).
    const predicates: GearDefaultSportPredicate[] = [
      { sport_type: 'cycling' },
    ]
    render(
      <GearSportsTab
        item={makeGear({
          type: 'bike.city.citybike',
          applicable_sport_predicates: predicates,
        })}
        gearType="bike.city.citybike"
        onSave={vi.fn()}
      />,
    )
    expect(screen.queryByText('✓ empfohlen')).not.toBeInTheDocument()
    expect(screen.queryByText('⚠ ungewöhnlich')).not.toBeInTheDocument()
  })

  it('TestModifierHintAbsentForNeutralGearType', () => {
    // bike.road is neither in recommendedFor nor notRecommendedFor for is_electric → no hint.
    const predicates: GearDefaultSportPredicate[] = [
      { sport_type: 'cycling', modifiers: { is_electric: true } },
    ]
    render(
      <GearSportsTab
        item={makeGear({
          type: 'bike.road',
          applicable_sport_predicates: predicates,
        })}
        gearType="bike.road"
        onSave={vi.fn()}
      />,
    )
    expect(screen.queryByText('✓ empfohlen')).not.toBeInTheDocument()
    expect(screen.queryByText('⚠ ungewöhnlich')).not.toBeInTheDocument()
  })
})

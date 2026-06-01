import { describe, expect, it } from 'vitest'
import {
  descriptorLabel,
  loopQualityDisplay,
  scaleAriaLabel,
  scaleMaxLabel,
  statsSummitsLabel,
  variantLabel,
  type ScaleKind,
  type SuggestRoutesTranslator,
} from './suggest-routes'

const t: SuggestRoutesTranslator = (key, options) => {
  if (key === 'routePlanner.summits.summary') {
    return `summits:${options?.count}`
  }
  if (key === 'routePlanner.profileChart.scaleMax') {
    return `scaleMax:${options?.level}`
  }
  if (key === 'routePlanner.profileChart.scaleAria') {
    return `scaleAria:${options?.kind}:${options?.level}`
  }
  return `translated:${key}`
}

describe('suggest route string helpers', () => {
  it('resolves suggestion variant and descriptor labels through route planner catalog keys', () => {
    expect(variantLabel('medium-2', t)).toBe('translated:routePlanner.comparison.variants.medium')
    expect(descriptorLabel('high_ascent', '820 m', t)).toBe('translated:routePlanner.comparison.descriptors.high_ascent (820 m)')
  })

  it('keeps backend extension ids as technical fallbacks instead of guessing translations', () => {
    expect(variantLabel('experimental_choice', t)).toBe('experimental_choice')
    expect(descriptorLabel('backend_experiment', undefined, t)).toBe('backend_experiment')
  })

  it('resolves loop quality and profile chart labels through catalog keys', () => {
    expect(loopQualityDisplay('partial_loop', t)).toEqual({
      label: 'translated:routePlanner.comparison.loopQuality.partial_loop',
      icon: '⟳',
    })
    expect(statsSummitsLabel(3, t)).toBe('summits:3')
    expect(scaleMaxLabel('T3', t)).toBe('scaleMax:T3')
    expect(scaleAriaLabel('mtb' satisfies ScaleKind, 'S3', t)).toBe('scaleAria:translated:routePlanner.profileChart.scaleKind.mtb:S3')
  })
})

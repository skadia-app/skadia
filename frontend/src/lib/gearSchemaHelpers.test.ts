import { describe, expect, it } from 'vitest'

import { gearSchemaLabel, gearSchemaLabelCandidates } from './gearSchemaHelpers'

describe('gearSchemaLabel', () => {
  it('uses localized labels before falling back to labelDe', () => {
    expect(
      gearSchemaLabel(
        {
          labelDe: 'Rahmenmaterial',
          labels: { de: 'Rahmenmaterial', en: 'Frame material' },
        },
        'en',
      ),
    ).toBe('Frame material')
  })

  it('falls back to labelDe only when labels are absent', () => {
    expect(
      gearSchemaLabel(
        {
          labelDe: 'Rahmenmaterial',
        },
        'en',
      ),
    ).toBe('Rahmenmaterial')
  })

  it('returns unique labels across all supported languages for schema comparisons', () => {
    expect(
      gearSchemaLabelCandidates({
        labelDe: 'Rahmenmaterial',
        labels: {
          de: 'Rahmenmaterial',
          en: 'Frame material',
          fr: 'Matériau du cadre',
          it: 'Materiale telaio',
          es: 'Material del cuadro',
        },
      }),
    ).toEqual([
      'Rahmenmaterial',
      'Frame material',
      'Matériau du cadre',
      'Materiale telaio',
      'Material del cuadro',
    ])
  })
})

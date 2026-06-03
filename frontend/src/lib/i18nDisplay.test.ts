import { describe, expect, it } from 'vitest'

import { i18n } from '@/i18n/i18n'
import { displaySystemTagName } from './i18nDisplay'

describe('displaySystemTagName', () => {
  it('localizes legacy weather tag names and running-distance tags', async () => {
    await i18n.changeLanguage('es')

    expect(displaySystemTagName('bewölkt', i18n.t)).toBe('nublado')
    expect(displaySystemTagName('sonnig', i18n.t)).toBe('soleado')
    expect(displaySystemTagName('Regen', i18n.t)).toBe('lluvia')
    expect(displaySystemTagName('Schnee', i18n.t)).toBe('nieve')
    expect(displaySystemTagName('Nebel', i18n.t)).toBe('niebla')
    expect(displaySystemTagName('Gewitter', i18n.t)).toBe('tormenta')
    expect(displaySystemTagName('Halbmarathon', i18n.t)).toBe('Media maratón')
    expect(displaySystemTagName('Marathon', i18n.t)).toBe('Maratón')
    expect(displaySystemTagName('Ultramarathon', i18n.t)).toBe('Ultramaratón')
  })

  it('keeps unknown user tag names unchanged', async () => {
    await i18n.changeLanguage('fr')

    expect(displaySystemTagName('Bloc-Hütte', i18n.t)).toBe('Bloc-Hütte')
  })
})

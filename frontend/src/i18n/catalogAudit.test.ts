import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import ts from 'typescript'
import { auditCatalogResources } from './catalogAudit'
import { resources } from './resources'

const scopedGermanFragments = [
  'Zeitraum',
  'wählen',
  'Länder',
  'anklicken',
  'einschließen',
  'ausschließen',
  'Farbgruppe',
  'zurücksetzen',
  'Nach Sportart',
  'Standort',
  'Segmente',
  'Strecken',
  'Nur gefahren',
  'Favoriten',
  'Gewässer',
  'besuchte',
  'Kartenname',
  'Beschreibung',
  'fehlgeschlagen',
  'Speichern',
  'Liftfahrten',
  'Vorschläge',
  'Ablehnbar',
  'Treffer',
  'Heuristik',
  'Höhen',
  'Prüfe',
  'Region entfernt',
  'Inkrementelles',
  'Aktualisierung',
]

const scopedEnglishFallbackFragments = [
  'Delete map',
  'Delete marker',
  'Deleting cannot be undone',
  'Click on map',
  'Add marker',
  'Map name',
  'New map',
  'Description (optional)',
  'Image upload failed',
  'Replace image',
  'Add image',
  'Save failed',
]

const scopedCatalogPrefixes = [
  'map.filters',
  'map.layers',
  'map.customMapPanel',
  'map.customMaps',
  'map.offline',
  'lift.help',
  'osm.regions',
]

function collectScopedValues(value: unknown, path = ''): Array<{ path: string; value: string }> {
  if (typeof value === 'string') {
    return scopedCatalogPrefixes.some(prefix => path === prefix || path.startsWith(`${prefix}.`))
      ? [{ path, value }]
      : []
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return []
  return Object.entries(value).flatMap(([key, child]) => collectScopedValues(child, path ? `${path}.${key}` : key))
}

function catalogValue(locale: keyof typeof resources, path: string): string {
  const value = path.split('.').reduce<unknown>((node, key) => {
    if (!node || typeof node !== 'object') return undefined
    return (node as Record<string, unknown>)[key]
  }, resources[locale])

  if (typeof value !== 'string') {
    throw new Error(`${locale}.${path} is not a string`)
  }
  return value
}

function catalogNode(locale: keyof typeof resources, path: string): unknown {
  return path.split('.').reduce<unknown>((node, key) => {
    if (!node || typeof node !== 'object') return undefined
    return (node as Record<string, unknown>)[key]
  }, resources[locale])
}

function readinessSourceCodes(): string[] {
  const source = readFileSync(resolve(process.cwd(), 'src/api/analytics.ts'), 'utf8')
  const sourceFile = ts.createSourceFile('analytics.ts', source, ts.ScriptTarget.Latest, true)
  let typeNode: ts.TypeAliasDeclaration | undefined

  sourceFile.forEachChild(node => {
    if (ts.isTypeAliasDeclaration(node) && node.name.text === 'ReadinessSource') {
      typeNode = node
    }
  })

  if (!typeNode) {
    throw new Error('ReadinessSource union not found in src/api/analytics.ts')
  }

  if (!ts.isUnionTypeNode(typeNode.type)) {
    throw new Error('ReadinessSource is not a string-literal union in src/api/analytics.ts')
  }

  return typeNode.type.types.map(node => {
    if (!ts.isLiteralTypeNode(node) || !ts.isStringLiteral(node.literal)) {
      throw new Error('ReadinessSource contains a non-string-literal member')
    }
    return node.literal.text
  })
}

describe('auditCatalogResources', () => {
  it('accepts the shipped locale catalogs', () => {
    expect(auditCatalogResources(resources)).toEqual([])
  })

  it('does not leave German UI text in the map, lift, and OSM target catalogs', () => {
    const issues = (['en', 'fr', 'it', 'es'] as const).flatMap(locale =>
      collectScopedValues(resources[locale])
        .filter(entry => scopedGermanFragments.some(fragment => entry.value.includes(fragment)))
        .map(entry => ({ locale, ...entry })),
    )

    expect(issues).toEqual([])
  })

  it('does not leave English fallback text in translated map target catalogs', () => {
    const issues = (['fr', 'it', 'es'] as const).flatMap(locale =>
      collectScopedValues(resources[locale])
        .filter(entry => scopedEnglishFallbackFragments.some(fragment => entry.value.includes(fragment)))
        .map(entry => ({ locale, ...entry })),
    )

    expect(issues).toEqual([])
  })

  it('does not leave English fallback labels in translated analytics-health UI text', () => {
    const scopedTerms = [
      { path: 'dashboardPage.baseline', fragments: ['Baseline'] },
      { path: 'analyticsReadiness.missingReasons.no_baseline', fragments: ['Baseline'] },
      { path: 'analyticsHealthInsights.runningForm.tier', fragments: ['Tier'] },
      { path: 'analyticsHealthInsights.runningForm.baselineCount_one', fragments: ['Baseline'] },
      { path: 'analyticsHealthInsights.runningForm.baselineCount_other', fragments: ['Baseline'] },
      { path: 'analyticsHealthInsights.hrvAnalysis.baselineBand', fragments: ['Baseline', 'SD'] },
      { path: 'analyticsHealthInsights.hrvAnalysis.baselineValue', fragments: ['Baseline'] },
      { path: 'analyticsHealthInsights.hrvAnalysis.sevenDayAverage', fragments: ['Avg'] },
      { path: 'analyticsHealthInsights.sleep.averageDuration', fragments: ['Avg'] },
      { path: 'analyticsHealthInsights.sleep.averageScore', fragments: ['Avg'] },
    ]

    const issues = (['de', 'fr', 'it', 'es'] as const).flatMap(locale =>
      scopedTerms.flatMap(({ path, fragments }) => {
        const value = catalogValue(locale, path)
        return fragments
          .filter(fragment => value.includes(fragment))
          .map(fragment => ({ locale, path, fragment, value }))
      }),
    )

    expect(issues).toEqual([])
  })

  it('ships unknown labels for guarded segment editor and POI codes', () => {
    const guardedKeys = [
      'segmentsList.surfaces.unknown',
      'routePlanner.poiSearch.types.unknown',
    ]

    const issues = (['de', 'en', 'fr', 'it', 'es'] as const).flatMap(locale =>
      guardedKeys.flatMap(path => {
        const value = path.split('.').reduce<unknown>((node, key) => {
          if (!node || typeof node !== 'object') return undefined
          return (node as Record<string, unknown>)[key]
        }, resources[locale])

        return typeof value === 'string' && value.length > 0 ? [] : [{ locale, path }]
      }),
    )

    expect(issues).toEqual([])
  })

  it('keeps every readiness source code represented in the source-label namespace', () => {
    const sourceCodes = readinessSourceCodes()
    const issues = (['de', 'en', 'fr', 'it', 'es'] as const).flatMap(locale => {
      const labels = catalogNode(locale, 'analyticsReadiness.sources')
      if (!labels || typeof labels !== 'object' || Array.isArray(labels)) {
        return sourceCodes.map(source => ({ locale, source, reason: 'missing-namespace' }))
      }

      return sourceCodes
        .filter(source => typeof (labels as Record<string, unknown>)[source] !== 'string')
        .map(source => ({ locale, source, reason: 'missing-label' }))
    })

    expect(issues).toEqual([])
  })

  it('reports missing keys in target locales', () => {
    const issues = auditCatalogResources({
      de: { common: { settings: 'Einstellungen', logout: 'Abmelden' } },
      en: { common: { settings: 'Settings' } },
      fr: { common: { settings: 'Parametres', logout: 'Deconnexion' } },
      it: { common: { settings: 'Impostazioni', logout: 'Esci' } },
      es: { common: { settings: 'Ajustes', logout: 'Cerrar sesion' } },
    })

    expect(issues).toContainEqual({
      code: 'missing-key',
      locale: 'en',
      key: 'common.logout',
    })
  })

  it('requires protected terms from the German source in every target locale', () => {
    const issues = auditCatalogResources({
      de: { gear: { mountainbike: 'Mountainbike-Profil' } },
      en: { gear: { mountainbike: 'Mountain bike profile' } },
      fr: { gear: { mountainbike: 'Profil Mountainbike' } },
      it: { gear: { mountainbike: 'Profilo Mountainbike' } },
      es: { gear: { mountainbike: 'Perfil Mountainbike' } },
    })

    expect(issues).toContainEqual({
      code: 'protected-term-changed',
      locale: 'en',
      key: 'gear.mountainbike',
      term: 'Mountainbike',
    })
  })

  it('reports ASCII umlaut spellings in German catalog text', () => {
    const issues = auditCatalogResources({
      de: { drawing: { hint: 'Doppelklick oder "Schliessen" zum Beenden.' } },
      en: { drawing: { hint: 'Double-click or "Close" to finish.' } },
      fr: { drawing: { hint: 'Double-cliquez ou choisissez "Fermer" pour terminer.' } },
      it: { drawing: { hint: 'Fai doppio clic o scegli "Chiudi" per terminare.' } },
      es: { drawing: { hint: 'Haz doble clic o elige "Cerrar" para terminar.' } },
    })

    expect(issues).toContainEqual({
      code: 'german-ascii-umlaut',
      locale: 'de',
      key: 'drawing.hint',
      text: 'Doppelklick oder "Schliessen" zum Beenden.',
    })
  })
})

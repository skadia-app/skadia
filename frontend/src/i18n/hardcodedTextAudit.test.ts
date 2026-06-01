import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { auditSourceText } from './hardcodedTextAudit'

const healthChartFilesWithoutDefaultFallbacks = [
  'src/components/analytics/filters/RangePresetBar.tsx',
  'src/components/analytics/filters/HealthRangePresetBar.tsx',
  'src/components/analytics/health/SpO2Chart.tsx',
  'src/components/analytics/health/StressChart.tsx',
  'src/components/analytics/health/FitnessAgeChart.tsx',
  'src/components/analytics/health/BloodPressureChart.tsx',
]

const healthRangeFilterFilesWithoutGermanFallbacks = [
  'src/components/analytics/filters/RangePresetBar.tsx',
  'src/components/analytics/filters/HealthRangePresetBar.tsx',
]

const difficultyAndSummitFilesWithoutVisibleConstantLabels = [
  'src/components/DifficultyBadge.tsx',
  'src/components/summit/summitConstants.ts',
]

const routeSuggestionFilesWithoutVisibleFallbacks = [
  'src/lib/strings/suggest-routes.ts',
]

const editorPoiLabelFilesWithoutRawDefaultFallbacks = [
  'src/components/segment-editor/ClimbTypePills.tsx',
  'src/components/segment-editor/SurfaceDropdown.tsx',
  'src/components/route-planner/POISearchInput.tsx',
]

const activityChartFilesWithoutPowerPhaseFallbacks = [
  'src/components/ActivityCharts.tsx',
]

const rawBackendLabelFallbackWatchlist = [
  'src/components/GearDocumentSection.tsx',
  'src/components/SplitTable.tsx',
  'src/components/analytics/insights/WarningsPanel.tsx',
  'src/components/target-objects/targetLabels.ts',
  'src/lib/jobLabels.ts',
  'src/pages/GearDetailPage.tsx',
  'src/pages/ImportsPage.tsx',
  'src/pages/TargetObjectDetailPage.tsx',
]

const gearDocumentAndDetailFiles = [
  'src/components/GearDocumentSection.tsx',
  'src/pages/GearDetailPage.tsx',
]

const forbiddenHealthRangeFallbacks = [
  'Benutzerdefiniert',
  'Letzte 7 Tage',
  '30 Tage',
  '90 Tage',
  '1 Jahr',
]

function readFrontendFile(path: string): string {
  return readFileSync(resolve(process.cwd(), path), 'utf8')
}

describe('auditSourceText', () => {
  it('flags German JSX text even when it has no umlaut', () => {
    const issues = auditSourceText('Example.tsx', `export function Example() { return <button>Abmelden</button> }`)

    expect(issues).toContainEqual({
      code: 'hardcoded-ui-text',
      file: 'Example.tsx',
      text: 'Abmelden',
      location: 'jsx-text',
    })
  })

  it('flags translated UI attributes', () => {
    const issues = auditSourceText(
      'Example.tsx',
      `export function Example() { return <button aria-label="Benachrichtigungen" title="Alle löschen" /> }`,
    )

    expect(issues.map(issue => issue.text)).toEqual(['Benachrichtigungen', 'Alle löschen'])
  })

  it('allows API paths, CSS classes, and enum-like values', () => {
    const issues = auditSourceText(
      'Example.tsx',
      `const path = '/api/v1/me'; const cls = 'text-sm font-medium'; const status = 'completed'`,
    )

    expect(issues).toEqual([])
  })

  it('flags user-visible call arguments', () => {
    const issues = auditSourceText(
      'Example.tsx',
      `toast.success('Gespeichert'); new Notification('Import abgeschlossen'); window.alert('Fehler beim Speichern')`,
    )

    expect(issues.map(issue => issue.text)).toEqual([
      'Gespeichert',
      'Import abgeschlossen',
      'Fehler beim Speichern',
    ])
    expect(issues.every(issue => issue.location === 'call-argument')).toBe(true)
  })

  it('flags user-visible object labels but allows translation keys', () => {
    const issues = auditSourceText(
      'Example.tsx',
      `const labels = [{ label: 'Alle Aktivitäten', description: 'Keine Daten vorhanden' }, { label: t('activity.filters.all') }]`,
    )

    expect(issues).toEqual([
      {
        code: 'hardcoded-ui-text',
        file: 'Example.tsx',
        text: 'Alle Aktivitäten',
        location: 'object-property',
      },
      {
        code: 'hardcoded-ui-text',
        file: 'Example.tsx',
        text: 'Keine Daten vorhanden',
        location: 'object-property',
      },
    ])
  })

  it('keeps audited health analytics surfaces on catalog keys instead of hardcoded default fallbacks', () => {
    const issues = healthChartFilesWithoutDefaultFallbacks.flatMap(file => {
      const matches = readFrontendFile(file).matchAll(/default(?:Value|Label):\s*(?:'[^']+'|"[^"]+"|`[^`]+`)/g)
      return Array.from(matches, match => ({ file, text: match[0] }))
    })

    expect(issues).toEqual([])
  })

  it('keeps health range filter components free of German fallback label literals', () => {
    const issues = healthRangeFilterFilesWithoutGermanFallbacks.flatMap(file => {
      const source = readFrontendFile(file)
      return forbiddenHealthRangeFallbacks
        .filter(text => source.includes(text))
        .map(text => ({ file, text }))
    })

    expect(issues).toEqual([])
  })

  it('keeps difficulty and summit filter constants free of visible fallback labels', () => {
    const issues = difficultyAndSummitFilesWithoutVisibleConstantLabels.flatMap(file =>
      auditSourceText(file, readFrontendFile(file)),
    )

    expect(issues).toEqual([])
  })

  it('keeps route suggestion helpers free of visible fallback labels', () => {
    const issues = routeSuggestionFilesWithoutVisibleFallbacks.flatMap(file =>
      auditSourceText(file, readFrontendFile(file)),
    )

    expect(issues).toEqual([])
  })

  it('keeps segment editor and POI labels off raw default fallbacks', () => {
    const issues = editorPoiLabelFilesWithoutRawDefaultFallbacks.flatMap(file => {
      const source = readFrontendFile(file)
      const matches = source.matchAll(
        /segmentEditor\.climbTypes\.[^)\n]*defaultValue|segmentsList\.surfaces\.[^)\n]*defaultValue|routePlanner\.poiSearch\.types\.[^)\n]*defaultValue|\(veraltet\)\s*\$\{value\}|CLIMB_TYPE_LABELS/g,
      )
      return Array.from(matches, match => ({ file, text: match[0] }))
    })

    expect(issues).toEqual([])
  })

  it('keeps activity chart power phase labels on catalog keys instead of visible default fallbacks', () => {
    const issues = activityChartFilesWithoutPowerPhaseFallbacks.flatMap(file => {
      const source = readFrontendFile(file)
      const matches = source.matchAll(
        /activityCharts\.powerPhase(?:Labels|Legend)\.[\w.]+[^)\n]*defaultValue|(?:left|right)PowerPhaseFallback/g,
      )
      return Array.from(matches, match => ({ file, text: match[0] }))
    })

    expect(issues).toEqual([])
  })

  it('keeps activity chart deco loading labels on catalog keys while preserving metric abbreviations', () => {
    const issues = activityChartFilesWithoutPowerPhaseFallbacks.flatMap(file => {
      const source = readFrontendFile(file)
      return ['N₂-Loading'].filter(text => source.includes(text)).map(text => ({ file, text }))
    })

    expect(issues).toEqual([])
  })

  it('keeps watched backend label helpers from rendering raw unknown codes', () => {
    const rawFallbackPatterns = [
      /return\s+labels\[[^\]]+\]\s*\?\?\s*(?:type|value|status|docType)\b/g,
      /return\s+key\s*\?\s*t\([^)]*\)\s*:\s*value\b/g,
      /\?\s*t\([^)]*\)\s*:\s*(?:type|value|status|docType)\b/g,
      /\bunknownCode\b[^'"\n]*\{\{\s*code\s*\}\}/g,
      /\bt\([^)\n]*unknownCode[^)\n]*\{\s*code\s*:/g,
    ]

    const issues = rawBackendLabelFallbackWatchlist.flatMap(file => {
      const source = readFrontendFile(file)
      return rawFallbackPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match => ({ file, text: match[0] })),
      )
    })

    expect(issues).toEqual([])
  })

  it('keeps gear document/detail UI labels on catalog keys while preserving technical values', () => {
    const rawGearLabelPatterns = [
      />\s*SN:\s*\{/g,
      /\b(?:label|title|placeholder):\s*['"](?:E-Bike|Ski|Seriennummer|Serial number)['"]/g,
      /\b(?:ebike|ski):\s*['"](?:E-Bike|Ski)['"]/g,
      /\b(?:label|title|placeholder):\s*['"][^'"]*(?:Dokument|Document|Kosten|Cost|Währung|Currency)[^'"]*['"]/g,
    ]

    const issues = gearDocumentAndDetailFiles.flatMap(file => {
      const source = readFrontendFile(file)
      return rawGearLabelPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match => ({ file, text: match[0] })),
      )
    })

    expect(issues).toEqual([])
  })
})

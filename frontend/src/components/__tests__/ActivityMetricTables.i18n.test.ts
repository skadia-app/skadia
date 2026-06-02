import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

function source(path: string): string {
  return readFileSync(resolve(process.cwd(), path), 'utf8')
}

describe('activity metric table i18n', () => {
  it('uses localized power labels where a sport-specific label is available', () => {
    expect(source('src/components/LapTable.tsx')).not.toContain('<th className={thClass}>Power</th>')
    expect(source('src/components/DetectedLapsTable.tsx')).not.toContain('>Power</th>')
    expect(source('src/components/LapTable.tsx')).toContain("t('colorMetricLabels.metrics.power')")
    expect(source('src/components/DetectedLapsTable.tsx')).toContain("t('colorMetricLabels.metrics.power')")
  })

  it('uses localized zone distribution toggle labels', () => {
    expect(source('src/components/activity/ZoneDistributionChart.tsx')).not.toContain('>HR</button>')
    expect(source('src/components/activity/ZoneDistributionChart.tsx')).not.toContain('>Power</button>')
    expect(source('src/components/activity/ZoneDistributionChart.tsx')).toContain("t('colorMetricLabels.metrics.hr')")
    expect(source('src/components/activity/ZoneDistributionChart.tsx')).toContain("t('colorMetricLabels.metrics.power')")
  })

  it('uses the existing calories catalog key for calories table headers', () => {
    expect(source('src/components/SplitTable.tsx')).not.toContain('<th className={thClass}>kcal</th>')
    expect(source('src/components/SwimLengthTable.tsx')).not.toContain('<th className={thClass}>kcal</th>')
    expect(source('src/components/SplitTable.tsx')).toContain("{t('activityStats.calories')} (kcal)")
    expect(source('src/components/SwimLengthTable.tsx')).toContain("{t('activityStats.calories')} (kcal)")
  })
})

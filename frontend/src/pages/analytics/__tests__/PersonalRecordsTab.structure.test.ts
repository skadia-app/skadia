import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('PersonalRecordsTab records UI structure', () => {
  it('keeps records on pill tabs and subfilters rather than the generic sport filter', () => {
    const source = readFileSync('src/pages/analytics/PersonalRecordsTab.tsx', 'utf8')

    expect(source).toContain('SubFilterPills')
    expect(source).toContain('MAIN_CATEGORY_ORDER')
    expect(source).not.toContain('SportFilter')
  })
})

import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  join(process.cwd(), 'src/components/route-planner/RouteComparisonPanel.tsx'),
  'utf8',
)

describe('RouteComparisonPanel i18n wiring', () => {
  it('does not keep route comparison defaultValue fallbacks in component-local helpers', () => {
    expect(source).not.toMatch(/routePlanner\.comparison\.[^`'"]+[`'"],\s*\{\s*defaultValue:/)
    expect(source).not.toMatch(/function\s+(variantLabel|descriptorLabel|loopQualityDisplay)\s*\(/)
  })
})

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SplitTable from '../SplitTable'
import { i18n } from '@/i18n/i18n'
import type { Split } from '@/api/types'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'de', units: 'metric' }),
}))

describe('SplitTable i18n', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('uses the catalog key for ski lift labels without a hardcoded fallback', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/components/SplitTable.tsx'), 'utf8')

    expect(source).not.toMatch(/activityTables\.splits\.types\.skiLift['"`],\s*\{\s*defaultValue:/)
  })

  it('uses a generic label for unknown split type values', () => {
    const split: Split = {
      id: 'split-1',
      split_index: 0,
      split_type: 'backend_private_split_type',
      start_time: '2026-05-12T00:00:00Z',
      distance_m: 1000,
      elapsed_time_s: 300,
      timer_time_s: 300,
      avg_speed_ms: 3,
    } as unknown as Split

    render(
      <SplitTable
        splits={[split]}
        summaries={[]}
        sportType="running"
        activityId="activity-1"
      />,
    )

    expect(screen.getByText('Unbekannter Split-Typ')).toBeInTheDocument()
    expect(screen.queryByText('backend_private_split_type')).not.toBeInTheDocument()
  })
})

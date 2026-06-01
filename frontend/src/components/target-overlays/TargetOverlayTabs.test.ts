import { describe, expect, it, vi } from 'vitest'
import { reportStatusLabel, reportTypeLabel } from './TargetOverlayTabs'

const t = vi.fn((key: string) => key)

describe('target overlay fallback labels', () => {
  it('uses structured report labels and hides unknown codes', () => {
    expect(reportTypeLabel('condition', t)).toBe('targetOverlays.reports.types.condition')
    expect(reportTypeLabel('new_report_code', t)).toBe('targetOverlays.reports.types.unknown')
    expect(reportStatusLabel('open', t)).toBe('targetOverlays.reports.status.open')
    expect(reportStatusLabel('custom_status_code', t)).toBe('targetOverlays.reports.status.unknown')
  })
})

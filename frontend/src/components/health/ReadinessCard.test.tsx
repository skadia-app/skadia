import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, beforeEach } from 'vitest'
import type { HealthReadinessScore } from '@/api/health'
import { i18n } from '@/i18n/i18n'
import ReadinessCard from './ReadinessCard'

const readiness: HealthReadinessScore = {
  user_id: 'user-1',
  calendar_date: '2026-05-31',
  score: 72,
  level: 'BACKEND_ADDED_LEVEL',
  sleep_score: null,
  hrv_weekly_avg_ms: null,
  recovery_time_min: null,
  acwr_pct: null,
  hrv_factor_pct: null,
  sleep_history_factor_pct: null,
  stress_history_factor_pct: null,
  recovery_time_factor_pct: null,
  source_type: 'manual',
  created_at: '2026-05-31T00:00:00Z',
}

describe('ReadinessCard', () => {
  beforeEach(() => i18n.changeLanguage('de'))

  it('does not expose unknown readiness level codes', () => {
    render(<ReadinessCard data={readiness} />)

    expect(screen.getByText(i18n.t('healthReadiness.levels.unknown'))).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('BACKEND_ADDED_LEVEL')
  })
})

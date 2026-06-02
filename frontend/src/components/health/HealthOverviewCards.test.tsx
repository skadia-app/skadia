import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { HealthOverview } from '@/api/health'
import { i18n } from '@/i18n/i18n'
import HealthOverviewCards from './HealthOverviewCards'

const userPrefs = vi.hoisted(() => ({ locale: 'en', units: 'imperial' as 'metric' | 'imperial' }))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => userPrefs,
}))

describe('HealthOverviewCards', () => {
  it('formats latest weight with the active unit preference', async () => {
    await i18n.changeLanguage('en')

    const data: HealthOverview = {
      weight: {
        user_id: 'user-1',
        measured_at_utc: '2026-05-31T00:00:00Z',
        weight_g: 70_000,
        bmi: 22.5,
        body_fat_pct: null,
        muscle_mass_g: null,
        bone_mass_g: null,
        source_type: 'manual',
        created_at: '2026-05-31T00:00:00Z',
      },
    }

    render(<HealthOverviewCards data={data} />)

    expect(screen.getByText('154.3 lb')).toBeInTheDocument()
    expect(screen.queryByText('70.0 kg')).not.toBeInTheDocument()
  })

  it('does not expose unknown readiness level codes', async () => {
    await i18n.changeLanguage('de')

    const data: HealthOverview = {
      readiness: {
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
      },
    }

    render(<HealthOverviewCards data={data} />)

    expect(screen.getByText(i18n.t('healthReadiness.levels.unknown'))).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('BACKEND_ADDED_LEVEL')
  })
})

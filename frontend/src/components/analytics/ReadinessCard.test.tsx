// ReadinessCard.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ReadinessCardCompact } from './ReadinessCard'
import ReadinessCardFull from './ReadinessCard'
import { I18nProvider } from '@/i18n/I18nProvider'
import type { ReadinessResponse, ReadinessPreferences, ReadinessFactor } from '@/api/analytics'

// ── Mock API ──────────────────────────────────────────────────────────────────

const mockFetchReadiness = vi.fn()
const mockFetchReadinessPreferences = vi.fn()
const mockPutReadinessPreferences = vi.fn()

vi.mock('@/api/analytics', async () => {
  const actual = await vi.importActual<typeof import('@/api/analytics')>('@/api/analytics')
  return {
    ...actual,
    fetchReadiness: () => mockFetchReadiness(),
    fetchReadinessPreferences: () => mockFetchReadinessPreferences(),
    putReadinessPreferences: (body: ReadinessPreferences) => mockPutReadinessPreferences(body),
  }
})

// ── Fixtures ──────────────────────────────────────────────────────────────────

// ReadinessSource covers device/pipeline sources on ReadinessFactor rows.
// ReadinessPathPreference ('manufacturer' | 'skadia' | null) is a separate domain
// used only in ReadinessPreferences.preferred_source — not valid as a factor source.

const FACTOR_GARMIN: ReadinessFactor = {
  key: 'hrv_status',
  value: 75,
  raw_value: 52.3,
  raw_unit: 'ms',
  source: 'garmin',
  available_sources: ['garmin'],
  stale_hours: null,
  missing: false,
}

const FACTOR_SKADIA: ReadinessFactor = {
  key: 'sleep',
  value: 60,
  raw_value: null,
  raw_unit: null,
  source: 'skadia',
  available_sources: ['skadia'],
  stale_hours: null,
  missing: false,
}

// Factor with two available sources: currently using garmin (manufacturer-equivalent),
// but skadia data is also available. This is the critical edge case for the toggle test.
const FACTOR_MULTI_SOURCE: ReadinessFactor = {
  key: 'training_load',
  value: 50,
  raw_value: null,
  raw_unit: null,
  source: 'garmin',
  available_sources: ['garmin', 'skadia'],
  stale_hours: null,
  missing: false,
}

const BASE_RESPONSE: ReadinessResponse = {
  refused: false,
  refusal_reason: null,
  score: 72,
  level: 'good',
  factors: [FACTOR_GARMIN, FACTOR_SKADIA],
  missing_factors: [],
  computed_at: '2026-05-01T06:00:00Z',
  data_window_end: '2026-05-01T05:59:59Z',
  preferences_version: null,
  confidence: 'high',
  recommendation_code: 'good_high_normal',
  signal_modifier: null,
  degraded: null,
}

const REFUSED_RESPONSE: ReadinessResponse = {
  refused: true,
  refusal_reason: 'no_data',
  score: null,
  level: null,
  factors: [],
  missing_factors: [],
  computed_at: '2026-05-01T06:00:00Z',
  data_window_end: null,
  preferences_version: null,
  confidence: null,
  recommendation_code: null,
  signal_modifier: null,
  degraded: null,
}

const DEFAULT_PREFS: ReadinessPreferences = {
  preferred_source: null,
}

// ── Wrapper ───────────────────────────────────────────────────────────────────

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: false },
    },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return <I18nProvider><QueryClientProvider client={qc}>{children}</QueryClientProvider></I18nProvider>
  }
  return Wrapper
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('ReadinessCardCompact', () => {
  beforeEach(() => {
    mockFetchReadiness.mockReset()
    mockFetchReadinessPreferences.mockReset()
    mockFetchReadinessPreferences.mockResolvedValue(DEFAULT_PREFS)
  })

  // Test 1: Compact renders refused-state — single-line compact hint (no heavy amber banner in dashboard context)
  it('renders refused-state with compact inline hint text', async () => {
    mockFetchReadiness.mockResolvedValue(REFUSED_RESPONSE) // refusal_reason: 'no_data'
    render(<ReadinessCardCompact />, { wrapper: makeWrapper() })

    await waitFor(() => {
      // Compact refused shows a short inline text, not the full amber refusal-banner.
      expect(screen.getByTestId('readiness-compact')).toBeInTheDocument()
    })
    // REFUSAL_TEXTS_COMPACT['no_data'] = 'Keine Daten'
    expect(screen.getByTestId('readiness-compact')).toHaveTextContent('Keine Daten')
    // Full amber refusal banner must NOT appear in compact/dashboard context.
    expect(screen.queryByTestId('refusal-banner')).not.toBeInTheDocument()
  })
})

describe('ReadinessCardCompact — error state', () => {
  beforeEach(() => {
    mockFetchReadiness.mockReset()
    mockFetchReadinessPreferences.mockReset()
    mockFetchReadinessPreferences.mockResolvedValue(DEFAULT_PREFS)
  })

  it('renders ghost ring on isError', async () => {
    mockFetchReadiness.mockRejectedValue(new Error('Network error'))
    render(<ReadinessCardCompact />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('readiness-compact-empty')).toBeInTheDocument()
    })
    expect(screen.getByTestId('readiness-compact-empty')).toHaveTextContent('Nicht verfügbar')
    // Populated card must not appear during error
    expect(screen.queryByTestId('readiness-compact')).not.toBeInTheDocument()
  })
})

describe('ReadinessCardFull — error state', () => {
  beforeEach(() => {
    mockFetchReadiness.mockReset()
    mockFetchReadinessPreferences.mockReset()
    mockFetchReadinessPreferences.mockResolvedValue(DEFAULT_PREFS)
  })

  it('renders ghost ring on isError with retry hint', async () => {
    mockFetchReadiness.mockRejectedValue(new Error('Network error'))
    render(<ReadinessCardFull />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('readiness-full-empty')).toBeInTheDocument()
    })
    expect(screen.getByTestId('readiness-full-empty')).toHaveTextContent(
      'Bereitschaftswert konnte nicht geladen werden.',
    )
    expect(screen.getByTestId('readiness-full-empty')).toHaveTextContent('Seite neu laden')
    // Populated card must not appear during error
    expect(screen.queryByTestId('readiness-full')).not.toBeInTheDocument()
  })
})

describe('ReadinessCardFull', () => {
  beforeEach(() => {
    mockFetchReadiness.mockReset()
    mockFetchReadinessPreferences.mockReset()
    mockPutReadinessPreferences.mockReset()
    mockFetchReadinessPreferences.mockResolvedValue(DEFAULT_PREFS)
  })

  // Test 2: Full renders factors with correct source-badges
  it('renders factor rows with correct source-badges', async () => {
    mockFetchReadiness.mockResolvedValue(BASE_RESPONSE)
    render(<ReadinessCardFull />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('factor-row-hrv_status')).toBeInTheDocument()
    })

    // Garmin badge
    expect(screen.getByTestId('source-badge-hrv_status')).toHaveTextContent('Garmin')
    // Skadia badge
    expect(screen.getByTestId('source-badge-sleep')).toHaveTextContent('Skadia')
  })

  // Test 3: Full hides Source-Toggle when all factors have available_sources.length<=1
  it('hides Source-Toggle when no factor has >=2 available_sources', async () => {
    mockFetchReadiness.mockResolvedValue(BASE_RESPONSE) // both factors have length 1
    render(<ReadinessCardFull />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('readiness-full')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('source-toggle')).not.toBeInTheDocument()
  })

  // Test 4: Full shows Source-Toggle when at least one factor has available_sources.length>=2
  // Critical edge case: currently chosen source is 'garmin' (manufacturer-equivalent),
  // but 'skadia' is also available — toggle must still appear because
  // available_sources.length >= 2. Using factor.source alone would hide the toggle.
  it('shows Source-Toggle when at least one factor has >=2 available_sources (even if active source is garmin/manufacturer-only)', async () => {
    const responseWithMultiSource: ReadinessResponse = {
      ...BASE_RESPONSE,
      factors: [
        FACTOR_GARMIN,       // available_sources: ['garmin'] — single source
        FACTOR_MULTI_SOURCE, // available_sources: ['garmin', 'skadia'] — multi, active=garmin
      ],
    }
    mockFetchReadiness.mockResolvedValue(responseWithMultiSource)
    render(<ReadinessCardFull />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('source-toggle')).toBeInTheDocument()
    })
  })

  // Test 5: Recommendation text rendered via recommendationText
  it('renders recommendation text for good_high_normal', async () => {
    mockFetchReadiness.mockResolvedValue(BASE_RESPONSE)
    render(<ReadinessCardFull />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('recommendation-text')).toBeInTheDocument()
    })
    // recommendationText('good_high_normal') returns text containing 'Planmäßige'
    expect(screen.getByTestId('recommendation-text')).toHaveTextContent('Planmäßige')
  })

  // Test 6 (R2-M12): ScaleBar renders with correct aria-progressbar attributes
  it('renders ScaleBar with correct aria-progressbar attributes when score present', async () => {
    mockFetchReadiness.mockResolvedValue(BASE_RESPONSE) // score: 72, level: 'good'
    render(<ReadinessCardFull />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('readiness-full')).toBeInTheDocument()
    })

    const bar = screen.getByRole('progressbar')
    expect(bar).toBeInTheDocument()
    expect(bar).toHaveAttribute('aria-valuemin', '0')
    expect(bar).toHaveAttribute('aria-valuemax', '100')
    expect(bar).toHaveAttribute('aria-valuenow', '72')
    expect(bar).toHaveAttribute('aria-label', 'Bereitschaftswert 72 von 100')
  })

  // Test 7 (R2-M12): MissingFactorsBanner renders per-factor missing_reason labels
  it('renders MissingFactorsBanner per-factor reasons (no_data, no_data_today, stale, no_baseline)', async () => {
    const responseWithMissing: ReadinessResponse = {
      ...BASE_RESPONSE,
      score: null,
      level: null,
      factors: [],
      missing_factors: [
        { key: 'hrv_status',    reason: 'no_data',       last_seen_at: null },
        { key: 'sleep',         reason: 'no_data_today',  last_seen_at: null },
        { key: 'training_load', reason: 'stale',          last_seen_at: '2026-04-28T08:00:00Z' },
        { key: 'dfa_alpha1',    reason: 'no_baseline',    last_seen_at: null },
      ],
    }
    mockFetchReadiness.mockResolvedValue(responseWithMissing)
    render(<ReadinessCardFull />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('missing-factors-banner')).toBeInTheDocument()
    })

    const banner = screen.getByTestId('missing-factors-banner')
    expect(banner).toHaveTextContent('HRV-Status')
    expect(banner).toHaveTextContent('Noch keine Daten')
    expect(banner).toHaveTextContent('Schlaf')
    expect(banner).toHaveTextContent('Heute keine Messung')
    expect(banner).toHaveTextContent('Trainingslast')
    expect(banner).toHaveTextContent('Letzte Messung zu alt')
    expect(banner).toHaveTextContent('DFA-α1')
    expect(banner).toHaveTextContent('Basislinie noch nicht ausreichend')
  })

  it('uses generic labels for unknown readiness factor and reason codes', async () => {
    const responseWithUnknownCodes: ReadinessResponse = {
      ...BASE_RESPONSE,
      factors: [
        {
          key: 'mystery_factor',
          value: 42,
          raw_value: null,
          raw_unit: null,
          source: 'skadia',
          available_sources: ['skadia'],
          stale_hours: null,
          missing: false,
        },
      ],
      missing_factors: [
        { key: 'mystery_missing_factor', reason: 'backend_code_without_label', last_seen_at: null },
      ],
    }
    mockFetchReadiness.mockResolvedValue(responseWithUnknownCodes)
    render(<ReadinessCardFull />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('readiness-full')).toBeInTheDocument()
    })

    const card = screen.getByTestId('readiness-full')
    expect(card).toHaveTextContent('Unbekannter Faktor')
    expect(card).toHaveTextContent('Unbekannter Grund')
    expect(card).not.toHaveTextContent('mystery_factor')
    expect(card).not.toHaveTextContent('mystery_missing_factor')
    expect(card).not.toHaveTextContent('backend_code_without_label')
  })
})

// ── Level snapshot tests ─────────────────────────────────────────────────────

describe('ReadinessCardFull — level snapshots', () => {
  beforeEach(() => {
    mockFetchReadiness.mockReset()
    mockFetchReadinessPreferences.mockResolvedValue(DEFAULT_PREFS)
  })

  const SHARED_FACTORS: ReadinessFactor[] = [FACTOR_GARMIN, FACTOR_SKADIA]

  const LEVEL_SCORES = {
    critical: 15,
    low: 30,
    moderate: 50,
    good: 72,
    optimal: 92,
  } as const

  for (const level of ['critical', 'low', 'moderate', 'good', 'optimal'] as const) {
    it(`snapshot for level="${level}"`, async () => {
      const response: ReadinessResponse = {
        ...BASE_RESPONSE,
        level,
        score: LEVEL_SCORES[level],
        factors: SHARED_FACTORS,
        recommendation_code: `${level}_high_normal`,
      }
      mockFetchReadiness.mockResolvedValue(response)

      const { container } = render(<ReadinessCardFull />, { wrapper: makeWrapper() })

      await waitFor(() => {
        expect(screen.getByTestId('readiness-full')).toBeInTheDocument()
      })

      expect(container.firstChild).toMatchSnapshot()
    })
  }
})

// ── Detail-overlay snapshot ─────────────────────────────────────────────────

describe('ReadinessCardFull — detail overlay snapshot', () => {
  beforeEach(() => {
    mockFetchReadiness.mockReset()
    mockFetchReadinessPreferences.mockResolvedValue({ preferred_source: 'manufacturer' })
  })

  it('snapshot: fully-expanded detail with visible toggle, rhr_changepoint_recent, all 5 factors', async () => {
    const allFiveFactors: ReadinessFactor[] = [
      { key: 'hrv_status',      value: 75, raw_value: 52.3, raw_unit: 'ms', source: 'garmin',        available_sources: ['garmin', 'skadia'],        stale_hours: null, missing: false },
      { key: 'sleep',           value: 62, raw_value: null,  raw_unit: null, source: 'garmin_fit',    available_sources: ['garmin_fit', 'skadia'],    stale_hours: null, missing: false },
      { key: 'training_load',   value: 48, raw_value: null,  raw_unit: null, source: 'garmin',        available_sources: ['garmin'],                   stale_hours: null, missing: false },
      { key: 'dfa_alpha1',      value: 35, raw_value: 0.72,  raw_unit: null, source: 'skadia',       available_sources: ['skadia'],                  stale_hours: 3,    missing: false },
      { key: 'rhr_changepoint', value: 40, raw_value: null,  raw_unit: null, source: 'skadia',       available_sources: ['skadia'],                  stale_hours: null, missing: false },
    ]

    const detailResponse: ReadinessResponse = {
      refused: false,
      refusal_reason: null,
      score: 58,
      level: 'moderate',
      factors: allFiveFactors,
      missing_factors: [],
      computed_at: '2026-05-01T06:00:00Z',
      data_window_end: '2026-05-01T05:59:59Z',
      preferences_version: null,
      confidence: 'medium',
      recommendation_code: 'moderate_medium_rhr_changepoint_recent',
      signal_modifier: 'rhr_changepoint_recent',
      degraded: null,
    }
    mockFetchReadiness.mockResolvedValue(detailResponse)

    const { container } = render(<ReadinessCardFull />, { wrapper: makeWrapper() })

    await waitFor(() => {
      expect(screen.getByTestId('readiness-full')).toBeInTheDocument()
    })

    // Verify critical elements are present before snapshotting
    expect(screen.getByTestId('source-toggle')).toBeInTheDocument()
    expect(screen.getByTestId('signal-modifier-notice')).toBeInTheDocument()
    expect(screen.getAllByTestId(/^factor-row-/)).toHaveLength(5)

    expect(container.firstChild).toMatchSnapshot()
  })
})

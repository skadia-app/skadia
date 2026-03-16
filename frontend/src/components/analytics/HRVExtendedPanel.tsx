import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import { getCapabilities } from '@/api/routes'

// ── Types ─────────────────────────────────────────────────────────────────────

interface HRVExtendedResponse {
  // Time-domain metrics
  mean_rr?: number
  sdnn?: number
  rmssd?: number
  sdsd?: number
  nn50?: number
  pnn50?: number
  cvnn?: number
  cvsd?: number
  // Frequency-domain metrics
  lf?: number
  hf?: number
  lf_hf_ratio?: number
  lfnu?: number
  hfnu?: number
  vlf?: number
  total_power?: number
  // Nonlinear metrics
  sd1?: number
  sd2?: number
  sd1_sd2_ratio?: number
  dfa_alpha1?: number
  dfa_alpha2?: number
  sample_entropy?: number
  approximate_entropy?: number
  // All metrics (124)
  [key: string]: number | undefined
}

interface FeaturesResponse {
  features?: Record<string, number>
  [key: string]: unknown
}

// ── Curated metric groups ──────────────────────────────────────────────────────

const TIME_DOMAIN_KEYS: { key: string; label: string; unit: string }[] = [
  { key: 'mean_rr', label: 'Mean RR', unit: 'ms' },
  { key: 'sdnn', label: 'SDNN', unit: 'ms' },
  { key: 'rmssd', label: 'RMSSD', unit: 'ms' },
  { key: 'sdsd', label: 'SDSD', unit: 'ms' },
  { key: 'nn50', label: 'NN50', unit: '' },
  { key: 'pnn50', label: 'pNN50', unit: '%' },
  { key: 'cvnn', label: 'CVNN', unit: '' },
  { key: 'cvsd', label: 'CVSD', unit: '' },
]

const FREQUENCY_DOMAIN_KEYS: { key: string; label: string; unit: string }[] = [
  { key: 'vlf', label: 'VLF', unit: 'ms²' },
  { key: 'lf', label: 'LF', unit: 'ms²' },
  { key: 'hf', label: 'HF', unit: 'ms²' },
  { key: 'total_power', label: 'Gesamtleistung', unit: 'ms²' },
  { key: 'lf_hf_ratio', label: 'LF/HF', unit: '' },
  { key: 'lfnu', label: 'LFnu', unit: 'nu' },
  { key: 'hfnu', label: 'HFnu', unit: 'nu' },
]

const NONLINEAR_KEYS: { key: string; label: string; unit: string }[] = [
  { key: 'sd1', label: 'SD1 (Poincaré)', unit: 'ms' },
  { key: 'sd2', label: 'SD2 (Poincaré)', unit: 'ms' },
  { key: 'sd1_sd2_ratio', label: 'SD1/SD2', unit: '' },
  { key: 'dfa_alpha1', label: 'DFA α1', unit: '' },
  { key: 'dfa_alpha2', label: 'DFA α2', unit: '' },
  { key: 'sample_entropy', label: 'Sample Entropy', unit: '' },
  { key: 'approximate_entropy', label: 'Approx. Entropy', unit: '' },
]

// ── Sub-components ────────────────────────────────────────────────────────────

type Tab = 'time' | 'freq' | 'nonlinear'

const TAB_LABELS: { id: Tab; label: string }[] = [
  { id: 'time', label: 'Zeit' },
  { id: 'freq', label: 'Frequenz' },
  { id: 'nonlinear', label: 'Nichtlinear' },
]

function MetricCard({ label, value, unit }: { label: string; value: number | undefined; unit: string }) {
  if (value == null) return null
  const formatted = Number.isInteger(value) ? value.toString() : value.toFixed(2)
  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/60 px-3 py-2">
      <p className="text-xs text-neutral-500 dark:text-neutral-400 truncate">{label}</p>
      <p className="text-sm font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums mt-0.5">
        {formatted}
        {unit && <span className="text-xs font-normal text-neutral-400 ml-1">{unit}</span>}
      </p>
    </div>
  )
}

function MetricGrid({
  keys,
  data,
}: {
  keys: { key: string; label: string; unit: string }[]
  data: HRVExtendedResponse
}) {
  const available = keys.filter(k => data[k.key] != null)
  if (available.length === 0) {
    return (
      <p className="text-sm text-neutral-400 text-center py-4">
        Keine Daten für diesen Bereich verfügbar
      </p>
    )
  }
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2">
      {available.map(k => (
        <MetricCard key={k.key} label={k.label} value={data[k.key]} unit={k.unit} />
      ))}
    </div>
  )
}

// ── All-metrics collapsible table ─────────────────────────────────────────────

function AllMetricsTable({ data }: { data: HRVExtendedResponse }) {
  const [open, setOpen] = useState(false)
  const entries = Object.entries(data).filter(([, v]) => typeof v === 'number')
  if (entries.length === 0) return null

  return (
    <div className="mt-4">
      <button
        onClick={() => setOpen(o => !o)}
        className="flex items-center gap-1.5 text-xs text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 transition-colors"
      >
        <svg
          className={`w-3.5 h-3.5 transition-transform ${open ? 'rotate-90' : ''}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        Alle {entries.length} Metriken anzeigen
      </button>
      {open && (
        <div className="mt-2 rounded-lg border border-neutral-200 dark:border-neutral-700 overflow-hidden">
          <table className="w-full text-xs">
            <thead className="bg-neutral-100 dark:bg-neutral-800">
              <tr>
                <th className="text-left px-3 py-2 text-neutral-600 dark:text-neutral-300 font-medium">Metrik</th>
                <th className="text-right px-3 py-2 text-neutral-600 dark:text-neutral-300 font-medium">Wert</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {entries.map(([key, value]) => (
                <tr key={key} className="hover:bg-neutral-50 dark:hover:bg-neutral-800/60">
                  <td className="px-3 py-1.5 text-neutral-600 dark:text-neutral-300 font-mono">{key}</td>
                  <td className="px-3 py-1.5 text-right tabular-nums text-neutral-900 dark:text-neutral-100">
                    {typeof value === 'number'
                      ? Number.isInteger(value) ? value : (value as number).toFixed(4)
                      : String(value)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

interface Props {
  activityId?: string
}

export default function HRVExtendedPanel({ activityId }: Props) {
  const [activeTab, setActiveTab] = useState<Tab>('time')

  const { data: capabilities } = useQuery({
    queryKey: ['capabilities'],
    queryFn: getCapabilities,
    staleTime: 30_000,
  })
  const mlAvailable = capabilities?.ml_service !== false

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'hrv-extended', activityId],
    queryFn: async () => {
      if (!activityId) return null
      try {
        return await apiClient<HRVExtendedResponse>(`/analytics/hrv-extended?activity_id=${activityId}`)
      } catch {
        // Fall back to features endpoint
        try {
          const r = await apiClient<FeaturesResponse>(`/analytics/features?activity_id=${activityId}`)
          return (r?.features ?? r) as HRVExtendedResponse
        } catch {
          return null
        }
      }
    },
    enabled: !!activityId && mlAvailable,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-5 space-y-4">
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200">
        Erweiterte HRV-Analyse
      </h3>

      {!mlAvailable ? (
        <p className="text-sm text-neutral-400 text-center py-4">
          Dieses Feature benötigt den ML-Service.
        </p>
      ) : !activityId ? (
        <p className="text-sm text-neutral-400 text-center py-4">Aktivität auswählen</p>
      ) : isLoading ? (
        <div className="space-y-2 animate-pulse">
          <div className="h-8 bg-neutral-100 dark:bg-neutral-800 rounded" />
          <div className="grid grid-cols-3 gap-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="h-14 bg-neutral-100 dark:bg-neutral-800 rounded" />
            ))}
          </div>
        </div>
      ) : isError || !data ? (
        <p className="text-sm text-neutral-400 text-center py-4">
          Keine erweiterten HRV-Daten verfügbar
        </p>
      ) : (
        <>
          {/* Tab bar */}
          <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs w-fit">
            {TAB_LABELS.map(({ id, label }) => (
              <button
                key={id}
                onClick={() => setActiveTab(id)}
                className={
                  activeTab === id
                    ? 'px-4 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                    : 'px-4 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
                }
              >
                {label}
              </button>
            ))}
          </div>

          {/* Tab content */}
          {activeTab === 'time' && <MetricGrid keys={TIME_DOMAIN_KEYS} data={data} />}
          {activeTab === 'freq' && <MetricGrid keys={FREQUENCY_DOMAIN_KEYS} data={data} />}
          {activeTab === 'nonlinear' && <MetricGrid keys={NONLINEAR_KEYS} data={data} />}

          {/* All-metrics toggle */}
          <AllMetricsTable data={data} />
        </>
      )}
    </div>
  )
}

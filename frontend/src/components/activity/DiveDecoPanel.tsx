import { useQuery } from '@tanstack/react-query'
import { fetchDiveInfo } from '@/api/activities'
import type { StreamPoint } from '@/api/types'

// ── Labels ─────────────────────────────────────────────────────────────────────

const LABELS = {
  equipment: 'Ausrüstung & Einstellungen',
  decoModel: 'Deko-Modell',
  gfLabel: 'GF Low/High',
  waterType: 'Wassertyp',
  waterDensity: 'Wasserdichte',
  po2Limits: 'PO₂-Limits (Warn / Krit / Deko)',
  ccrSetpoints: 'CCR-Setpoints',
  ccrLow: 'Low',
  ccrHigh: 'High',
  gasMixes: 'Gas-Mischungen',
  gasIndex: 'Gas',
  oxygen: 'O₂%',
  helium: 'He%',
  status: 'Status',
  mode: 'Modus',
  ndlGauge: 'Minimaler NDL',
  ndlDeko: 'DEKO',
  ndlMinutes: 'min',
  ndlUnit: 'NDL',
  decoCharts: 'Deko-Zeitreihen',
  ndlLine: 'NDL',
  cnsLine: 'CNS%',
  n2Line: 'N₂-Loading%',
  decoStopLine: 'Deko-Stop (m)',
  loading: 'Lade Dive-Info…',
  noData: 'Keine Deko-Daten verfügbar',
}

// ── Types ──────────────────────────────────────────────────────────────────────

interface DiveDecoPanelProps {
  activityId: string
  streams: StreamPoint[]
}

// ── Helper: last non-null value ────────────────────────────────────────────────

function lastNonNull(streams: StreamPoint[], key: keyof StreamPoint): number | null {
  for (let i = streams.length - 1; i >= 0; i--) {
    const v = streams[i][key]
    if (v != null) return v as number
  }
  return null
}

// ── Helper: water type display ────────────────────────────────────────────────

function waterTypeLabel(raw: string): string {
  switch (raw.toLowerCase()) {
    case 'fresh': return 'Süßwasser'
    case 'salt': return 'Salzwasser'
    case 'en13319': return 'EN13319'
    default: return raw
  }
}

// ── Helper: gas mode display ──────────────────────────────────────────────────

function gasModeLabel(mode: string): string {
  switch (mode.toLowerCase()) {
    case 'oc_gas': return 'OC'
    case 'cc_diluent': return 'Diluent (CCR)'
    case 'cc_o2': return 'O₂ (CCR)'
    default: return mode
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

export function DiveDecoPanel({ activityId, streams }: DiveDecoPanelProps) {
  const { data: diveInfo, isLoading } = useQuery({
    queryKey: ['dive-info', activityId],
    queryFn: () => fetchDiveInfo(activityId),
    staleTime: 5 * 60 * 1000,
  })

  // ── NDL Gauge value — show MINIMUM NDL during the dive (most critical moment) ──

  let minNdlSec: number | null = null
  for (const s of streams) {
    if (s.ndl_time_s != null) {
      if (minNdlSec === null || s.ndl_time_s < minNdlSec) minNdlSec = s.ndl_time_s
    }
  }
  const ndlMin = minNdlSec != null ? Math.round(minNdlSec / 60) : null

  // Deco chart data is now rendered by DecoChartPanel in ActivityCharts.tsx
  const hasAnyDecoStreams = streams.some(s => s.ndl_time_s != null || s.cns_load_pct != null)

  // ── Render ─────────────────────────────────────────────────────────────────

  if (isLoading) {
    return (
      <div className="text-sm text-neutral-500 dark:text-neutral-400 py-4 text-center">
        {LABELS.loading}
      </div>
    )
  }

  return (
    <div className="space-y-4">

      {/* A) Equipment Info */}
      {diveInfo?.settings && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4">
          <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">
            {LABELS.equipment}
          </h3>
          <dl className="grid grid-cols-2 sm:grid-cols-3 gap-x-6 gap-y-2 text-sm">
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{LABELS.decoModel}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {diveInfo.settings.deco_model || '—'}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{LABELS.gfLabel}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {diveInfo.settings.gf_low?.toString() ?? '–'}/{diveInfo.settings.gf_high?.toString() ?? '–'}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{LABELS.waterType}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {diveInfo.settings.water_type ? waterTypeLabel(diveInfo.settings.water_type) : '–'}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{LABELS.waterDensity}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {diveInfo.settings.water_density?.toFixed(3) ?? '–'} kg/l
              </dd>
            </div>
            <div>
              <dt className="text-xs text-neutral-500 dark:text-neutral-400">{LABELS.po2Limits}</dt>
              <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                {diveInfo.settings.po2_warn?.toFixed(1) ?? '–'} / {diveInfo.settings.po2_critical?.toFixed(1) ?? '–'} / {diveInfo.settings.po2_deco?.toFixed(1) ?? '–'} bar
              </dd>
            </div>
            {((diveInfo.settings.ccr_low_setpoint ?? 0) > 0 || (diveInfo.settings.ccr_high_setpoint ?? 0) > 0) && (
              <div>
                <dt className="text-xs text-neutral-500 dark:text-neutral-400">{LABELS.ccrSetpoints}</dt>
                <dd className="font-medium text-neutral-900 dark:text-neutral-100">
                  {LABELS.ccrLow}: {diveInfo.settings.ccr_low_setpoint?.toFixed(2) ?? '–'} bar
                  {' @ '}{diveInfo.settings.ccr_low_setpoint_depth_m ?? '–'} m
                  {' · '}
                  {LABELS.ccrHigh}: {diveInfo.settings.ccr_high_setpoint?.toFixed(2) ?? '–'} bar
                  {' @ '}{diveInfo.settings.ccr_high_setpoint_depth_m ?? '–'} m
                </dd>
              </div>
            )}
          </dl>
        </div>
      )}

      {/* B) Gas Mix Table */}
      {diveInfo && diveInfo.gases.length > 0 && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4">
          <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">
            {LABELS.gasMixes}
          </h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-200 dark:border-neutral-800">
                  <th className="text-left text-xs text-neutral-500 dark:text-neutral-400 pb-2 pr-4">{LABELS.gasIndex}</th>
                  <th className="text-right text-xs text-neutral-500 dark:text-neutral-400 pb-2 pr-4">{LABELS.oxygen}</th>
                  <th className="text-right text-xs text-neutral-500 dark:text-neutral-400 pb-2 pr-4">{LABELS.helium}</th>
                  <th className="text-left text-xs text-neutral-500 dark:text-neutral-400 pb-2 pr-4">{LABELS.status}</th>
                  <th className="text-left text-xs text-neutral-500 dark:text-neutral-400 pb-2">{LABELS.mode}</th>
                </tr>
              </thead>
              <tbody>
                {diveInfo.gases.map(gas => {
                  const isTrimix = gas.helium_pct > 0
                  const isDiluent = gas.mode.toLowerCase().includes('diluent')
                  const isBailout = gas.status.toLowerCase() === 'bailout'
                  const rowClass = isDiluent
                    ? 'text-blue-700 dark:text-blue-400'
                    : isBailout
                      ? 'text-amber-700 dark:text-amber-400'
                      : 'text-neutral-900 dark:text-neutral-100'
                  const gasLabel = isTrimix
                    ? `Trimix ${gas.oxygen_pct}/${gas.helium_pct}`
                    : gas.oxygen_pct === 21
                      ? 'Air'
                      : `EAN${gas.oxygen_pct}`
                  return (
                    <tr key={gas.gas_index} className="border-b border-neutral-100 dark:border-neutral-900 last:border-0">
                      <td className={`py-1.5 pr-4 font-medium ${rowClass}`}>
                        {gasLabel}
                      </td>
                      <td className={`py-1.5 pr-4 text-right tabular-nums ${rowClass}`}>
                        {gas.oxygen_pct}%
                      </td>
                      <td className={`py-1.5 pr-4 text-right tabular-nums ${rowClass}`}>
                        {gas.helium_pct > 0 ? `${gas.helium_pct}%` : '—'}
                      </td>
                      <td className={`py-1.5 pr-4 capitalize ${rowClass}`}>
                        {gas.status}
                      </td>
                      <td className={`py-1.5 capitalize ${rowClass}`}>
                        {gasModeLabel(gas.mode)}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* C) NDL Gauge */}
      {ndlMin !== null && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 flex items-center gap-6">
          <div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{LABELS.ndlGauge}</div>
            {ndlMin <= 0 ? (
              <div className="text-2xl font-bold text-red-600 dark:text-red-400">{LABELS.ndlDeko}</div>
            ) : (
              <div className={`text-3xl font-bold tabular-nums ${
                ndlMin > 10
                  ? 'text-green-600 dark:text-green-400'
                  : ndlMin >= 5
                    ? 'text-amber-600 dark:text-amber-400'
                    : 'text-red-600 dark:text-red-400'
              }`}>
                {ndlMin}
                <span className="text-base font-normal text-neutral-500 dark:text-neutral-400 ml-1">{LABELS.ndlMinutes}</span>
              </div>
            )}
          </div>
          {/* Visual color bar */}
          <div className="flex-1 h-3 rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden">
            {ndlMin > 0 && (
              <div
                className={`h-full rounded-full transition-all ${
                  ndlMin > 10 ? 'bg-green-500' : ndlMin >= 5 ? 'bg-amber-500' : 'bg-red-500'
                }`}
                style={{ width: `${Math.min(100, (ndlMin / 30) * 100)}%` }}
              />
            )}
            {ndlMin <= 0 && <div className="h-full bg-red-500 w-full" />}
          </div>
        </div>
      )}

      {!hasAnyDecoStreams && !diveInfo?.settings && (
        <div className="text-sm text-neutral-500 dark:text-neutral-400 py-4 text-center">
          {LABELS.noData}
        </div>
      )}
    </div>
  )
}

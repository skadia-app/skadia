// StreamChartContainer — metric tab navigation + dynamic metric detection.
// Renders a StreamChart for the active metric across all visible sources.

import { useState, useMemo } from 'react'
import StreamChart, { type StreamSeries } from './StreamChart'
import { useSources } from './SourcesContext'
import { useStreamData, type PreparedStreamData } from '@/hooks/useStreamData'
import { formatPace } from '@/lib/sportFieldLabels'

// ── Metric definitions ────────────────────────────────────────────────────────

type MetricKey =
  | 'hr' | 'power' | 'speed' | 'altitude' | 'cadence' | 'temperature'
  | 'gct' | 'vertOsc' | 'stanceTime' | 'stepLength' | 'lrBalance'

interface MetricDef {
  key: MetricKey
  label: string
  unit: string
  dataKey: keyof PreparedStreamData
  priority: 1 | 2
}

// Priority 1 = Core, Priority 2 = Running-specific
const METRIC_DEFS: MetricDef[] = [
  { key: 'hr',          label: 'Herzfrequenz',      unit: 'bpm',    dataKey: 'hrData',                  priority: 1 },
  { key: 'power',       label: 'Leistung',           unit: 'W',      dataKey: 'powerData',               priority: 1 },
  { key: 'speed',       label: 'Speed',              unit: 'km/h',   dataKey: 'speedData',               priority: 1 },
  { key: 'altitude',    label: 'Höhe',               unit: 'm',      dataKey: 'altitudeData',            priority: 1 },
  { key: 'cadence',     label: 'Kadenz',             unit: 'rpm',    dataKey: 'cadenceData',             priority: 1 },
  { key: 'temperature', label: 'Temperatur',         unit: '°C',     dataKey: 'temperatureData',         priority: 1 },
  { key: 'gct',         label: 'Bodenkontakt',       unit: 'ms',     dataKey: 'groundContactTimeData',   priority: 2 },
  { key: 'vertOsc',     label: 'Vert. Bewegung',     unit: 'mm',     dataKey: 'verticalOscillationData', priority: 2 },
  { key: 'stanceTime',  label: 'Standzeit',          unit: '%',      dataKey: 'stanceTimePercentData',   priority: 2 },
  { key: 'stepLength',  label: 'Schrittlänge',       unit: 'mm',     dataKey: 'stepLengthData',          priority: 2 },
  { key: 'lrBalance',   label: 'L/R Balance',        unit: '%',      dataKey: 'leftRightBalanceData',    priority: 2 },
]

// ── Per-source data hook ──────────────────────────────────────────────────────

function SourceStreamData({
  sourceId,
  sportType,
  streams,
  onData,
}: {
  sourceId: string
  sportType: string
  streams: import('@/api/types').StreamPoint[] | undefined
  onData: (id: string, data: PreparedStreamData) => void
}) {
  const data = useStreamData(streams, sportType)
  // Notify parent — use a stable callback pattern
  onData(sourceId, data)
  return null
}

// ── Main component ────────────────────────────────────────────────────────────

interface StreamChartContainerProps {
  /** Override chart height in px */
  chartHeight?: number
}

export default function StreamChartContainer({ chartHeight = 140 }: StreamChartContainerProps) {
  const { visibleSources } = useSources()
  const [selectedKey, setSelectedKey] = useState<MetricKey | null>(null)

  // Collect prepared data for all sources (using a ref-based accumulator pattern)
  const preparedMap = useMemo(() => new Map<string, PreparedStreamData>(), [])

  // Build series for a given metric key and prepared data map
  function buildSeries(key: MetricKey, pMap: Map<string, PreparedStreamData>): StreamSeries[] {
    const def = METRIC_DEFS.find(m => m.key === key)
    if (!def) return []
    return visibleSources
      .map(src => {
        const pd = pMap.get(src.id)
        if (!pd) return null
        const data = pd[def.dataKey] as [number, number][]
        if (data.length === 0) return null
        return { label: src.label, color: src.color, data }
      })
      .filter((s): s is StreamSeries => s !== null)
  }

  // We need a component that renders per-source to call hooks — but here we
  // take a simpler approach: build data inline using useMemo per-source would
  // violate hooks rules if sources change length. Instead we use a render-based
  // accumulation: render null children that populate preparedMap.
  //
  // The actual approach: use a flat useMemo over all source streams (safe
  // because array length is stable for a given set of sources).

  return (
    <StreamChartContainerInner
      chartHeight={chartHeight}
      selectedKey={selectedKey}
      setSelectedKey={setSelectedKey}
      preparedMap={preparedMap}
    />
  )
}

// ── Inner implementation ──────────────────────────────────────────────────────
// Split to allow hooks to be called with stable source array.

function StreamChartContainerInner({
  chartHeight,
  selectedKey,
  setSelectedKey,
  preparedMap: _preparedMap,
}: {
  chartHeight: number
  selectedKey: MetricKey | null
  setSelectedKey: (k: MetricKey | null) => void
  preparedMap: Map<string, PreparedStreamData>
}) {
  const { visibleSources } = useSources()

  // Prepare data for each source (one hook call per source index — sources array
  // length must be stable for this to be valid. In practice it is stable enough
  // for comparison contexts where sources are set up front).
  // We pad to a max of 8 to keep hooks count stable.
  const MAX_SOURCES = 8
  const padded = [...visibleSources, ...Array(Math.max(0, MAX_SOURCES - visibleSources.length)).fill(null)]

  // eslint-disable-next-line react-hooks/rules-of-hooks
  const d0 = useStreamData(padded[0]?.streams, padded[0]?.activity?.sport_type ?? 'generic')
  // eslint-disable-next-line react-hooks/rules-of-hooks
  const d1 = useStreamData(padded[1]?.streams, padded[1]?.activity?.sport_type ?? 'generic')
  // eslint-disable-next-line react-hooks/rules-of-hooks
  const d2 = useStreamData(padded[2]?.streams, padded[2]?.activity?.sport_type ?? 'generic')
  // eslint-disable-next-line react-hooks/rules-of-hooks
  const d3 = useStreamData(padded[3]?.streams, padded[3]?.activity?.sport_type ?? 'generic')
  // eslint-disable-next-line react-hooks/rules-of-hooks
  const d4 = useStreamData(padded[4]?.streams, padded[4]?.activity?.sport_type ?? 'generic')
  // eslint-disable-next-line react-hooks/rules-of-hooks
  const d5 = useStreamData(padded[5]?.streams, padded[5]?.activity?.sport_type ?? 'generic')
  // eslint-disable-next-line react-hooks/rules-of-hooks
  const d6 = useStreamData(padded[6]?.streams, padded[6]?.activity?.sport_type ?? 'generic')
  // eslint-disable-next-line react-hooks/rules-of-hooks
  const d7 = useStreamData(padded[7]?.streams, padded[7]?.activity?.sport_type ?? 'generic')

  const allData = [d0, d1, d2, d3, d4, d5, d6, d7].slice(0, visibleSources.length)

  // Build a map of sourceId → PreparedStreamData
  const dataMap = useMemo(() => {
    const m = new Map<string, PreparedStreamData>()
    visibleSources.forEach((src, i) => { if (allData[i]) m.set(src.id, allData[i]) })
    return m
  // allData items are stable references from useMemo
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visibleSources, ...allData])

  // Determine which metrics have data across any visible source
  const available = useMemo(() => METRIC_DEFS.filter(def =>
    visibleSources.some(src => {
      const pd = dataMap.get(src.id)
      return pd && (pd[def.dataKey] as [number, number][]).length > 0
    })
  ), [visibleSources, dataMap])

  const activeKey = available.find(m => m.key === selectedKey)?.key ?? available[0]?.key ?? null
  const activeDef = METRIC_DEFS.find(m => m.key === activeKey) ?? null

  // Build series for active metric
  const series = useMemo<StreamSeries[]>(() => {
    if (!activeDef) return []
    return visibleSources
      .map(src => {
        const pd = dataMap.get(src.id)
        if (!pd) return null
        const data = pd[activeDef.dataKey] as [number, number][]
        if (data.length === 0) return null
        return { label: src.label, color: src.color, data }
      })
      .filter((s): s is StreamSeries => s !== null)
  }, [activeDef, visibleSources, dataMap])

  // GPS data for hover sync
  const sourcesGps = useMemo(() => visibleSources.map(src => ({
    color: src.color,
    gpsPoints: dataMap.get(src.id)?.gpsPoints ?? [],
  })), [visibleSources, dataMap])

  // Speed unit and pace formatter
  const isPaceMode = allData[0]?.isPaceMode ?? false
  const speedDef = activeDef?.key === 'speed'

  const effectiveUnit = useMemo(() => {
    if (!activeDef) return ''
    if (speedDef && isPaceMode) return 'min/km'
    return activeDef.unit
  }, [activeDef, speedDef, isPaceMode])

  const formatTooltipValue = useMemo(() => {
    if (!speedDef || !isPaceMode) return undefined
    return (v: number) => {
      // v is already in min/km (decimal) — convert back to ms for formatPace
      if (v <= 0) return '—'
      const speedMs = 1000 / (v * 60)
      return formatPace(speedMs)
    }
  }, [speedDef, isPaceMode])

  if (available.length === 0) {
    return (
      <div className="flex items-center justify-center h-32 text-neutral-400 text-xs rounded-xl border border-neutral-200 dark:border-neutral-800">
        Keine Zeitreihendaten verfügbar
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-2">
      {/* Metric tabs */}
      <div className="flex flex-wrap gap-1">
        {available.map(m => (
          <button
            key={m.key}
            onClick={() => setSelectedKey(m.key)}
            className={`px-2 py-0.5 rounded text-xs font-medium transition-colors cursor-pointer ${
              m.key === activeKey
                ? 'bg-neutral-800 dark:bg-neutral-200 text-white dark:text-neutral-900'
                : 'text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300'
            }`}
          >
            {m.label}
          </button>
        ))}
      </div>

      {/* Chart */}
      {activeDef && series.length > 0 && (
        <StreamChart
          series={series}
          unit={effectiveUnit}
          sourcesGps={sourcesGps}
          height={chartHeight}
          formatTooltipValue={formatTooltipValue}
        />
      )}
    </div>
  )
}

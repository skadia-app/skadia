import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import * as echarts from 'echarts/core'
import { PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { fetchInsightsClusters } from '@/api/analytics'
import type { ActivityCluster } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { getCapabilities } from '@/api/routes'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { useECharts } from '@/hooks/useECharts'
import { sportLabel } from '@/lib/sportTaxonomy'

echarts.use([PieChart, TooltipComponent, LegendComponent, CanvasRenderer])

const CLUSTER_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ef4444',
  '#06b6d4', '#84cc16', '#f97316', '#ec4899', '#6b7280',
]

type ClusterLabelKey = 'mountain' | 'recovery' | 'endurance' | 'interval' | 'race' | 'mixed'

const CLUSTER_LABEL_KEYS = new Set<string>(['mountain', 'recovery', 'endurance', 'interval', 'race', 'mixed'])

const LEGACY_CLUSTER_LABEL_KEYS: Record<string, ClusterLabelKey> = {
  Berg: 'mountain',
  Climb: 'mountain',
  'Montée': 'mountain',
  'Montana': 'mountain',
  'Montaña': 'mountain',
  Salita: 'mountain',
  Erholung: 'recovery',
  Recovery: 'recovery',
  'Récupération': 'recovery',
  Recuperacion: 'recovery',
  'Recuperación': 'recovery',
  Recupero: 'recovery',
  Ausdauer: 'endurance',
  Endurance: 'endurance',
  Resistencia: 'endurance',
  Resistenza: 'endurance',
  Intervall: 'interval',
  Interval: 'interval',
  Intervalle: 'interval',
  Intervalo: 'interval',
  Intervallo: 'interval',
  Wettkampf: 'race',
  Race: 'race',
  Course: 'race',
  Competicion: 'race',
  'Competición': 'race',
  Gara: 'race',
  Gemischt: 'mixed',
  Mixed: 'mixed',
  Mixte: 'mixed',
  Mixto: 'mixed',
  Misto: 'mixed',
}

export function localizedClusterLabel(cluster: Pick<ActivityCluster, 'label' | 'label_key'>, t: TFunction, fallback: string): string {
  const labelKey = cluster.label_key && CLUSTER_LABEL_KEYS.has(cluster.label_key)
    ? cluster.label_key
    : LEGACY_CLUSTER_LABEL_KEYS[cluster.label]
  return labelKey ? t(`analyticsHealthInsights.clusters.labels.${labelKey}`) : (cluster.label || fallback)
}

export function formatClusterDistance(
  meters: number,
  formatDistanceCompact: (meters: number) => string,
): string {
  return formatDistanceCompact(meters)
}

function ClusterInterpretation({ clusters }: { clusters: ActivityCluster[] }) {
  const { t } = useTranslation()
  if (clusters.length === 0) return null

  // Simple interpretation based on cluster sizes
  const sorted = [...clusters].sort((a, b) => b.size - a.size)
  const dominant = sorted[0]

  return (
    <div className="rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-200 dark:border-neutral-700 p-3">
      <p className="text-xs font-medium text-neutral-700 dark:text-neutral-200 mb-1">{t('analyticsHealthInsights.clusters.interpretationTitle')}</p>
      <p className="text-xs text-neutral-500 dark:text-neutral-400">
        {t('analyticsHealthInsights.clusters.dominantPrefix')} <strong className="text-neutral-700 dark:text-neutral-200">"{dominant.label}"</strong> {t('analyticsHealthInsights.clusters.dominantSuffix', { count: dominant.size })}
        {dominant.description && ` ${dominant.description}`}
      </p>
    </div>
  )
}

export default function TrainingClusters() {
  const { t, i18n } = useTranslation()
  const { toQueryString } = useAnalyticsFilters()
  const unitFormatters = useUnitFormatters()
  const queryParams = toQueryString()
  const { data: capabilities } = useQuery({
    queryKey: ['capabilities'],
    queryFn: getCapabilities,
    staleTime: 30_000,
  })
  const mlAvailable = capabilities?.ml_service === true

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'insights', 'clusters', queryParams],
    queryFn: async () => {
      try {
        return await fetchInsightsClusters(queryParams)
      } catch {
        return null
      }
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
    enabled: mlAvailable,
  })

  // ML service may return `count` instead of `size` and older cache entries may
  // contain localized labels without stable label_key. Normalize both for render.
  const clusters = useMemo(() => (
    Array.isArray(data) ? data.map(c => ({
      ...c,
      label: localizedClusterLabel(c, t, t('analyticsHealthInsights.clusters.fallbackName', { index: c.id + 1 })),
      size: c.size ?? (c as unknown as Record<string, number>).count ?? 0,
      avg_distance_m: c.avg_distance_m ?? 0,
      avg_hr: c.avg_hr ?? null,
      sport_types: c.sport_types ?? [],
      description: c.description ?? null,
    })) : []
  ), [data, t, i18n.language])

  const containerRef = useECharts(
    () => {
      if (clusters.length === 0) return { series: [] }

      return {
        backgroundColor: 'transparent',
        tooltip: {
          confine: true,
          trigger: 'item',
          backgroundColor: 'rgba(17,24,39,0.92)',
          borderColor: '#374151',
          textStyle: { color: '#f3f4f6', fontSize: 12 },
          formatter: (params: unknown) => {
            const p = params as { name: string; value: number; percent: number }
            return `<div style="font-size:12px;font-weight:600;margin-bottom:2px">${p.name}</div>
                    <div>${t('analyticsHealthInsights.clusters.tooltipActivities', { count: p.value, percent: p.percent.toFixed(1) })}</div>`
          },
        },
        legend: { show: false },
        series: [
          {
            type: 'pie',
            radius: ['40%', '72%'],
            center: ['50%', '50%'],
            data: clusters.map((c, i) => ({
              name: c.label ?? t('analyticsHealthInsights.clusters.fallbackName', { index: i + 1 }),
              value: c.size ?? (c as unknown as Record<string, number>).count ?? 0,
              itemStyle: { color: CLUSTER_COLORS[i % CLUSTER_COLORS.length] },
            })),
            label: {
              show: true,
              fontSize: 10,
              color: '#9ca3af',
              formatter: '{b}: {d}%',
            },
            labelLine: { lineStyle: { color: '#4b5563' } },
            emphasis: {
              itemStyle: { shadowBlur: 10, shadowOffsetX: 0, shadowColor: 'rgba(0,0,0,0.5)' },
            },
          },
        ],
      }
    },
    [clusters, t, i18n.language],
  )

  if (!mlAvailable) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200 mb-3">{t('analyticsHealthInsights.clusters.title')}</h3>
        <p className="text-sm text-neutral-400 text-center py-6">
          {t('analyticsHealthInsights.common.mlRequired')}
        </p>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 animate-pulse">
        <div className="h-4 w-40 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
        <div className="w-full h-[220px] bg-neutral-200 dark:bg-neutral-700 rounded-full mx-auto" style={{ maxWidth: 220 }} />
      </div>
    )
  }

  if (isError || !data || !Array.isArray(data) || data.length === 0) {
    const noCache = !isError && (data === null || (Array.isArray(data) && data.length === 0))
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200 mb-3">{t('analyticsHealthInsights.clusters.title')}</h3>
        <div className="flex flex-col items-center justify-center h-[180px] gap-2 text-neutral-400 text-sm">
          <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
              d="M11 3.055A9.001 9.001 0 1020.945 13H11V3.055z" />
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
              d="M20.488 9H15V3.512A9.025 9.025 0 0120.488 9z" />
          </svg>
          <span>{noCache ? t('analyticsHealthInsights.common.notCalculated') : t('analyticsHealthInsights.clusters.noData')}</span>
          <span className="text-xs text-center">{noCache ? t('analyticsHealthInsights.common.nextRefreshHint') : t('analyticsHealthInsights.clusters.minActivitiesHint')}</span>
        </div>
      </div>
    )
  }

  const total = clusters.reduce((sum, c) => sum + c.size, 0)

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200">{t('analyticsHealthInsights.clusters.title')}</h3>

      {/* Donut chart */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 overflow-hidden bg-white dark:bg-neutral-900">
        <div ref={containerRef} className="w-full h-[220px]" />
      </div>

      {/* Legend with details */}
      <div className="space-y-2">
        {clusters.map((cluster, i) => {
          const pct = total > 0 ? ((cluster.size / total) * 100).toFixed(1) : '0'
          return (
            <div key={cluster.id} className="flex items-start gap-2 text-xs">
              <span
                className="mt-0.5 flex-shrink-0 w-2.5 h-2.5 rounded-full"
                style={{ backgroundColor: CLUSTER_COLORS[i % CLUSTER_COLORS.length] }}
              />
              <div className="flex-1 min-w-0">
                <div className="flex items-center justify-between gap-1">
                  <span className="font-medium text-neutral-700 dark:text-neutral-200 truncate">{cluster.label}</span>
                  <span className="flex-shrink-0 text-neutral-500">{pct}%</span>
                </div>
                <div className="flex items-center gap-3 text-neutral-400 mt-0.5 flex-wrap">
                  <span>{t('analyticsHealthInsights.clusters.activityCount', { count: cluster.size })}</span>
                  {cluster.avg_distance_m > 0 && (
                    <span>&#8960; {formatClusterDistance(cluster.avg_distance_m, unitFormatters.formatDistanceCompact)}</span>
                  )}
                  {cluster.avg_hr && <span>&#8960; {cluster.avg_hr.toFixed(0)} bpm</span>}
                  {cluster.sport_types.length > 0 && (
                    <span className="truncate">{cluster.sport_types.slice(0, 2).map(s => sportLabel(s)).join(', ')}</span>
                  )}
                </div>
              </div>
            </div>
          )
        })}
      </div>

      <ClusterInterpretation clusters={clusters} />
    </div>
  )
}

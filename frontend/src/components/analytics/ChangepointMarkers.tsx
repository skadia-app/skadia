import { useEffect, useState } from 'react'
import { apiClient } from '@/api/client'

export interface Changepoint {
  date: string
  magnitude: number
  direction: 'up' | 'down'
  before_mean: number
  after_mean: number
}

interface Props {
  metric: 'hrv_rmssd' | 'resting_hr' | 'ctl'
  days?: number
}

export function useChangepoints(metric: Props['metric'], days = 180): Changepoint[] {
  const [data, setData] = useState<Changepoint[]>([])
  useEffect(() => {
    apiClient<{ changepoints: Changepoint[] }>(`/analytics/changepoints?metric=${metric}&days=${days}`)
      .then(r => setData(r.changepoints))
      .catch(() => {})
  }, [metric, days])
  return data
}

/**
 * Returns ECharts markLine data for changepoints.
 * Usage: add to an ECharts series as markLine.data
 */
export function changepointsToMarkLines(changepoints: Changepoint[]): object[] {
  return changepoints.map(cp => ({
    xAxis: cp.date,
    lineStyle: {
      color: cp.direction === 'up' ? '#22c55e' : '#ef4444',
      type: 'dashed',
      width: 1.5,
    },
    label: {
      show: true,
      formatter: `${cp.direction === 'up' ? '↑' : '↓'}${Math.abs(cp.magnitude).toFixed(1)}`,
      fontSize: 10,
    },
  }))
}

import { useMemo } from 'react'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useSearchParams } from 'react-router-dom'
import type { StreamPoint } from '@/api/types'

export const CHART_TAB = {
  alles: 'alles',
  uebersicht: 'uebersicht',
  leistung: 'leistung',
  dynamik: 'dynamik',
  pedal: 'pedal',
  ebike: 'ebike',
  tauchgang: 'tauchgang',
  sensoren: 'sensoren',
  mtb: 'mtb',
  ausdauer: 'ausdauer',
  barometer: 'barometer',
} as const

export type ChartTabValue = typeof CHART_TAB[keyof typeof CHART_TAB]

interface TabDef {
  value: string
  label: string
  visible: boolean
}

interface Props {
  streams: StreamPoint[]
  children: (activeTab: string) => React.ReactNode
}

export function ChartTabs({ streams, children }: Props) {
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = searchParams.get('tab') ?? CHART_TAB.alles

  const visibleTabs = useMemo(() => {
    const tabs: TabDef[] = [
      { value: CHART_TAB.alles, label: 'Alles', visible: true },
      { value: CHART_TAB.uebersicht, label: 'Übersicht', visible: true },
      { value: CHART_TAB.leistung, label: 'Leistung', visible: streams.some(s => s.power_w != null) },
      { value: CHART_TAB.dynamik, label: 'Dynamik', visible: streams.some(s => s.ground_contact_time_ms != null) },
      { value: CHART_TAB.pedal, label: 'Pedal-Analytik', visible: streams.some(s => s.left_torque_effectiveness_pct != null) },
      { value: CHART_TAB.ebike, label: 'E-Bike', visible: streams.some(s => s.motor_power_w != null) },
      { value: CHART_TAB.tauchgang, label: 'Tauchgang', visible: streams.some(s => s.depth_m != null) },
      { value: CHART_TAB.sensoren, label: 'Sensoren', visible: streams.some(s => s.core_temperature_c != null) },
      { value: CHART_TAB.barometer, label: 'Barometer', visible: streams.some(s => s.absolute_pressure_pa != null) },
      { value: CHART_TAB.mtb, label: 'MTB', visible: streams.some(s => s.grit != null || s.flow != null) },
      { value: CHART_TAB.ausdauer, label: 'Ausdauer', visible: streams.some(s => s.potential_stamina != null) },
    ]
    return tabs.filter(t => t.visible)
  }, [streams])
  const showTabs = visibleTabs.length > 2

  const handleTabChange = (value: string) => {
    const next = new URLSearchParams(searchParams)
    if (value === CHART_TAB.alles) {
      next.delete('tab')
    } else {
      next.set('tab', value)
    }
    setSearchParams(next, { replace: true })
  }

  const validTab = visibleTabs.find(t => t.value === activeTab) ? activeTab : CHART_TAB.alles

  if (!showTabs) return <>{children(CHART_TAB.alles)}</>

  return (
    <Tabs value={validTab} onValueChange={handleTabChange}>
      <TabsList className="mb-4">
        {visibleTabs.map(t => (
          <TabsTrigger key={t.value} value={t.value}>{t.label}</TabsTrigger>
        ))}
      </TabsList>
      <div className="mt-2">
        {children(validTab)}
      </div>
    </Tabs>
  )
}

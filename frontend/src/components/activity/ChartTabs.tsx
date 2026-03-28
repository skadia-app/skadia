import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useSearchParams } from 'react-router-dom'
import type { StreamPoint } from '@/api/types'

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
  const activeTab = searchParams.get('tab') ?? 'alles'

  const tabs: TabDef[] = [
    { value: 'alles', label: 'Alles', visible: true },
    { value: 'uebersicht', label: 'Übersicht', visible: true },
    { value: 'leistung', label: 'Leistung', visible: streams.some(s => s.power_w != null) },
    { value: 'dynamik', label: 'Dynamik', visible: streams.some(s => s.ground_contact_time_ms != null) },
    { value: 'pedal', label: 'Pedal-Analytik', visible: streams.some(s => s.left_torque_effectiveness_pct != null) },
    { value: 'ebike', label: 'E-Bike', visible: streams.some(s => s.motor_power_w != null) },
    { value: 'tauchgang', label: 'Tauchgang', visible: streams.some(s => s.depth_m != null) },
    { value: 'sensoren', label: 'Sensoren', visible: streams.some(s => s.core_temperature_c != null) },
  ]

  const visibleTabs = tabs.filter(t => t.visible)
  const showTabs = visibleTabs.length > 2

  const handleTabChange = (value: string) => {
    const next = new URLSearchParams(searchParams)
    if (value === 'alles') {
      next.delete('tab')
    } else {
      next.set('tab', value)
    }
    setSearchParams(next, { replace: true })
  }

  const validTab = visibleTabs.find(t => t.value === activeTab) ? activeTab : 'alles'

  if (!showTabs) return <>{children('alles')}</>

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

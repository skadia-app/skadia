import { useState } from 'react'
import RangePresetBar, { type Preset, type PresetValue, type DateRange } from './RangePresetBar'
import { useHealthRange } from './HealthRangeContext'

const HEALTH_PRESETS: Preset[] = [
  { value: '7d',     label: 'Letzte 7 Tage' },
  { value: '30d',    label: '30 Tage' },
  { value: '90d',    label: '90 Tage' },
  { value: '365d',   label: '1 Jahr' },
  { value: 'custom', label: 'Benutzerdefiniert' },
]

const DEFAULT_PRESET: PresetValue = '30d'

export default function HealthRangePresetBar() {
  const { setRange } = useHealthRange()
  const [activePreset, setActivePreset] = useState<PresetValue>(DEFAULT_PRESET)

  function handleChange(range: DateRange, preset: PresetValue) {
    setActivePreset(preset)
    setRange(range)
  }

  return (
    <RangePresetBar
      presets={HEALTH_PRESETS}
      value={activePreset}
      onChange={handleChange}
    />
  )
}

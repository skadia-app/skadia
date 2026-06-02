import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import RangePresetBar, { type Preset, type PresetValue, type DateRange } from './RangePresetBar'
import { useHealthRange } from './HealthRangeContext'

const HEALTH_PRESETS: { value: PresetValue; labelKey: string }[] = [
  { value: '7d',     labelKey: 'analyticsHealthInsights.healthRange.7d' },
  { value: '30d',    labelKey: 'analyticsHealthInsights.healthRange.30d' },
  { value: '90d',    labelKey: 'analyticsHealthInsights.healthRange.90d' },
  { value: '365d',   labelKey: 'analyticsHealthInsights.healthRange.365d' },
  { value: 'custom', labelKey: 'analyticsHealthInsights.healthRange.custom' },
]

const DEFAULT_PRESET: PresetValue = '30d'

export default function HealthRangePresetBar() {
  const { t } = useTranslation()
  const { setRange } = useHealthRange()
  const [activePreset, setActivePreset] = useState<PresetValue>(DEFAULT_PRESET)
  const presets: Preset[] = HEALTH_PRESETS.map(p => ({
    value: p.value,
    label: t(p.labelKey),
  }))

  function handleChange(range: DateRange, preset: PresetValue) {
    setActivePreset(preset)
    setRange(range)
  }

  return (
    <RangePresetBar
      presets={presets}
      value={activePreset}
      onChange={handleChange}
    />
  )
}

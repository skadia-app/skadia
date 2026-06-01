import { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import RangePresetBar, { type Preset, type PresetValue, type DateRange, presetToRange } from './RangePresetBar'

interface Props {
  defaultPreset?: '7d' | '30d' | '90d' | '180d' | '1y' | 'all'
  value?: DateRange
  onChange: (range: DateRange) => void
}

export default function PureAggregateRangePicker({ defaultPreset = '90d', onChange }: Props) {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const presets: Preset[] = [
    { value: '7d', label: t('analyticsTrainingStats.filters.ranges.last7Days') },
    { value: '30d', label: t('analyticsTrainingStats.filters.ranges.days30') },
    { value: '90d', label: t('analyticsTrainingStats.filters.ranges.days90') },
    { value: '180d', label: t('analyticsTrainingStats.filters.ranges.days180') },
    { value: '1y', label: t('analyticsTrainingStats.filters.ranges.year1') },
    { value: 'all', label: t('common.all') },
    { value: 'custom', label: t('analyticsTrainingStats.filters.ranges.custom') },
  ]

  // On first mount: initialize from URL params if present, then detach (no URL sync).
  const [activePreset, setActivePreset] = useState<PresetValue>(() => {
    const urlFrom = searchParams.get('from')
    const urlTo = searchParams.get('to')
    if (urlFrom && urlTo) return 'custom'
    return defaultPreset
  })

  const [range, setRange] = useState<DateRange>(() => {
    const urlFrom = searchParams.get('from')
    const urlTo = searchParams.get('to')
    if (urlFrom && urlTo) return { from: urlFrom, to: urlTo }
    return presetToRange(defaultPreset)
  })

  // Emit the initial range on mount so the parent query can fire immediately.
  useEffect(() => {
    onChange(range)
    // Intentionally only on mount — searchParams changes are intentionally ignored
    // after initial read (detached from URL as per spec).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function handleChange(newRange: DateRange, preset: PresetValue) {
    setActivePreset(preset)
    setRange(newRange)
    onChange(newRange)
  }

  return (
    <RangePresetBar
      presets={presets}
      value={activePreset}
      onChange={handleChange}
    />
  )
}

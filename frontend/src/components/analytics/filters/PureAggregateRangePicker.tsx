import { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import RangePresetBar, { type Preset, type PresetValue, type DateRange, presetToRange } from './RangePresetBar'

const PURE_AGGREGATE_PRESETS: Preset[] = [
  { value: '7d',     label: 'Letzte 7 Tage' },
  { value: '30d',    label: '30 Tage' },
  { value: '90d',    label: '90 Tage' },
  { value: '180d',   label: '180 Tage' },
  { value: '1y',     label: '1 Jahr' },
  { value: 'all',    label: 'Alle' },
  { value: 'custom', label: 'Benutzerdefiniert' },
]

interface Props {
  defaultPreset?: '7d' | '30d' | '90d' | '180d' | '1y' | 'all'
  value?: DateRange
  onChange: (range: DateRange) => void
}

export default function PureAggregateRangePicker({ defaultPreset = '90d', onChange }: Props) {
  const [searchParams] = useSearchParams()

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
      presets={PURE_AGGREGATE_PRESETS}
      value={activePreset}
      onChange={handleChange}
    />
  )
}

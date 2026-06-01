import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import DateRangePicker from '@/components/DateRangePicker'
import { toLocalISO } from '@/lib/dateUtils'
import { cn } from '@/lib/utils'

export type PresetValue = '7d' | '30d' | '90d' | '180d' | '1y' | '365d' | 'all' | 'custom'

export interface Preset {
  value: PresetValue
  label?: string
}

export interface DateRange {
  from: string
  to: string
}

interface Props {
  presets: Preset[]
  /** Controlled active-preset value — caller is responsible for updating on onChange. */
  value: PresetValue
  /** Called with the computed date range AND the selected preset value. */
  onChange: (range: DateRange, preset: PresetValue) => void
  dense?: boolean
}

export function presetToRange(value: PresetValue): DateRange {
  const to = new Date()
  const from = new Date()
  switch (value) {
    case '7d':   from.setDate(from.getDate() - 7);             break
    case '30d':  from.setDate(from.getDate() - 30);            break
    case '90d':  from.setDate(from.getDate() - 90);            break
    case '180d': from.setDate(from.getDate() - 180);           break
    case '365d': from.setDate(from.getDate() - 365);           break
    case '1y':   from.setFullYear(from.getFullYear() - 1);     break
    case 'all':  return { from: '0001-01-01', to: toLocalISO(to) }
    case 'custom': return { from: toLocalISO(from), to: toLocalISO(to) }
  }
  return { from: toLocalISO(from), to: toLocalISO(to) }
}

export default function RangePresetBar({ presets, value, onChange, dense }: Props) {
  const { t } = useTranslation()
  const [customFrom, setCustomFrom] = useState('')
  const [customTo, setCustomTo] = useState('')
  const [showPicker, setShowPicker] = useState(false)

  const px = dense ? 'px-2 py-0.5' : 'px-2.5 py-1'
  const textSize = 'text-xs'

  const nonCustomPresets = presets.filter(p => p.value !== 'custom')
  const hasCustom = presets.some(p => p.value === 'custom')
  const customLabel = presets.find(p => p.value === 'custom')?.label ?? t('analyticsTrainingStats.filters.ranges.custom')

  function handlePresetClick(preset: Preset) {
    if (preset.value === 'custom') {
      setShowPicker(v => !v)
      return
    }
    setShowPicker(false)
    onChange(presetToRange(preset.value), preset.value)
  }

  function handleCustomChange(from: string, to: string) {
    setCustomFrom(from)
    setCustomTo(to)
    if (from && to) {
      onChange({ from, to }, 'custom')
      setShowPicker(false)
    }
  }

  return (
    <div className="flex items-center gap-1 flex-wrap">
      <div className={cn('flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700', textSize)}>
        {nonCustomPresets.map(preset => (
          <button
            key={preset.value}
            onClick={() => handlePresetClick(preset)}
            className={value === preset.value
              ? cn(px, 'bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer')
              : cn(px, 'hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 cursor-pointer transition-colors')}
          >
            {preset.label ?? preset.value}
          </button>
        ))}
      </div>

      {hasCustom && (
        <div className="relative">
          <button
            onClick={() => handlePresetClick({ value: 'custom', label: customLabel })}
            className={cn(
              textSize, px,
              'rounded-lg border cursor-pointer transition-colors',
              value === 'custom'
                ? 'border-neutral-200 dark:border-neutral-700 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                : 'border-neutral-200 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300',
            )}
          >
            {customLabel}
          </button>

          {showPicker && (
            // Render the picker popover directly — no intermediate "Zeitraum" trigger.
            <div data-testid="date-range-picker-wrapper" className="absolute top-full mt-1 right-0 z-30">
              <DateRangePicker
                dateFrom={customFrom}
                dateTo={customTo}
                onChange={handleCustomChange}
                defaultOpen
                triggerless
                onClose={() => setShowPicker(false)}
              />
            </div>
          )}
        </div>
      )}
    </div>
  )
}

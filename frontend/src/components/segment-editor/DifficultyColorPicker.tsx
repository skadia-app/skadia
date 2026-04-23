import { Pill } from '../ui/Pill'
import { ColorSwatch } from './ColorSwatch'
import type { SegType } from './constants'

interface Props {
  segType: SegType
  value: 'blue' | 'red' | 'black' | null
  onChange: (value: 'blue' | 'red' | 'black' | null) => void
  disabled?: boolean
}

export function DifficultyColorPicker({ segType, value, onChange, disabled }: Props) {
  if (segType !== 'wintersport') return null
  
  const colors: Array<{ key: 'blue' | 'red' | 'black'; label: string }> = [
    { key: 'blue', label: 'Blau (einfach)' },
    { key: 'red', label: 'Rot (mittel)' },
    { key: 'black', label: 'Schwarz (schwer)' },
  ]
  
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-neutral-500 uppercase tracking-wider">Schwierigkeit</label>
      <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label="Pistenfarbe">
        <Pill active={value === null} onClick={() => onChange(null)} disabled={disabled}>—</Pill>
        {colors.map(c => (
          <Pill
            key={c.key}
            active={value === c.key}
            onClick={() => onChange(c.key)}
            disabled={disabled}
            leftSlot={<ColorSwatch color={c.key} size={12} />}
          >
            {c.label}
          </Pill>
        ))}
      </div>
    </div>
  )
}

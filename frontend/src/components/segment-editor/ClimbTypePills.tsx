import { Pill } from './Pill'
import { CLIMB_TYPE_LABELS, type ClimbType, type SegType } from './constants'

interface ClimbTypePillsProps {
  segType: SegType
  value: ClimbType
  onChange: (value: ClimbType) => void
  disabled?: boolean
}

export function ClimbTypePills({ segType, value, onChange, disabled }: ClimbTypePillsProps) {
  if (segType !== 'standard' && segType !== 'trail') return null
  
  const types: ClimbType[] = ['standard', 'bergauf', 'bergab']
  
  return (
    <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label="Höhenprofil">
      {types.map(t => (
        <Pill
          key={t}
          active={value === t}
          onClick={() => onChange(t)}
          disabled={disabled}
          role="radio"
          ariaChecked={value === t}
        >
          {CLIMB_TYPE_LABELS[t]}
        </Pill>
      ))}
    </div>
  )
}

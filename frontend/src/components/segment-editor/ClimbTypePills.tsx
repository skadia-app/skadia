import { Pill } from '../ui/Pill'
import { type ClimbType, type SegType } from './constants'
import { useTranslation } from 'react-i18next'

interface ClimbTypePillsProps {
  segType: SegType
  value: ClimbType
  onChange: (value: ClimbType) => void
  disabled?: boolean
}

export function ClimbTypePills({ segType, value, onChange, disabled }: ClimbTypePillsProps) {
  const { t } = useTranslation()
  if (segType !== 'standard' && segType !== 'trail') return null
  
  const types: ClimbType[] = ['standard', 'bergauf', 'bergab']
  
  return (
    <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label={t('segmentEditor.sidebar.fields.elevationProfile')}>
      {types.map(climbType => (
        <Pill
          key={climbType}
          active={value === climbType}
          onClick={() => onChange(climbType)}
          disabled={disabled}
          role="radio"
          ariaChecked={value === climbType}
        >
          {t(`segmentEditor.climbTypes.${climbType}`)}
        </Pill>
      ))}
    </div>
  )
}

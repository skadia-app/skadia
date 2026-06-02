import { SAC_SCALE_OPTIONS, MTB_SCALE_OPTIONS, type SegType } from './constants'
import { SAC_BADGE_COLORS, MTB_BADGE_COLORS } from '@/lib/scaleColors'
import { ColoredSelect, type ColoredSelectOption } from '@/components/ui/ColoredSelect'
import { useTranslation } from 'react-i18next'

interface DifficultySelectsProps {
  segType: SegType
  sacScale: string | null
  mtbScale: string | null
  onChange: (field: 'sac' | 'mtb', value: string | null) => void
  disabled?: boolean
}

const sacOptions: ReadonlyArray<ColoredSelectOption> = SAC_SCALE_OPTIONS.map(o => ({
  value: o.value,
  label: o.label,
  shortLabel: o.value, // Trigger shows e.g. "T3"; dropdown list keeps "T3 — anspruchsvolles Bergwandern"
  color: SAC_BADGE_COLORS[o.value],
}))

const mtbOptions: ReadonlyArray<ColoredSelectOption> = MTB_SCALE_OPTIONS.map(o => ({
  value: o.value,
  label: o.label,
  color: MTB_BADGE_COLORS[o.value],
}))

export function DifficultySelects({ segType, sacScale, mtbScale, onChange, disabled }: DifficultySelectsProps) {
  const { t } = useTranslation()
  if (segType !== 'trail') return null

  return (
    <div className="grid grid-cols-1 gap-4">
      <div className="space-y-1.5">
        <label className="text-xs font-medium text-neutral-500 tracking-wide">{t('segmentEditor.sidebar.sacScale')}</label>
        <ColoredSelect
          value={sacScale}
          onChange={v => onChange('sac', v)}
          options={sacOptions}
          disabled={disabled}
        />
      </div>

      <div className="space-y-1.5">
        <label className="flex items-center gap-1.5 text-xs font-medium text-neutral-500 tracking-wide">
          {t('segmentEditor.sidebar.mtbScale')}
          <a
            href="https://www.singletrail-skala.de/"
            target="_blank"
            rel="noopener noreferrer"
            className="text-[10px] font-normal text-neutral-400 hover:text-accent-DEFAULT"
            title={t('segmentEditor.sidebar.officialSingletrailScale')}
          >
            {t('segmentEditor.sidebar.detailsLink')}
          </a>
        </label>
        <ColoredSelect
          value={mtbScale}
          onChange={v => onChange('mtb', v)}
          options={mtbOptions}
          disabled={disabled}
        />
      </div>
    </div>
  )
}

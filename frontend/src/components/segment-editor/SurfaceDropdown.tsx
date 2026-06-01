import { SURFACE_DROPDOWN_OPTIONS, type SegType } from './constants'
import { ColoredSelect, type ColoredSelectOption } from '@/components/ui/ColoredSelect'
import { useTranslation } from 'react-i18next'

interface SurfaceDropdownProps {
  segType: SegType
  value: string | null
  onChange: (value: string | null) => void
  disabled?: boolean
}

// Surface is relevant only for land-based seg types. Hidden for:
//  - wintersport: snow/ice implicit
//  - wassersport: water is the surface by definition
//  - lift: trivial infrastructure, no meaningful surface choice
// Parent (SegmentSidebar.handleSegTypeChange) already clears meta.surface on
// transitions into any of these, so values do not silently persist.
const SURFACE_HIDDEN_FOR: readonly SegType[] = ['wintersport', 'wassersport', 'lift']

export function SurfaceDropdown({ segType, value, onChange, disabled }: SurfaceDropdownProps) {
  const { t } = useTranslation()
  if (SURFACE_HIDDEN_FOR.includes(segType)) return null
  const hasKnownSurface = value == null || value === '' || SURFACE_DROPDOWN_OPTIONS.includes(value)
  const surfaceOptions: ReadonlyArray<ColoredSelectOption> = [
    ...SURFACE_DROPDOWN_OPTIONS.map(opt => ({
      value: opt,
      label: t(`segmentsList.surfaces.${opt}`),
    })),
    ...(!hasKnownSurface && value != null
      ? [{ value, label: t('segmentsList.surfaces.unknown') }]
      : []),
  ]

  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-neutral-500 tracking-wide">{t('segmentEditor.sidebar.surface')}</label>
      <ColoredSelect
        value={value}
        onChange={onChange}
        options={surfaceOptions}
        disabled={disabled}
      />
    </div>
  )
}

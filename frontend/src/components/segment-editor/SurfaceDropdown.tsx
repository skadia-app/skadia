import { SURFACE_DROPDOWN_OPTIONS, type SegType } from './constants'
import { ColoredSelect, type ColoredSelectOption } from '@/components/ui/ColoredSelect'

interface SurfaceDropdownProps {
  segType: SegType
  value: string | null
  onChange: (value: string | null) => void
  disabled?: boolean
}

// Surface is relevant for all seg types except wintersport (snow/ice implicit)
// and lift (trivial infrastructure — no meaningful surface choice). Fix: #6b.
const SURFACE_HIDDEN_FOR: readonly SegType[] = ['wintersport', 'lift']

const surfaceOptions: ReadonlyArray<ColoredSelectOption> = SURFACE_DROPDOWN_OPTIONS.map(opt => ({
  value: opt,
  label: opt,
}))

export function SurfaceDropdown({ segType, value, onChange, disabled }: SurfaceDropdownProps) {
  if (SURFACE_HIDDEN_FOR.includes(segType)) return null

  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-neutral-500 tracking-wide">Oberfläche</label>
      <ColoredSelect
        value={value}
        onChange={onChange}
        options={surfaceOptions}
        disabled={disabled}
        allowLegacy
      />
    </div>
  )
}

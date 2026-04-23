import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ChevronDown } from 'lucide-react'
import { SURFACE_DROPDOWN_OPTIONS, type SegType } from './constants'
import { cn } from '@/lib/utils'

interface SurfaceDropdownProps {
  segType: SegType
  value: string | null
  onChange: (value: string | null) => void
  disabled?: boolean
}

export function SurfaceDropdown({ segType, value, onChange, disabled }: SurfaceDropdownProps) {
  if (segType !== 'trail') return null
  
  const isLegacy = value !== null && !SURFACE_DROPDOWN_OPTIONS.includes(value)
  const displayOptions = isLegacy 
    ? [...SURFACE_DROPDOWN_OPTIONS, value!] 
    : SURFACE_DROPDOWN_OPTIONS
  
  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-neutral-500 uppercase tracking-wider">Surface</label>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button 
            type="button"
            disabled={disabled} 
            className={cn(
              "flex w-full items-center justify-between gap-1.5 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-2.5 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 transition-colors duration-150 hover:border-neutral-400 dark:hover:border-neutral-600 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent-DEFAULT disabled:opacity-50",
              isLegacy && "border-amber-400 dark:border-amber-600 text-amber-700 dark:text-amber-400"
            )}
          >
            <span className="truncate">{value ? (isLegacy ? `(veraltet) ${value}` : value) : '—'}</span>
            <ChevronDown className="h-3.5 w-3.5 shrink-0 text-neutral-400" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="z-[300] max-h-60 overflow-y-auto">
          <DropdownMenuItem onClick={() => onChange(null)}>—</DropdownMenuItem>
          {displayOptions.map(opt => (
            <DropdownMenuItem key={opt} onClick={() => onChange(opt)} className={opt === value ? "font-medium text-accent-DEFAULT" : ""}>
              {opt === value && isLegacy ? `(veraltet) ${opt}` : opt}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

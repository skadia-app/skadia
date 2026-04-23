import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ChevronDown } from 'lucide-react'
import { 
  SAC_SCALE_OPTIONS, MTB_SCALE_OPTIONS, 
  SAC_COLOR_MAP, MTB_COLOR_MAP,
  type SegType 
} from './constants'
import { ColorSwatch } from './ColorSwatch'
import { cn } from '@/lib/utils'

interface DifficultySelectsProps {
  segType: SegType
  sacScale: string | null
  mtbScale: string | null
  onChange: (field: 'sac' | 'mtb', value: string | null) => void
  disabled?: boolean
}

export function DifficultySelects({ segType, sacScale, mtbScale, onChange, disabled }: DifficultySelectsProps) {
  if (segType !== 'trail') return null
  
  return (
    <div className="grid grid-cols-1 gap-4">
      {/* SAC */}
      <div className="space-y-1.5">
        <label className="text-xs font-medium text-neutral-500 tracking-wide">SAC-Skala</label>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button 
              type="button"
              disabled={disabled} 
              className={cn(
                "flex w-full items-center justify-between gap-1.5 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-2.5 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 transition-colors duration-150 hover:border-neutral-400 dark:hover:border-neutral-600 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent-DEFAULT disabled:opacity-50"
              )}
            >
              <div className="flex items-center gap-1.5 truncate">
                {sacScale || '—'}
                {sacScale && SAC_COLOR_MAP[sacScale] && <ColorSwatch color={SAC_COLOR_MAP[sacScale] as any} size={12} />}
              </div>
              <ChevronDown className="h-3.5 w-3.5 shrink-0 text-neutral-400" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="z-[300] max-h-60 overflow-y-auto">
            <DropdownMenuItem onClick={() => onChange('sac', null)}>—</DropdownMenuItem>
            {SAC_SCALE_OPTIONS.map(opt => (
              <DropdownMenuItem key={opt.value} onClick={() => onChange('sac', opt.value)} className="flex items-center gap-2">
                <ColorSwatch color={SAC_COLOR_MAP[opt.value] as any} size={12} />
                <span>{opt.label}</span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      
      {/* MTB */}
      <div className="space-y-1.5">
        <label className="text-xs font-medium text-neutral-500 tracking-wide">MTB-Skala</label>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button 
              type="button"
              disabled={disabled} 
              className={cn(
                "flex w-full items-center justify-between gap-1.5 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-2.5 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 transition-colors duration-150 hover:border-neutral-400 dark:hover:border-neutral-600 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent-DEFAULT disabled:opacity-50"
              )}
            >
              <div className="flex items-center gap-1.5 truncate">
                {mtbScale || '—'}
                {mtbScale && MTB_COLOR_MAP[mtbScale] && <ColorSwatch color={MTB_COLOR_MAP[mtbScale] as any} size={12} />}
              </div>
              <ChevronDown className="h-3.5 w-3.5 shrink-0 text-neutral-400" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="z-[300] max-h-60 overflow-y-auto">
            <DropdownMenuItem onClick={() => onChange('mtb', null)}>—</DropdownMenuItem>
            {MTB_SCALE_OPTIONS.map(opt => (
              <DropdownMenuItem key={opt.value} onClick={() => onChange('mtb', opt.value)} className="flex items-center gap-2">
                <ColorSwatch color={MTB_COLOR_MAP[opt.value] as any} size={12} />
                <span>{opt.label}</span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}

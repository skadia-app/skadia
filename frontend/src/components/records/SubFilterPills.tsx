import { cn } from '@/lib/utils'
import type { SubFilter } from '@/lib/recordsConstants'

interface SubFilterPillsProps {
  filters: SubFilter[]
  active: string | null
  onChange: (label: string | null) => void
}

export function SubFilterPills({ filters, active, onChange }: SubFilterPillsProps) {
  if (filters.length === 0) return null

  return (
    <div className="flex items-center gap-1.5">
      {filters.map((f) => {
        const isActive = (f.label === 'Alle' && active === null) || f.label === active
        return (
          <button
            key={f.label}
            onClick={() => onChange(f.label === 'Alle' ? null : f.label)}
            className={cn(
              'px-3 py-1 rounded-lg text-xs font-medium border transition-colors',
              isActive
                ? 'bg-orange-500/15 text-orange-600 dark:text-orange-400 border-orange-500'
                : 'bg-transparent text-neutral-500 dark:text-neutral-400 border-neutral-300 dark:border-neutral-700 hover:text-neutral-700 dark:hover:text-neutral-200 hover:border-neutral-400 dark:hover:border-neutral-500',
            )}
          >
            {f.label}
          </button>
        )
      })}
    </div>
  )
}

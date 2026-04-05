import { useMemo } from 'react'
import type { BestEffortRanked } from '@/api/bestEfforts'
import { PRCell } from './PRCell'
import { CATEGORY_PREFIXES } from '@/lib/recordsConstants'
import { CATEGORY_LABELS } from '@/lib/bestEffortLabels'

const CATEGORY_KEYS = Object.keys(CATEGORY_LABELS)

interface SectionDef {
  prefix: string
  label: string
  color: string
}

const SECTIONS: SectionDef[] = [
  { prefix: CATEGORY_PREFIXES.dist,  label: 'Distanz',     color: 'blue' },
  { prefix: CATEGORY_PREFIXES.power, label: 'Leistung',    color: 'orange' },
  { prefix: CATEGORY_PREFIXES.elev,  label: 'Höhenmeter',  color: 'green' },
]

interface CategoryGridProps {
  records: BestEffortRanked[]
  sportGroup?: string
}

export function CategoryGrid({ records, sportGroup }: CategoryGridProps) {
  const rank1 = useMemo(() => records.filter((r) => r.rank === 1), [records])

  const sections = useMemo(() => {
    return SECTIONS.map((sec) => {
      const items = rank1
        .filter((r) => r.category.startsWith(sec.prefix))
        .sort((a, b) => {
          const ia = CATEGORY_KEYS.indexOf(a.category)
          const ib = CATEGORY_KEYS.indexOf(b.category)
          return (ia === -1 ? 999 : ia) - (ib === -1 ? 999 : ib)
        })
      return { ...sec, items }
    }).filter((sec) => sec.items.length > 0)
  }, [rank1])

  if (sections.length === 0) return null

  const borderClass: Record<string, string> = {
    blue: 'border-l-blue-500',
    orange: 'border-l-orange-500',
    green: 'border-l-green-500',
  }
  const textClass: Record<string, string> = {
    blue: 'text-blue-400',
    orange: 'text-orange-400',
    green: 'text-green-400',
  }

  return (
    <div className="space-y-3">
      {sections.map((sec) => (
        <div key={sec.prefix} className={`border-l-[3px] ${borderClass[sec.color]} pl-3`}>
          <div className={`text-[10px] font-semibold uppercase tracking-wider mb-2 ${textClass[sec.color]}`}>
            {sec.label}
          </div>
          <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-7 gap-1.5">
            {sec.items.map((record) => (
              <PRCell key={record.id} record={record} sportGroup={sportGroup} />
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

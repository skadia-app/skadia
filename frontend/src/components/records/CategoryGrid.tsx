import React, { useMemo } from 'react'
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

// Inline SVG icons from mockup
const SECTION_ICONS: Record<string, React.ReactElement> = {
  blue: (
    <svg className="w-3 h-3" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
      <path d="M5 12h14M12 5l7 7-7 7" />
    </svg>
  ),
  orange: (
    <svg className="w-3 h-3" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
      <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
    </svg>
  ),
  green: (
    <svg className="w-3 h-3" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
      <path d="M22 20L14 4l-4 8-6-4-2 12z" />
    </svg>
  ),
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
          <div className={`text-[10px] font-semibold uppercase tracking-wider mb-2 flex items-center gap-1.5 ${textClass[sec.color]}`}>
            {SECTION_ICONS[sec.color]}
            {sec.label}
          </div>
          <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-7 gap-1.5">
            {sec.items.map((record) => (
              <PRCell
                key={record.id}
                record={record}
                sportGroup={sportGroup}
                colorHint={sec.color as 'blue' | 'orange' | 'green'}
              />
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

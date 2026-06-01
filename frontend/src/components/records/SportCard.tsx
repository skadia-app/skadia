import type { BestEffortRanked } from '@/api/bestEfforts'
import { useTranslation } from 'react-i18next'
import { CategoryGrid } from './CategoryGrid'
import {
  MAIN_CATEGORIES,
  getMainCategoryLabel,
  getSportGroupBadgeLabel,
  type MainCategory,
} from '@/lib/recordsConstants'
import type { SupportedLanguage } from '@/i18n/languages'
import { pickBestPerCategory } from '@/lib/bestEffortRanking'
import { cn } from '@/lib/utils'

interface SportCardProps {
  mainCategory: MainCategory
  records: BestEffortRanked[]
  color: string
  onNavigate: () => void
  language?: SupportedLanguage
}

const VARIANT_BADGE_STYLES: Record<string, string> = {
  cycling_electric: 'bg-purple-500/15 text-purple-400 border-purple-500/20',
  cycling_virtual:  'bg-neutral-200 dark:bg-neutral-800 text-neutral-500 border-neutral-300 dark:border-neutral-700',
  swim_ow:          'bg-cyan-500/15 text-cyan-400 border-cyan-500/20',
  swim_pool:        'bg-cyan-500/10 text-cyan-300 border-cyan-500/15',
}

export function SportCard({ mainCategory, records, color, onNavigate, language }: SportCardProps) {
  const { t } = useTranslation()
  const rank1Count = pickBestPerCategory(records).length

  const sportGroups = MAIN_CATEGORIES[mainCategory]
  const presentGroups = new Set(records.map((r) => r.sport_group))
  const badges = sportGroups
    .filter((sg) => VARIANT_BADGE_STYLES[sg] && presentGroups.has(sg))
    .map((sg) => ({
      key: sg,
      label: getSportGroupBadgeLabel(sg, language),
      className: VARIANT_BADGE_STYLES[sg],
    }))

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
      <div
        className="px-5 py-3 bg-neutral-50 dark:bg-neutral-800/60 border-b border-neutral-200 dark:border-neutral-800 flex items-center gap-2.5 cursor-pointer"
        onClick={onNavigate}
      >
        <span className="w-3 h-3 rounded-full" style={{ backgroundColor: color }} />
        <span className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">
          {getMainCategoryLabel(mainCategory, language)}
        </span>
        <span className="text-xs text-neutral-500 dark:text-neutral-600">{t('records.recordCount', { count: rank1Count })}</span>
        {badges.length > 0 && (
          <div className="ml-auto flex items-center gap-1.5">
            {badges.map((b) => (
              <span key={b.key} className={cn('text-[10px] px-1.5 py-0.5 rounded border', b.className)}>
                {b.label}
              </span>
            ))}
            <svg className="w-3 h-3 text-neutral-400 dark:text-neutral-600 ml-1" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path d="M9 5l7 7-7 7" />
            </svg>
          </div>
        )}
        {badges.length === 0 && (
          <span className="ml-auto text-xs text-neutral-400 dark:text-neutral-600 flex items-center gap-1">
            {t('records.details')}
            <svg className="w-3 h-3" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path d="M9 5l7 7-7 7" />
            </svg>
          </span>
        )}
      </div>
      <div className="p-4">
        <CategoryGrid records={records} language={language} />
      </div>
    </div>
  )
}

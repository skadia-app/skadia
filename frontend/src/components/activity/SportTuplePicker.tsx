import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Search } from 'lucide-react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { selectableSportTupleOptions, sportDetailLabel } from '@/lib/sportTaxonomy'
import { isSupportedLanguage, type SupportedLanguage } from '@/i18n/languages'

interface SportTuplePickerProps {
  sportType: string
  subType: string
  onChange: (sportType: string, subType: string) => void
}

function currentLanguage(language: string): SupportedLanguage {
  const normalized = language.split('-')[0]
  return isSupportedLanguage(normalized) ? normalized : 'de'
}

export default function SportTuplePicker({ sportType, subType, onChange }: SportTuplePickerProps) {
  const { t, i18n } = useTranslation()
  const language = currentLanguage(i18n.language)
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const ref = useRef<HTMLDivElement>(null)
  useClickOutside(ref, () => setOpen(false), open)

  const options = useMemo(() => selectableSportTupleOptions(), [])
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return options
    return options.filter(option => {
      const label = sportDetailLabel(option.value, option.subType ?? null, language)
      return label.toLowerCase().includes(q) ||
        option.value.toLowerCase().includes(q) ||
        (option.subType ?? '').toLowerCase().includes(q)
    })
  }, [language, options, search])

  const currentLabel = sportDetailLabel(sportType, subType || null, language)

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(prev => !prev)}
        className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-left text-sm text-neutral-900 dark:text-neutral-100 hover:bg-neutral-50 dark:hover:bg-neutral-700"
      >
        {currentLabel}
      </button>
      {open && (
        <div className="absolute z-50 mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg">
          <div className="relative p-2">
            <Search className="pointer-events-none absolute left-4 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-neutral-400" />
            <input
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder={t('activityDetail.sportTuplePicker.searchPlaceholder')}
              className="w-full rounded border border-neutral-200 dark:border-neutral-700 bg-transparent py-1.5 pl-8 pr-2 text-sm text-neutral-900 outline-none focus:border-accent-400 dark:text-neutral-100"
            />
          </div>
          <div className="max-h-64 overflow-y-auto py-1">
            {filtered.map(option => {
              const optionSubType = option.subType ?? ''
              const selected = option.value === sportType && optionSubType === subType
              return (
                <button
                  key={`${option.value}:${optionSubType}`}
                  type="button"
                  onClick={() => {
                    onChange(option.value, optionSubType)
                    setSearch('')
                    setOpen(false)
                  }}
                  className={`w-full px-3 py-2 text-left text-sm ${selected
                    ? 'bg-accent-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
                    : 'text-neutral-700 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800'
                  }`}
                >
                  {sportDetailLabel(option.value, optionSubType || null, language)}
                </button>
              )
            })}
            {filtered.length === 0 && (
              <div className="px-3 py-2 text-sm text-neutral-400">{t('activityDetail.sportTuplePicker.noSportFound')}</div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

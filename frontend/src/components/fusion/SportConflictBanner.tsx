import { useTranslation } from 'react-i18next'
import { sportDetailLabel } from '@/lib/sportTaxonomy'
import { sportModifierSuffix } from '@/lib/sportModifiers'
import { isSupportedLanguage } from '@/i18n/languages'

export interface SportConflictChoice {
  key: string
  sportType: string
  subType?: string | null
  isElectric?: boolean
  isIndoor?: boolean
  isVirtual?: boolean
}

interface SportConflictBannerProps {
  choices: SportConflictChoice[]
  selectedKey: string | null
  onSelect: (choice: SportConflictChoice) => void
  isSaving?: boolean
}

/**
 * Banner shown when a recording group has members with different sport types.
 * Used in both DuplicateReviewPanel and RecordingGroupPage.
 */
export default function SportConflictBanner({
  choices,
  selectedKey,
  onSelect,
  isSaving = false,
}: SportConflictBannerProps) {
  const { t, i18n } = useTranslation()
  const language = isSupportedLanguage(i18n.language) ? i18n.language : 'de'

  return (
    <div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg px-4 py-3 mb-4">
      <p className="text-amber-700 dark:text-amber-400 text-sm font-medium mb-2">
        {t('workshopOperations.sportConflict.differentSportsDetected')}
      </p>
      <div className="flex gap-3">
        {choices.map(choice => {
          const suffix = sportModifierSuffix({
            isElectric: choice.isElectric,
            isIndoor: choice.isIndoor,
            isVirtual: choice.isVirtual,
          }, language)
          return (
          <button
            key={choice.key}
            onClick={() => onSelect(choice)}
            disabled={isSaving}
            className={`flex-1 rounded-lg py-2 text-sm text-center transition-colors border-2 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed ${
              selectedKey === choice.key
                ? 'border-amber-400 bg-amber-100 dark:bg-amber-800/40 text-amber-800 dark:text-amber-300 font-medium'
                : 'border-transparent bg-white dark:bg-neutral-800 text-neutral-700 dark:text-neutral-300 hover:border-amber-300'
            }`}
          >
            {sportDetailLabel(choice.sportType, choice.subType, language)}
            {suffix && (
              <span className="ml-1 text-[10px] uppercase tracking-wide opacity-70">
                {suffix}
              </span>
            )}
          </button>
          )
        })}
      </div>
    </div>
  )
}

import { sportLabel } from '@/lib/sportTaxonomy'

interface SportConflictBannerProps {
  sportTypes: string[]
  selectedSportType: string | null
  onSelect: (sportType: string) => void
  isSaving?: boolean
}

/**
 * Banner shown when a recording group has members with different sport types.
 * Used in both DuplicateReviewPanel and RecordingGroupPage.
 */
export default function SportConflictBanner({
  sportTypes,
  selectedSportType,
  onSelect,
  isSaving = false,
}: SportConflictBannerProps) {
  return (
    <div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg px-4 py-3 mb-4">
      <p className="text-amber-700 dark:text-amber-400 text-sm font-medium mb-2">
        Unterschiedliche Sportarten erkannt
      </p>
      <div className="flex gap-3">
        {sportTypes.map(st => (
          <button
            key={st}
            onClick={() => onSelect(st)}
            disabled={isSaving}
            className={`flex-1 rounded-lg py-2 text-sm text-center transition-colors border-2 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed ${
              selectedSportType === st
                ? 'border-amber-400 bg-amber-100 dark:bg-amber-800/40 text-amber-800 dark:text-amber-300 font-medium'
                : 'border-transparent bg-white dark:bg-neutral-800 text-neutral-700 dark:text-neutral-300 hover:border-amber-300'
            }`}
          >
            {sportLabel(st)}
          </button>
        ))}
      </div>
    </div>
  )
}

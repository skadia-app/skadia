import { useTranslation } from 'react-i18next'
import type { ActivitySet } from '@/api/types'
import { fmtDuration } from '@/lib/format'
import { isSupportedLanguage, localeForLanguage } from '@/i18n/languages'
import { formatWeight as formatUnitWeight, type UnitSystem } from '@/lib/units'

const EXERCISE_LABEL_KEYS: Record<number, string> = {
  0: 'benchPress', 1: 'calfRaise', 2: 'cardio', 3: 'carry',
  4: 'chopper',
  5: 'core', 6: 'crunches', 7: 'curls', 8: 'deadlift',
  9: 'flye', 10: 'hipRaise', 11: 'hipStability', 12: 'hipSwing',
  13: 'hyperextension', 14: 'lateralRaise',
  15: 'legCurl', 16: 'legRaise', 17: 'lunge',
  18: 'olympicLift', 19: 'plank', 20: 'plyometrics',
  21: 'pullUp', 22: 'pushUp', 23: 'rowExercise',
  24: 'shoulderPress', 25: 'shoulderStability', 26: 'shrug', 27: 'sitUps',
  28: 'squat', 29: 'totalBody', 30: 'tricepsPress', 31: 'warmUp',
  32: 'suspension', 33: 'kettlebell', 34: 'barbell', 35: 'dumbbell',
}

function exerciseLabel(categories: number[] | undefined, t: (key: string, options?: Record<string, unknown>) => string): string {
  if (!categories?.length) return t('activityDetail.strengthSets.unknown')
  const key = EXERCISE_LABEL_KEYS[categories[0]]
  return key ? t(`activityDetail.strengthSets.exercises.${key}`) : t('activityDetail.strengthSets.exerciseNumber', { number: categories[0] })
}

function setTypeLabel(type: number | undefined, t: (key: string, options?: Record<string, unknown>) => string): string {
  if (type === 0) return t('activityDetail.strengthSets.workSet')
  if (type === 1) return t('activityDetail.strengthSets.rest')
  return t('activityDetail.strengthSets.typeNumber', { type: type ?? '?' })
}

const FIT_BASE_UNIT_POUND = 2

function importedWeightUnit(weightDisplayUnit: number | undefined): UnitSystem {
  return weightDisplayUnit === FIT_BASE_UNIT_POUND ? 'imperial' : 'metric'
}

function formatImportedWeight(weightKg: number | null | undefined, weightDisplayUnit: number | undefined, locale: string): string {
  if (weightKg == null) return '—'
  return formatUnitWeight(weightKg, { locale, units: importedWeightUnit(weightDisplayUnit) })
}

interface Props { sets: ActivitySet[] }

export function StrengthSetsTable({ sets }: Props) {
  const { t, i18n } = useTranslation()
  const languageCode = i18n.resolvedLanguage ?? i18n.language
  const activeLanguage = isSupportedLanguage(languageCode) ? languageCode : 'de'
  const locale = localeForLanguage(activeLanguage)
  if (sets.length === 0) return null

  const groups: { label: string; sets: (ActivitySet & { index: number })[] }[] = []
  let currentLabel = ''
  let currentGroup: (ActivitySet & { index: number })[] = []

  sets.forEach((s, i) => {
    const label = exerciseLabel(s.exercise_categories, t)
    if (label !== currentLabel) {
      if (currentGroup.length > 0) groups.push({ label: currentLabel, sets: currentGroup })
      currentLabel = label
      currentGroup = []
    }
    currentGroup.push({ ...s, index: i + 1 })
  })
  if (currentGroup.length > 0) groups.push({ label: currentLabel, sets: currentGroup })

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="px-4 py-3 bg-neutral-50 dark:bg-neutral-900/60 border-b border-neutral-200 dark:border-neutral-800">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">{t('activityDetail.strengthSets.title')}</h3>
      </div>
      <div className="divide-y divide-neutral-100 dark:divide-neutral-800">
        {groups.map(g => (
          <div key={g.label} className="px-4 py-3">
            <div className="text-sm font-medium text-neutral-800 dark:text-neutral-200 mb-2">{g.label}</div>
            <table className="w-full text-sm">
              <thead>
                <tr className="text-xs text-neutral-500 dark:text-neutral-400">
                  <th className="text-left w-8">#</th>
                  <th className="text-left">{t('activityDetail.strengthSets.type')}</th>
                  <th className="text-right">{t('activityDetail.strengthSets.reps')}</th>
                  <th className="text-right">{t('activityDetail.strengthSets.weight')}</th>
                  <th className="text-right">{t('activityDetail.strengthSets.duration')}</th>
                </tr>
              </thead>
              <tbody>
                {g.sets.map(s => (
                  <tr key={s.index} className="text-neutral-700 dark:text-neutral-300">
                    <td className="tabular-nums">{s.index}</td>
                    <td>{setTypeLabel(s.set_type, t)}</td>
                    <td className="text-right tabular-nums">{s.repetitions ?? '—'}×</td>
                    <td className="text-right tabular-nums">
                      {formatImportedWeight(s.weight_kg, s.weight_display_unit, locale)}
                    </td>
                    <td className="text-right tabular-nums">{s.duration_s != null ? fmtDuration(s.duration_s, 'short') : '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
      </div>
    </div>
  )
}

import { useTranslation } from 'react-i18next'
import type { AutoTitleDisplayPolicy } from '@/api/types'
import '@/i18n/i18n'

type Props = {
  value: AutoTitleDisplayPolicy
  onChange: (value: AutoTitleDisplayPolicy) => void
  disabled?: boolean
}

const POLICY_OPTIONS: Array<{ value: Exclude<AutoTitleDisplayPolicy, 'source_unless_generic'>; labelKey: string; descriptionKey: string }> = [
  {
    value: 'source_first',
    labelKey: 'autoTitleDisplayPolicy.options.sourceFirst.label',
    descriptionKey: 'autoTitleDisplayPolicy.options.sourceFirst.description',
  },
  {
    value: 'auto_first',
    labelKey: 'autoTitleDisplayPolicy.options.autoFirst.label',
    descriptionKey: 'autoTitleDisplayPolicy.options.autoFirst.description',
  },
]

export function AutoTitleDisplayPolicySelect({ value, onChange, disabled = false }: Props) {
  const { t } = useTranslation()
  const uiValue = value === 'auto_first' ? 'auto_first' : 'source_first'
  const selected = POLICY_OPTIONS.find(option => option.value === uiValue) ?? POLICY_OPTIONS[0]

  return (
    <div className="flex flex-col gap-2 py-2">
      <div className="flex flex-col gap-1 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
        <div className="min-w-0">
          <label htmlFor="auto-title-display-policy" className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            {t('autoTitleDisplayPolicy.label')}
          </label>
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            {t(selected.descriptionKey)}
          </p>
        </div>
        <select
          id="auto-title-display-policy"
          value={uiValue}
          onChange={e => onChange(e.target.value as AutoTitleDisplayPolicy)}
          disabled={disabled}
          className="block w-full max-w-xs rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900 disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100"
        >
          {POLICY_OPTIONS.map(option => (
            <option key={option.value} value={option.value}>{t(option.labelKey)}</option>
          ))}
        </select>
      </div>
    </div>
  )
}

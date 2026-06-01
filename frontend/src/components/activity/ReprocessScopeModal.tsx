import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Capabilities } from '@/api/routes'
import {
  availableReprocessScopeKeys,
  DEFAULT_SCOPES,
  toBackendScope,
  toggleScopeKey,
  type AnyReprocessKey,
} from '@/lib/reprocessScopes'
import { ScopeCheckboxList } from '@/components/ScopeCheckboxList'

function ReprocessScopeModal({
  onConfirm,
  onClose,
  isPending,
  capabilities,
}: {
  onConfirm: (scope: string[]) => void
  onClose: () => void
  isPending: boolean
  capabilities?: Capabilities | null
}) {
  const { t } = useTranslation()
  const [scope, setScope] = useState<Set<AnyReprocessKey>>(new Set(DEFAULT_SCOPES))
  const availableScopeKeys = availableReprocessScopeKeys(capabilities)
  const selectedScopeKeys = toBackendScope(scope, availableScopeKeys)
  const reprocessesTags = selectedScopeKeys.includes('tags')

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <div className="bg-white dark:bg-neutral-900 rounded-xl shadow-xl p-6 max-w-sm w-full mx-4 space-y-4">
        <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
          {t('reprocessModal.selectDetailsTitle')}
        </h3>
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {t('reprocessModal.selectDetailsDescription')}
          {' '}
          {t('reprocessModal.preservation')}
          {reprocessesTags
            ? t('reprocessModal.tagsReprocessed')
            : t('reprocessModal.tagsPreserved')}
        </p>

        <ScopeCheckboxList
          selected={scope}
          onToggle={key => setScope(prev => toggleScopeKey(prev, key))}
          layout="vertical"
          capabilities={capabilities}
        />

        <div className="flex justify-end gap-3 pt-1">
          <button
            onClick={onClose}
            disabled={isPending}
            className="px-4 py-2 rounded-lg text-sm text-neutral-600 dark:text-neutral-400
                       hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer
                       disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {t('common.cancel')}
          </button>
          <button
            onClick={() => onConfirm(selectedScopeKeys)}
            disabled={isPending || selectedScopeKeys.length === 0}
            className="px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT text-white
                       hover:bg-accent-600 transition-colors cursor-pointer
                       disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isPending ? t('reprocessModal.processing') : t('reprocessModal.start')}
          </button>
        </div>
      </div>
    </div>
  )
}

export default ReprocessScopeModal

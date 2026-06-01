import { useTranslation } from 'react-i18next'
import ImportQueueTab from '@/components/ImportQueueTab'

export default function ImportQueuePage() {
  const { t } = useTranslation()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          {t('importQueue.page.title')}
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          {t('importQueue.page.description')}
        </p>
      </div>
      <ImportQueueTab />
    </div>
  )
}

import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import AdminStorageDashboard from '@/components/AdminStorageDashboard'
import { AdminLibrarySettingsTab } from '@/components/AdminLibrarySettingsTab'
import AdminUsersPage from '@/pages/admin/AdminUsersPage'
import AdminSystemTab from '@/pages/settings/AdminSystemTab'
import AdminSystemStatusTab from '@/pages/settings/AdminSystemStatusTab'
import AdminMapTilesSubTab from '@/pages/settings/AdminMapTilesSubTab'

const SUB_TABS = [
  { id: 'users' as const, labelKey: 'settings.adminTabs.users' },
  { id: 'status' as const, labelKey: 'settings.adminTabs.status' },
  { id: 'storage' as const, labelKey: 'settings.adminTabs.storage' },
  { id: 'system' as const, labelKey: 'settings.adminTabs.system' },
  { id: 'library' as const, labelKey: 'admin.library.title' },
  { id: 'maps' as const, labelKey: 'settings.adminTabs.maps' },
] as const

type SubTab = (typeof SUB_TABS)[number]['id']

const VALID_SUBS = new Set<SubTab>(['users', 'status', 'storage', 'system', 'library', 'maps'])

export default function AdminSettingsTab() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const rawSection = searchParams.get('section')
  const activeSubTab: SubTab = VALID_SUBS.has(rawSection as SubTab) ? (rawSection as SubTab) : 'users'

  const setSubTab = useCallback((sub: SubTab) => {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      if (sub === 'users') next.delete('section')
      else next.set('section', sub)
      return next
    }, { replace: true })
  }, [setSearchParams])

  return (
    <div className="space-y-6">
      <div className="flex gap-1 p-1 bg-neutral-100 dark:bg-neutral-800/50 rounded-lg w-fit">
        {SUB_TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => setSubTab(tab.id)}
            className={`px-3 py-1 rounded-md text-sm font-medium cursor-pointer transition-colors ${
              activeSubTab === tab.id
                ? 'bg-white dark:bg-neutral-700 text-neutral-900 dark:text-neutral-100 shadow-sm'
                : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200'
            }`}
          >
            {t(tab.labelKey)}
          </button>
        ))}
      </div>

      {activeSubTab === 'users' && <AdminUsersPage />}
      {activeSubTab === 'status' && <AdminSystemStatusTab />}
      {activeSubTab === 'storage' && <AdminStorageDashboard />}
      {activeSubTab === 'system' && <AdminSystemTab />}
      {activeSubTab === 'library' && <AdminLibrarySettingsTab />}
      {activeSubTab === 'maps' && <AdminMapTilesSubTab />}
    </div>
  )
}

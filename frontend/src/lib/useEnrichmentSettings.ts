import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchEnrichmentSettings, updateEnrichmentSettings } from '@/api/enrichment'
import type { EnrichmentSettings } from '@/api/types'
import { useToast } from '@/lib/toast'
import { useTranslation } from 'react-i18next'

const QUERY_KEY = ['enrichment-settings'] as const

export function useEnrichmentSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { t } = useTranslation()

  const query = useQuery({
    queryKey: QUERY_KEY,
    queryFn: fetchEnrichmentSettings,
  })

  const mutation = useMutation({
    mutationFn: (patch: Partial<EnrichmentSettings>) => updateEnrichmentSettings(patch),
    onSuccess: (updated) => {
      queryClient.setQueryData(QUERY_KEY, updated)
    },
    onError: () => {
      queryClient.invalidateQueries({ queryKey: QUERY_KEY })
      addToast({ type: 'error', title: t('settingsPage.analysis.saveFailed'), duration: 0 })
    },
  })

  return { settings: query.data, isLoading: query.isLoading, isError: query.isError, mutation }
}

import { i18n } from '@/i18n/i18n'
import type { EnrichmentModule } from './useEnrichmentProgress'

export function getEnrichmentModuleLabel(module: EnrichmentModule): string {
  return i18n.t(`jobs.enrichment.moduleLabels.${module}`)
}

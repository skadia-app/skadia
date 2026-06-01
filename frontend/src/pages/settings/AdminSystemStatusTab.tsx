import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import type { BuildInfo, ComponentStatus, SystemStatusState } from '@/api/systemStatus'
import { downloadSystemStatusExport, fetchSystemStatus } from '@/api/systemStatus'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { frontendBuildInfo } from '@/lib/buildInfo'
import { useToast } from '@/lib/toast'
import { triggerBlobDownload } from '@/lib/utils'

export default function AdminSystemStatusTab() {
  const { t, i18n } = useTranslation()
  const { addToast } = useToast()
  const [exporting, setExporting] = useState(false)
  const [featureSearch, setFeatureSearch] = useState('')
  const [featureFilter, setFeatureFilter] = useState<'all' | 'attention' | 'disabled'>('all')
  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin-system-status'],
    queryFn: fetchSystemStatus,
    staleTime: 15_000,
  })

  if (isLoading) return <div className="text-sm text-neutral-500">{t('settings.adminSystemStatus.loading')}</div>
  if (isError || !data) return <div className="text-sm text-red-500">{t('settings.adminSystemStatus.loadError')}</div>

  const build = {
    ...data.build,
    frontend: data.build.frontend ?? frontendBuildInfo(),
  }
  const featureRows = data.feature_matrix
    ? Object.values(data.feature_matrix)
      .filter((component) => matchesFeatureSearch(component, featureSearch, t))
      .filter((component) => matchesFeatureFilter(component, featureFilter))
      .sort((a, b) => featureLabel(a.key, a.label, t).localeCompare(featureLabel(b.key, b.label, t), i18n.language))
    : []

  async function handleExport() {
    if (exporting) return
    setExporting(true)
    try {
      const { blob, filename } = await downloadSystemStatusExport()
      triggerBlobDownload(blob, filename || 'skadia-system-status.json')
      addToast({ type: 'success', title: t('settings.adminSystemStatus.exportSuccess') })
    } catch {
      addToast({
        type: 'error',
        title: t('settings.adminSystemStatus.exportErrorTitle'),
        message: t('settings.adminSystemStatus.exportErrorMessage'),
        duration: 0,
      })
    } finally {
      setExporting(false)
    }
  }

  return (
    <div className="space-y-6">
      <SettingsSection title={t('settings.adminSystemStatus.title')} description={t('settings.adminSystemStatus.description')}>
        <div className="flex flex-col gap-3 text-sm sm:flex-row sm:items-start sm:justify-between">
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-neutral-600 dark:text-neutral-400">{t('settings.adminSystemStatus.overallStatus')}</span>
              <StatusBadge state={data.overall} t={t} />
            </div>
            <BuildTable build={build} t={t} />
          </div>
          <button type="button" onClick={handleExport} disabled={exporting} className="rounded border border-neutral-300 px-3 py-1.5 text-sm hover:bg-neutral-50 disabled:cursor-wait disabled:opacity-60 dark:border-neutral-700 dark:hover:bg-neutral-900">
            {exporting ? t('settings.adminSystemStatus.exporting') : t('settings.adminSystemStatus.export')}
          </button>
        </div>
      </SettingsSection>
      <SettingsSection title={t('settings.adminSystemStatus.sections.data.title')} description={t('settings.adminSystemStatus.sections.data.description')}>
        <StatusGrid items={[
          [t('settings.adminSystemStatus.components.db'), data.components.db],
          [t('settings.adminSystemStatus.components.timescale_compression'), data.components.timescale_compression],
          [t('settings.adminSystemStatus.components.storage'), data.components.storage],
          [t('settings.adminSystemStatus.components.jobs'), data.components.jobs],
          [t('settings.adminSystemStatus.components.gdpr_artifacts'), data.components.gdpr_artifacts],
        ]} t={t} />
      </SettingsSection>
      <SettingsSection title={t('settings.adminSystemStatus.sections.services.title')} description={t('settings.adminSystemStatus.sections.services.description')}>
        <StatusGrid items={[
          ['API', data.components.api],
          [t('settings.adminSystemStatus.components.ml'), data.components.ml],
          ['NATS / JetStream', data.components.nats],
        ]} t={t} />
      </SettingsSection>
      <SettingsSection title={t('settings.adminSystemStatus.sections.routing.title')} description={t('settings.adminSystemStatus.sections.routing.description')}>
        <StatusGrid items={[
          ['BRouter', data.components.brouter],
          ['BadRouter-Routinggraph', data.components.badrouter],
          [t('settings.adminSystemStatus.components.osm'), data.components.osm],
          [t('settings.adminSystemStatus.components.dem'), data.components.dem],
          [t('settings.adminSystemStatus.components.coverage'), data.components.coverage],
          [t('settings.adminSystemStatus.components.tiles'), data.components.tiles],
        ]} t={t} />
      </SettingsSection>
      {data.feature_matrix && (
        <SettingsSection title={t('settings.adminSystemStatus.sections.features.title')} description={t('settings.adminSystemStatus.sections.features.description')}>
          <div className="mb-4 flex flex-col gap-3 rounded-lg border border-neutral-200 bg-neutral-50 p-3 dark:border-neutral-800 dark:bg-neutral-900/40 lg:flex-row lg:items-center">
            <input
              type="search"
              value={featureSearch}
              onChange={(event) => setFeatureSearch(event.target.value)}
              placeholder={t('settings.adminSystemStatus.featureSearchPlaceholder')}
              className="min-h-10 flex-1 rounded-md border border-neutral-300 bg-white px-3 text-sm outline-none focus:border-accent-DEFAULT dark:border-neutral-700 dark:bg-neutral-950"
            />
            <select
              value={featureFilter}
              onChange={(event) => setFeatureFilter(event.target.value as 'all' | 'attention' | 'disabled')}
              className="min-h-10 rounded-md border border-neutral-300 bg-white px-3 text-sm outline-none focus:border-accent-DEFAULT dark:border-neutral-700 dark:bg-neutral-950"
            >
              <option value="all">{t('settings.adminSystemStatus.filters.all')}</option>
              <option value="attention">{t('settings.adminSystemStatus.filters.attention')}</option>
              <option value="disabled">{t('settings.adminSystemStatus.filters.disabled')}</option>
            </select>
          </div>
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {featureRows.map((component) => (
              <StatusCard key={component.key} title={featureLabel(component.key, component.label, t)} component={component} compact t={t} />
            ))}
          </div>
        </SettingsSection>
      )}
    </div>
  )
}

function StatusGrid({ items, t }: { items: Array<[string, ComponentStatus | undefined]>; t: TFunction }) {
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {items.map(([title, component]) => (
        <StatusCard key={title} title={title} component={component} t={t} />
      ))}
    </div>
  )
}

function StatusCard({ title, component, compact = false, t }: { title: string; component?: ComponentStatus; compact?: boolean; t: TFunction }) {
  const state = component?.state ?? 'unknown'
  const reason = component?.reason_code
  const details = component?.details ? Object.entries(component.details).slice(0, compact ? 2 : 3) : []
  return (
    <div className={`rounded-lg border border-neutral-200 bg-white p-4 dark:border-neutral-800 dark:bg-neutral-950/40 ${compact ? 'min-h-[7.25rem]' : 'min-h-[8.5rem]'}`}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">{title}</h3>
          {reason && <p className="mt-1 text-xs leading-5 text-neutral-500 dark:text-neutral-400">{reasonLabel(reason, t)}</p>}
        </div>
        <StatusBadge state={state} t={t} />
      </div>
      {details.length > 0 && (
        <dl className="mt-4 grid gap-1 text-xs text-neutral-500 dark:text-neutral-400">
          {details.map(([key, value]) => (
            <div key={key} className="flex min-w-0 justify-between gap-3">
              <dt className="truncate">{detailLabel(key, t)}</dt>
              <dd className="truncate text-right font-mono text-neutral-700 dark:text-neutral-300">{detailValue(value, t)}</dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  )
}

function StatusBadge({ state, t }: { state: SystemStatusState; t: TFunction }) {
  const color = stateTone(state)
  return (
    <span className={`inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium tabular-nums ${color.container}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${color.dot}`} />
      {stateLabel(state, t)}
    </span>
  )
}

function BuildTable({ build, t }: { build: Record<string, BuildInfo>; t: TFunction }) {
  const rows = ['api', 'frontend', 'ml']
    .map((key) => build[key])
    .filter((row): row is BuildInfo => Boolean(row))
  if (rows.length === 0) return null
  return (
    <div className="overflow-x-auto text-xs text-neutral-600 dark:text-neutral-400">
      <table className="min-w-full border-separate border-spacing-x-4 border-spacing-y-1">
        <thead>
          <tr className="text-left">
            <th>{t('settings.adminSystemStatus.build.service')}</th>
            <th>Commit</th>
            <th>{t('settings.adminSystemStatus.build.commitTime')}</th>
            <th>{t('settings.adminSystemStatus.build.buildTime')}</th>
            <th>{t('settings.adminSystemStatus.build.config')}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.service}>
              <td>{serviceLabel(row.service)}</td>
              <td className="font-mono">{shortValue(row.git_commit, t)}</td>
              <td>{fallbackValue(row.commit_time, t)}</td>
              <td>{fallbackValue(row.build_time, t)}</td>
              <td>{fallbackValue(row.config_version, t)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function translateKnown(t: TFunction, key: string, unknownKey: string): string {
  const translated = t(key)
  if (translated !== key) return translated
  return t(unknownKey)
}

function featureLabel(key: string, _backendLabel: string | undefined, t: TFunction): string {
  return translateKnown(t, `settings.adminSystemStatus.features.${key}`, 'settings.adminSystemStatus.features.unknown')
}

function matchesFeatureSearch(component: ComponentStatus, query: string, t: TFunction): boolean {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  return [
    featureLabel(component.key, component.label, t),
    reasonLabel(component.reason_code ?? '', t),
  ].some((value) => value.toLowerCase().includes(normalized))
}

function matchesFeatureFilter(component: ComponentStatus, filter: 'all' | 'attention' | 'disabled'): boolean {
  if (filter === 'all') return true
  if (filter === 'disabled') return component.state === 'disabled' || component.state === 'not_configured'
  return component.state !== 'ok' && component.state !== 'disabled' && component.state !== 'not_configured'
}

function stateTone(state: SystemStatusState): { container: string; dot: string } {
  switch (state) {
    case 'ok':
      return {
        container: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
        dot: 'bg-emerald-500',
      }
    case 'degraded':
    case 'missing_artifacts':
      return {
        container: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
        dot: 'bg-amber-500',
      }
    case 'unavailable':
    case 'blocked':
      return {
        container: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
        dot: 'bg-red-500',
      }
    case 'disabled':
    case 'not_configured':
      return {
        container: 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400',
        dot: 'bg-neutral-400',
      }
    default:
      return {
        container: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-400',
        dot: 'bg-sky-500',
      }
  }
}

function stateLabel(state: SystemStatusState, t: TFunction): string {
  return translateKnown(t, `settings.adminSystemStatus.states.${state}`, 'settings.adminSystemStatus.states.unknown')
}

function reasonLabel(reason: string, t: TFunction): string {
  if (!reason) return ''
  return translateKnown(t, `settings.adminSystemStatus.reasons.${reason}`, 'settings.adminSystemStatus.reasons.unknown')
}

function detailLabel(key: string, t: TFunction): string {
  return translateKnown(t, `settings.adminSystemStatus.details.${key}`, 'settings.adminSystemStatus.details.unknown')
}

function detailValue(value: string, t: TFunction): string {
  if (value === 'true') return t('common.yes')
  if (value === 'false') return t('common.no')
  return value
}

function serviceLabel(service: string): string {
  if (service === 'api') return 'API'
  if (service === 'frontend') return 'Frontend'
  if (service === 'ml') return 'ML'
  return service
}

function shortValue(value: string | undefined, t: TFunction): string {
  const raw = fallbackValue(value, t)
  if (raw === t('common.unknown')) return raw
  return raw.length > 12 ? raw.slice(0, 12) : raw
}

function fallbackValue(value: string | undefined, t: TFunction): string {
  if (!value || value === 'unknown') return t('common.unknown')
  return value
}

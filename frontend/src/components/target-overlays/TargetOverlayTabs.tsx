import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Activity, AlertTriangle, CheckCircle2, MessageSquare } from 'lucide-react'
import {
  addTargetReportStatus,
  getTargetOverlaySummary,
  listTargetObservations,
  listTargetReports,
  listTargetUserContent,
  type OverlaySubjectKind,
  type TargetReport,
} from '@/api/targetOverlays'
import {
  reportStatusLabel as formatReportStatusLabel,
  reportTypeLabel as formatReportTypeLabel,
} from '@/components/target-objects/targetLabels'
import ObservationList from './ObservationList'
import ReportComposer from './ReportComposer'
import UserContentPanel from './UserContentPanel'

type TargetOverlayTabsProps = {
  subjectKind: OverlaySubjectKind
  subjectId: string
  targetVersionId?: string
  canWrite?: boolean
  canManageReports?: boolean
}

type TabKey = 'activity' | 'reports' | 'content'

const TABS: Array<{ key: TabKey; labelKey: string; icon: typeof Activity }> = [
  { key: 'activity', labelKey: 'targetOverlays.tabs.activity', icon: Activity },
  { key: 'reports', labelKey: 'targetOverlays.tabs.reports', icon: AlertTriangle },
  { key: 'content', labelKey: 'targetOverlays.tabs.content', icon: MessageSquare },
]

export default function TargetOverlayTabs({ subjectKind, subjectId, targetVersionId, canWrite = false, canManageReports = false }: TargetOverlayTabsProps) {
  const { i18n, t } = useTranslation()
  const [activeTab, setActiveTab] = useState<TabKey>('activity')
  const queryClient = useQueryClient()
  const baseQuery = useMemo(() => ({
    subject_kind: subjectKind,
    subject_id: subjectId,
    target_version_id: targetVersionId,
  }), [subjectKind, subjectId, targetVersionId])
  const queryPrefix = ['target-overlays', subjectKind, subjectId, targetVersionId ?? null]

  const summary = useQuery({
    queryKey: [...queryPrefix, 'summary'],
    queryFn: () => getTargetOverlaySummary(baseQuery),
    enabled: Boolean(subjectId),
  })
  const observations = useQuery({
    queryKey: [...queryPrefix, 'observations'],
    queryFn: () => listTargetObservations({ ...baseQuery, limit: 30 }),
    enabled: Boolean(subjectId) && activeTab === 'activity',
  })
  const reports = useQuery({
    queryKey: [...queryPrefix, 'reports'],
    queryFn: () => listTargetReports({ ...baseQuery, limit: 30 }),
    enabled: Boolean(subjectId) && activeTab === 'reports',
  })
  const content = useQuery({
    queryKey: [...queryPrefix, 'content'],
    queryFn: () => listTargetUserContent({ ...baseQuery, limit: 50 }),
    enabled: Boolean(subjectId) && activeTab === 'content',
  })
  const resolveReport = useMutation({
    mutationFn: (report: TargetReport) => addTargetReportStatus(report.id, { status: 'resolved', reason: 'erledigt' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryPrefix })
    },
  })

  const overlaySummary = summary.data?.summary

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: queryPrefix })
  }

  return (
    <section className="space-y-4">
      <div className="flex flex-col gap-3 border-b border-neutral-200 pb-3 dark:border-neutral-800 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">{t('targetOverlays.title')}</h2>
          {overlaySummary && (
            <div className="mt-2 flex flex-wrap gap-2 text-xs text-neutral-500 dark:text-neutral-400">
              <SummaryChip label={t('targetOverlays.summary.open')} value={overlaySummary.public_open_report_count} />
              <SummaryChip label={t('targetOverlays.summary.activity')} value={overlaySummary.public_recent_observation_count_30d} />
              <SummaryChip label={t('targetOverlays.summary.comments')} value={overlaySummary.public_comment_count} />
              {overlaySummary.public_rating_count > 0 && (
                <SummaryChip label={t('targetOverlays.summary.rating')} value={overlaySummary.public_rating_mean?.toFixed(1) ?? String(overlaySummary.public_rating_count)} />
              )}
            </div>
          )}
        </div>
        <div className="flex overflow-x-auto">
          {TABS.map(tab => {
            const Icon = tab.icon
            const active = activeTab === tab.key
            return (
              <button
                key={tab.key}
                type="button"
                onClick={() => setActiveTab(tab.key)}
                className={`inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors ${
                  active
                    ? 'border-neutral-900 text-neutral-900 dark:border-neutral-100 dark:text-neutral-100'
                    : 'border-transparent text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100'
                }`}
              >
                <Icon size={15} />
                {t(tab.labelKey)}
              </button>
            )
          })}
        </div>
      </div>

      {activeTab === 'activity' && (
        <ObservationList observations={observations.data?.observations ?? []} loading={observations.isLoading} />
      )}

      {activeTab === 'reports' && (
        <div className="space-y-4">
          {canWrite && <ReportComposer subjectKind={subjectKind} subjectId={subjectId} targetVersionId={targetVersionId} onCreated={refresh} />}
          {reports.isLoading ? (
            <p className="py-4 text-sm text-neutral-500 dark:text-neutral-400">{t('targetOverlays.reports.loading')}</p>
          ) : (reports.data?.reports ?? []).length === 0 ? (
            <p className="py-4 text-sm text-neutral-500 dark:text-neutral-400">{t('targetOverlays.reports.empty')}</p>
          ) : (
            <div className="divide-y divide-neutral-200 dark:divide-neutral-800">
              {(reports.data?.reports ?? []).map(report => (
                <div key={report.id} className="flex items-start justify-between gap-3 py-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{formatReportTypeLabel(report.report_type, t)}</p>
                      {report.latest_status && (
                        <span className="rounded-md bg-neutral-100 px-2 py-0.5 text-xs text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400">
                          {formatReportStatusLabel(report.latest_status, t)}
                        </span>
                      )}
                    </div>
                    <p className="mt-1 whitespace-pre-wrap break-words text-sm text-neutral-700 dark:text-neutral-300">{report.description}</p>
                    <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">{formatDateTime(report.observed_at, i18n.language)}</p>
                  </div>
                  {canManageReports && report.latest_status === 'open' && (
                    <button
                      type="button"
                      onClick={() => resolveReport.mutate(report)}
                      disabled={resolveReport.isPending}
                      className="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-neutral-300 px-2.5 py-1.5 text-xs font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
                    >
                      <CheckCircle2 size={14} />
                      {t('targetOverlays.reports.resolve')}
                    </button>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {activeTab === 'content' && (
        <UserContentPanel
          subjectKind={subjectKind}
          subjectId={subjectId}
          targetVersionId={targetVersionId}
          content={content.data?.content ?? []}
          loading={content.isLoading}
          canWrite={canWrite}
          onChanged={refresh}
        />
      )}
    </section>
  )
}

function SummaryChip({ label, value }: { label: string; value: string | number }) {
  return <span className="rounded-md bg-neutral-100 px-2 py-1 dark:bg-neutral-800">{label}: {value}</span>
}

export { reportStatusLabel, reportTypeLabel } from '@/components/target-objects/targetLabels'

function formatDateTime(value: string, locale: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(locale, { dateStyle: 'medium', timeStyle: 'short' })
}

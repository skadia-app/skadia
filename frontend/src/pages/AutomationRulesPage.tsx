import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  fetchRules,
  createRule,
  updateRule,
  deleteRule,
  applyRules,
  dryRunAllRules,
  dryRunSingleRule,
  applySingleRule,
} from '@/api/rules'
import { fetchTags } from '@/api/tags'
import type { AutomationRule, RuleSportTupleConditionValue } from '@/api/types'
import { ApiError } from '@/api/client'
import type { ApplyResult, DryRunAllResult, DryRunSingleResult } from '@/api/rules'
import RuleEditor from '@/components/RuleEditor'
import Toggle from '@/components/Toggle'
import { useToast } from '@/lib/toast'
import { sportDetailLabel, sportLabel } from '@/lib/sportTaxonomy'
import { isSupportedLanguage, type SupportedLanguage } from '@/i18n/languages'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { displayAutomationRuleName, displaySystemTagName, displayTagName } from '@/lib/i18nDisplay'
import {
  formatDistanceCompact as formatUnitDistanceCompact,
  formatElevation as formatUnitElevation,
  formatPace as formatUnitPace,
  formatSpeed as formatUnitSpeed,
  formatTemperature,
  type UnitFormatOptions,
} from '@/lib/units'

const READONLY_SYNC_SYSTEM_TYPES = ['electric_sync', 'virtual_sync', 'indoor_sync'] as const

function isReadonlySyncRule(rule: AutomationRule): boolean {
  return Boolean(rule.is_system && rule.system_type && READONLY_SYNC_SYSTEM_TYPES.includes(rule.system_type as typeof READONLY_SYNC_SYSTEM_TYPES[number]))
}

interface DryRunMatch {
  activity_id: string
  activity_title: string
  started_at?: string
  sport_type?: string
  distance_m?: number
  rules: { rule_name: string; action: string }[]
}

const OP_SYMBOLS: Record<string, string> = {
  eq: '=', neq: '≠', gt: '>', lt: '<', gte: '≥', lte: '≤',
}

const OP_I18N_KEYS: Record<string, string> = {
  contains: 'contains',
  not_contains: 'notContains',
  starts_with: 'startsWith',
  ends_with: 'endsWith',
  between: 'between',
  in: 'in',
}

const DISTANCE_FIELDS = new Set(['distance_m', 'start_end_distance_m', 'net_distance_m', 'lift_distance_m'])
const SPEED_FIELDS = new Set(['avg_speed_ms', 'max_speed_ms', 'min_speed_ms', 'weather_wind_speed_ms', 'weather_wind_gusts_ms'])
const PACE_FIELDS = new Set(['avg_pace_min_km', 'min_pace_min_km', 'max_pace_min_km'])
const TEMPERATURE_FIELDS = new Set(['avg_temperature_c', 'min_temperature_c', 'max_temperature_c', 'weather_temp_c', 'weather_temp_min_c', 'weather_temp_max_c'])

function currentLanguage(language: string): SupportedLanguage {
  const code = language.split('-')[0]
  return isSupportedLanguage(code) ? code : 'de'
}

function fieldLabel(field: string, t: TFunction): string {
  const key = `automationRules.summary.fields.${field}`
  const label = t(key)
  return label === key ? t('automationRules.summary.unknownField') : label
}

function hasActionLabel(action: string, t: TFunction): boolean {
  const key = `automationRules.summary.actions.${action}`
  return t(key) !== key
}

function opLabel(op: string, t: TFunction): string {
  if (OP_SYMBOLS[op]) return OP_SYMBOLS[op]
  const key = OP_I18N_KEYS[op]
  return key ? t(`ruleEditor.operators.${key}`) : op
}

function yesNo(value: unknown, t: TFunction): string {
  return value ? t('ruleEditor.choices.yes') : t('ruleEditor.choices.no')
}

function numericConditionValue(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const numeric = Number(value)
  return Number.isFinite(numeric) ? numeric : null
}

function formatNumericConditionValue(field: string, value: unknown, unitOptions: UnitFormatOptions): string | null {
  const numeric = numericConditionValue(value)
  if (numeric == null) return null
  if (DISTANCE_FIELDS.has(field)) return formatUnitDistanceCompact(numeric, unitOptions)
  if (SPEED_FIELDS.has(field)) return formatUnitSpeed(numeric, unitOptions)
  if (PACE_FIELDS.has(field)) return formatUnitPace(numeric * 60, unitOptions)
  if (TEMPERATURE_FIELDS.has(field)) return formatTemperature(numeric, unitOptions)
  return null
}

function formatConditionValue(c: { field: string; op: string; value: unknown }, t: TFunction, language: SupportedLanguage, unitOptions: UnitFormatOptions): string {
  if (c.field === 'sport_tuple') return formatSportTupleConditionValue(c.value, t, language)
  if (typeof c.value === 'boolean') return yesNo(c.value, t)
  if (c.value === null || c.value === undefined) return ''
  const v = String(c.value)
  // Sport type -> localized label
  if (c.field === 'sport_type') return sportLabel(v, null, { language })
  const formattedNumeric = formatNumericConditionValue(c.field, c.value, unitOptions)
  if (formattedNumeric) return formattedNumeric
  if (c.field.includes('elevation') || c.field.includes('ascent') || c.field.includes('descent')) {
    const n = Number(v)
    if (!isNaN(n)) return formatUnitElevation(n, unitOptions)
  }
  // Between: "min,max" → formatted
  if (c.op === 'between' && v.includes(',')) {
    const [lo, hi] = v.split(',', 2)
    const loN = Number(lo), hiN = Number(hi)
    if (!isNaN(loN) && !isNaN(hiN)) {
      const formattedLo = formatNumericConditionValue(c.field, loN, unitOptions)
      const formattedHi = formatNumericConditionValue(c.field, hiN, unitOptions)
      if (formattedLo && formattedHi) return `${formattedLo}–${formattedHi}`
    }
    return `${lo}–${hi}`
  }
  return v
}

function formatSportTupleConditionValue(value: unknown, t: TFunction, language: SupportedLanguage): string {
  if (!Array.isArray(value)) return ''
  return value.map((entry) => {
    const tuple = entry as RuleSportTupleConditionValue
    const parts = [sportDetailLabel(tuple.sport_type, tuple.sub_type ?? null, language)]
    if (tuple.is_electric !== undefined) parts.push(`${t('automationRules.summary.modifiers.electric')}=${yesNo(tuple.is_electric, t)}`)
    if (tuple.is_indoor !== undefined) parts.push(`${t('automationRules.summary.modifiers.indoor')}=${yesNo(tuple.is_indoor, t)}`)
    if (tuple.is_virtual !== undefined) parts.push(`${t('automationRules.summary.modifiers.virtual')}=${yesNo(tuple.is_virtual, t)}`)
    if (tuple.is_commute !== undefined) parts.push(`${t('automationRules.summary.modifiers.commute')}=${yesNo(tuple.is_commute, t)}`)
    return parts.join(', ')
  }).join(` ${t('ruleEditor.logic.or')} `)
}

function formatActionParams(action: string, params: Record<string, unknown> | undefined, tags: { id: string; name: string; display_key?: string | null }[] | undefined, t: TFunction, language: SupportedLanguage): string {
  if (!params) return ''
  if (!hasActionLabel(action, t)) return ''
  if ((action === 'add_tag' || action === 'remove_tag') && params.tag_id) {
    const tag = tags?.find((t) => t.id === params.tag_id)
    return tag ? `"${displayTagName(tag, t)}"` : String(params.tag_id)
  }
  if (action === 'set_field' && params.field) {
    const label = fieldLabel(String(params.field), t)
    return `${label} = ${params.value}`
  }
  if (action === 'set_sport_type') return sportDetailLabel(String(params.sport_type || ''), params.sub_type == null ? null : String(params.sub_type), language)
  if (action === 'set_electric') return yesNo(params.electric, t)
  if (action === 'set_name') return `"${params.template || ''}"`
  if (action === 'set_visibility') return String(params.visibility || '')
  if (action === 'set_intensity') return String(params.intensity || '')
  if (action === 'set_indoor') return yesNo(params.indoor, t)
  if (action === 'set_virtual') return yesNo(params.virtual, t)
  if (action === 'set_commute') return yesNo(params.commute, t)
  if (action === 'set_gear' && params.gear_id) return String(params.gear_id)
  return JSON.stringify(params)
}

export default function AutomationRulesPage() {
  const { t, i18n } = useTranslation()
  const unitFormatters = useUnitFormatters()
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()

  // URL params: ?new=1&tag_name=foo or ?new=1&tag_id=uuid
  const urlNew = searchParams.get('new') === '1'
  const urlTagName = searchParams.get('tag_name')
  const urlTagId = searchParams.get('tag_id')

  const { addToast } = useToast()

  const [creating, setCreating] = useState(urlNew)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [applyResult, setApplyResult] = useState<ApplyResult | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)
  const [showReadonlySyncRules, setShowReadonlySyncRules] = useState(false)
  const [search, setSearch] = useState('')
  const [dryRunResults, setDryRunResults] = useState<DryRunAllResult | null>(null)
  const [dryRunLoading, setDryRunLoading] = useState(false)

  // Build initial rule data from URL params
  const initialRuleData: Partial<AutomationRule> | undefined = urlNew && (urlTagName || urlTagId)
    ? {
        name: urlTagName ? `${t('automationRules.summary.actions.add_tag')} "${urlTagName}"` : '',
        actions: [{ action: 'add_tag', params: { tag: urlTagName ?? '' } }],
      }
    : undefined

  async function handleDryRunAll() {
    setDryRunLoading(true)
    try {
      const result = await dryRunAllRules()
      setDryRunResults(result)
    } catch {
      addToast({
        type: 'error',
        title: t('automationRules.toasts.previewFailed'),
        message: t('automationRules.toasts.rulesCouldNotBeEvaluated'),
        duration: 0,
      })
    } finally {
      setDryRunLoading(false)
    }
  }

  const { data: rules, isLoading } = useQuery({
    queryKey: ['automation-rules'],
    queryFn: fetchRules,
  })

  const { data: allTags } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })

  function clearUrlParams() {
    if (urlNew || urlTagName || urlTagId) {
      setSearchParams({}, { replace: true })
    }
  }

  const createMutation = useMutation({
    mutationFn: (data: Partial<AutomationRule>) => createRule(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['automation-rules'] })
      setCreating(false)
      clearUrlParams()
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<AutomationRule> }) => updateRule(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['automation-rules'] })
      setEditingId(null)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteRule(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['automation-rules'] })
      setDeleteConfirmId(null)
    },
    onError: (err: unknown) => {
      // Prefer RFC7807 `detail` field over the raw body string so the user sees
      // the human-readable message rather than the full JSON envelope.
      const msg = err instanceof ApiError
        ? (typeof err.json?.detail === 'string' ? err.json.detail : err.body)
        : err instanceof Error ? err.message : t('tagsPage.unknownError')
      addToast({ type: 'error', title: t('automationRules.toasts.deleteFailed'), message: msg, duration: 0 })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, is_active }: { id: string; is_active: boolean }) => updateRule(id, { is_active }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['automation-rules'] })
    },
  })

  const applyMutation = useMutation({
    mutationFn: applyRules,
    onSuccess: (result) => {
      setApplyResult(result)
      queryClient.invalidateQueries({ queryKey: ['automation-rules'] })
      setTimeout(() => setApplyResult(null), 5000)
    },
  })

  const visibleRules = (rules ?? []).filter(r => {
    if (!showReadonlySyncRules && isReadonlySyncRule(r)) {
      return false
    }
    if (search) {
      const q = search.toLowerCase()
      if (r.name.toLowerCase().includes(q)) return true
      const raw = JSON.stringify(r.conditions) + JSON.stringify(r.condition_groups) + JSON.stringify(r.actions)
      return raw.toLowerCase().includes(q)
    }
    return true
  })

  const systemRules = visibleRules.filter(r => r.is_system)
  const userRules = visibleRules.filter(r => !r.is_system)
  const sortedSystemRules = [
    ...systemRules.filter(isReadonlySyncRule),
    ...systemRules.filter(rule => !isReadonlySyncRule(rule)),
  ]

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">{t('automationRules.loading')}</div>
  }

  return (
    <div className="space-y-6">
      <Link
        to={urlTagName || urlTagId ? '/workshop/tags' : '/workshop'}
        className="inline-flex items-center gap-1 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
      >
        <span>&#8592;</span> {urlTagName || urlTagId ? t('automationRules.backToTags') : t('navigation.workshop')}
      </Link>

      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{t('automationRules.title')}</h1>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-sm text-neutral-500 dark:text-neutral-400 cursor-pointer select-none">
            <input
              type="checkbox"
              checked={showReadonlySyncRules}
              onChange={e => setShowReadonlySyncRules(e.target.checked)}
              className="rounded"
            />
            {t('automationRules.showSyncRules')}
          </label>
          <button
            onClick={handleDryRunAll}
            disabled={dryRunLoading || applyMutation.isPending}
            className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 border border-neutral-300 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            {dryRunLoading ? t('automationRules.actions.checking') : t('automationRules.actions.preview')}
          </button>
          <button
            onClick={() => applyMutation.mutate()}
            disabled={applyMutation.isPending || dryRunLoading}
            className="px-4 py-2 rounded-lg text-sm font-medium text-accent-600 dark:text-orange-400 border border-orange-300 dark:border-orange-700 hover:bg-orange-50 dark:hover:bg-orange-900/20 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            {applyMutation.isPending ? t('automationRules.actions.applying') : t('automationRules.actions.applyRules')}
          </button>
          <button
            onClick={() => setCreating(true)}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 cursor-pointer transition-colors"
          >
            {t('automationRules.actions.createRule')}
          </button>
        </div>
      </div>

      {applyResult && (
        <div className="rounded-lg border border-emerald-300 dark:border-emerald-700 bg-emerald-50 dark:bg-emerald-900/20 px-4 py-3 text-sm">
          <span className="text-emerald-700 dark:text-emerald-400">
            {applyResult.job_id
              ? t('automationRules.results.started')
              : applyResult.applied != null
                ? t('automationRules.results.applied', { count: applyResult.applied })
                : t('automationRules.results.completed')}
          </span>
        </div>
      )}

      {/* Search */}
      <input
        type="text"
        placeholder={t('automationRules.searchPlaceholder')}
        value={search}
        onChange={e => setSearch(e.target.value)}
        className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 placeholder:text-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
      />

      {/* Create form */}
      {creating && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">{t('automationRules.newRule')}</h2>
          <RuleEditor
            initialData={initialRuleData}
            onSave={(data) => createMutation.mutate(data)}
            onCancel={() => { setCreating(false); clearUrlParams() }}
            isSaving={createMutation.isPending}
          />
        </div>
      )}

      {/* System rules section */}
      {systemRules.length > 0 && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-2">{t('automationRules.systemRules')}</h3>
          <div className="space-y-3">
            {sortedSystemRules.map((rule) => (
              <div key={rule.id}>
                {editingId === rule.id ? (
                  <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
                    <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">{t('automationRules.editRule')}</h2>
                    <RuleEditor
                      rule={rule}
                      onSave={(data) => updateMutation.mutate({ id: rule.id, data })}
                      onCancel={() => setEditingId(null)}
                      isSaving={updateMutation.isPending}
                    />
                  </div>
                ) : (
                  <RuleCard
                    rule={rule}
                    tags={allTags}
                    onEdit={() => setEditingId(rule.id)}
                    onToggle={(active) => toggleMutation.mutate({ id: rule.id, is_active: active })}
                    onDelete={() => setDeleteConfirmId(rule.id)}
                    onBackfill={addToast}
                  />
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* User rules section */}
      {userRules.length > 0 ? (
        <div>
          {systemRules.length > 0 && (
            <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-2">{t('automationRules.myRules')}</h3>
          )}
          <div className="space-y-3">
            {userRules.map((rule) => (
              <div key={rule.id}>
                {editingId === rule.id ? (
                  <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
                    <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">{t('automationRules.editRule')}</h2>
                    <RuleEditor
                      rule={rule}
                      onSave={(data) => updateMutation.mutate({ id: rule.id, data })}
                      onCancel={() => setEditingId(null)}
                      isSaving={updateMutation.isPending}
                    />
                  </div>
                ) : (
                  <RuleCard
                    rule={rule}
                    tags={allTags}
                    onEdit={() => setEditingId(rule.id)}
                    onToggle={(active) => toggleMutation.mutate({ id: rule.id, is_active: active })}
                    onDelete={() => setDeleteConfirmId(rule.id)}
                    onBackfill={addToast}
                  />
                )}
              </div>
            ))}
          </div>
        </div>
      ) : (
        !creating && systemRules.length === 0 && (
          <div className="text-center py-16 text-neutral-400">
            {t('automationRules.empty')}
          </div>
        )
      )}

      {/* Dry-run preview modal */}
      {dryRunResults !== null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setDryRunResults(null)}>
          <div
            className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-700 p-6 shadow-xl max-w-2xl w-full mx-4 space-y-4 max-h-[80vh] flex flex-col"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
                {t('automationRules.preview.title')}
              </h3>
              <button
                onClick={() => setDryRunResults(null)}
                className="text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 cursor-pointer transition-colors"
                title={t('common.close')}
              >
                <svg className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                </svg>
              </button>
            </div>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">
              {dryRunResults.total_matches === 0
                ? t('automationRules.preview.noMatchesActive')
                : dryRunResults.total_matches === 1
                  ? t('automationRules.preview.oneMatch')
                  : t('automationRules.preview.matchCount', { count: dryRunResults.total_matches })}
              {dryRunResults.truncated && t('automationRules.preview.truncated')}
            </p>
            {dryRunResults.matches.length > 0 && (
              <div className="overflow-y-auto flex-1 space-y-2">
                {(dryRunResults.matches as DryRunMatch[]).map((m) => (
                  <div key={m.activity_id} className="rounded-lg border border-neutral-200 dark:border-neutral-700 px-3 py-2">
                    <a
                      href={`/activities/${m.activity_id}`}
                      className="text-sm font-medium text-neutral-900 dark:text-neutral-100 hover:text-accent-DEFAULT truncate block"
                      onClick={(e) => e.stopPropagation()}
                    >
                      {m.activity_title || m.activity_id}
                      {m.started_at && (
                        <span className="text-neutral-400 font-normal ml-2">{m.started_at}</span>
                      )}
                      {m.sport_type && (
                        <span className="text-neutral-400 font-normal ml-2">{sportLabel(m.sport_type, null, { language: currentLanguage(i18n.language) })}</span>
                      )}
                      {m.distance_m != null && m.distance_m > 0 && (
                        <span className="text-neutral-400 font-normal ml-2">{unitFormatters.formatDistanceCompact(m.distance_m)}</span>
                      )}
                    </a>
                    <div className="mt-0.5 flex flex-wrap gap-1.5">
                      {m.rules.map((rm, i) => (
                        <span key={i} className="inline-flex items-center gap-1 text-xs bg-orange-50 dark:bg-orange-900/20 text-orange-700 dark:text-orange-400 border border-orange-200 dark:border-orange-800 rounded px-1.5 py-0.5">
                          <span className="font-medium">{displaySystemTagName(rm.rule_name, t)}</span>
                          <span className="text-orange-500 dark:text-orange-500">→</span>
                          <span>{rm.action}</span>
                        </span>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            )}
            <div className="flex items-center justify-end gap-3 pt-2 border-t border-neutral-200 dark:border-neutral-700">
              <button
                onClick={() => setDryRunResults(null)}
                className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
              >
                {t('common.cancel')}
              </button>
              <button
                onClick={() => { setDryRunResults(null); applyMutation.mutate() }}
                disabled={applyMutation.isPending}
                className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
              >
                {t('automationRules.actions.applyRules')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete confirmation dialog */}
      {deleteConfirmId !== null && (() => {
        const ruleToDelete = rules?.find(r => r.id === deleteConfirmId)
        if (!ruleToDelete) return null
        return (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setDeleteConfirmId(null)}>
            <div
              className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-700 p-6 shadow-xl max-w-sm w-full mx-4 space-y-4"
              onClick={(e) => e.stopPropagation()}
            >
              <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">{t('automationRules.deleteDialog.title')}</h3>
              <p className="text-sm text-neutral-600 dark:text-neutral-400">
                {t('automationRules.deleteDialog.confirmPrefix')}{' '}
                <span className="font-semibold text-neutral-900 dark:text-neutral-100">"{displayAutomationRuleName(ruleToDelete, t)}"</span>{' '}
                {t('automationRules.deleteDialog.confirmSuffix')}
              </p>
              <div className="flex items-center justify-end gap-3">
                <button
                  onClick={() => setDeleteConfirmId(null)}
                  disabled={deleteMutation.isPending}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {t('common.cancel')}
                </button>
                <button
                  onClick={() => deleteMutation.mutate(deleteConfirmId)}
                  disabled={deleteMutation.isPending}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-rose-500 hover:bg-rose-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
                >
                  {deleteMutation.isPending ? t('automationRules.deleteDialog.deleting') : t('automationRules.deleteDialog.delete')}
                </button>
              </div>
            </div>
          </div>
        )
      })()}
    </div>
  )
}

function RuleCard({
  rule,
  tags,
  onEdit,
  onToggle,
  onDelete,
  onBackfill,
}: {
  rule: AutomationRule
  tags?: { id: string; name: string; display_key?: string | null }[]
  onEdit: () => void
  onToggle: (active: boolean) => void
  onDelete: () => void
  onBackfill?: ReturnType<typeof useToast>['addToast']
}) {
  const { t, i18n } = useTranslation()
  const unitFormatters = useUnitFormatters()
  const [backfillPreview, setBackfillPreview] = useState<DryRunSingleResult | null>(null)
  const [backfillLoading, setBackfillLoading] = useState(false)
  const [backfillApplying, setBackfillApplying] = useState(false)
  const language = currentLanguage(i18n.language)
  const ruleLabel = displayAutomationRuleName(rule, t)

  async function handleBackfillPreview() {
    if (!onBackfill) return
    setBackfillLoading(true)
    try {
      const result = await dryRunSingleRule(rule.id)
      setBackfillPreview(result)
    } catch {
      onBackfill({ type: 'error', title: t('automationRules.toasts.previewFailed'), message: t('automationRules.toasts.ruleCouldNotBeEvaluated'), duration: 0 })
    } finally {
      setBackfillLoading(false)
    }
  }

  async function handleBackfillApply() {
    if (!onBackfill) return
    setBackfillApplying(true)
    try {
      await applySingleRule(rule.id)
      setBackfillPreview(null)
      onBackfill({ type: 'success', title: t('automationRules.toasts.ruleWillBeApplied', { name: ruleLabel }) })
    } catch {
      onBackfill({ type: 'error', title: t('automationRules.toasts.backfillFailed'), message: t('automationRules.toasts.ruleCouldNotBeApplied'), duration: 0 })
    } finally {
      setBackfillApplying(false)
    }
  }

  const isReadonlySystem = isReadonlySyncRule(rule)

  const readonlyDescriptions: Record<string, string> = {
    electric_sync: t('automationRules.readonly.electricSync'),
    virtual_sync: t('automationRules.readonly.virtualSync'),
    indoor_sync: t('automationRules.readonly.indoorSync'),
  }
  const readonlyDescription = rule.system_type ? (readonlyDescriptions[rule.system_type] ?? t('automationRules.readonly.default')) : ''

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
      <div className="flex items-start justify-between">
        <div
          className={`flex-1 min-w-0 ${!isReadonlySystem ? 'cursor-pointer' : ''}`}
          onClick={!isReadonlySystem ? onEdit : undefined}
        >
          <div className="flex items-center gap-3">
            <h3 className="text-base font-medium text-neutral-900 dark:text-neutral-100">{ruleLabel}</h3>
            {rule.is_system && isReadonlySystem && (
              <span className="text-xs bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 px-1.5 py-0.5 rounded font-medium">
                {t('automationRules.card.system')}
              </span>
            )}
            <span className="text-xs text-neutral-400 tabular-nums">{t('automationRules.card.priority', { priority: rule.priority })}</span>
            {rule.trigger_event && rule.trigger_event !== 'on_import' && (
              <span className="text-xs text-neutral-400 tabular-nums">
                · {rule.trigger_event === 'on_enrichment_complete' ? t('ruleEditor.triggers.onEnrichmentComplete') : t('ruleEditor.triggers.onManualApply')}
              </span>
            )}
            {rule.stop_on_match && (
              <span className="text-xs text-amber-500 tabular-nums">· {t('automationRules.card.stops')}</span>
            )}
            <span className="text-xs text-neutral-400 tabular-nums">{t('automationRules.card.timesApplied', { count: rule.times_applied })}</span>
            {rule.last_evaluated_at && (
              <span className="text-xs text-neutral-400 tabular-nums">
                · {t('automationRules.card.lastEvaluated', { date: new Date(rule.last_evaluated_at).toLocaleDateString(i18n.language) })}
                {rule.last_matched_count != null && (
                  <> ({t('automationRules.card.matches', { count: rule.last_matched_count })})</>
                )}
              </span>
            )}
          </div>
          {isReadonlySystem ? (
            <div className="mt-1 text-sm text-neutral-500 dark:text-neutral-400 italic">
              {readonlyDescription}
            </div>
          ) : rule.branches && rule.branches.length > 0 ? (
            <div className="mt-1 space-y-0.5">
              {rule.branches.map((branch, i) => {
                const condStr = branch.condition_groups?.flatMap(g => g.conditions).map(c =>
                  `${fieldLabel(c.field, t)} ${opLabel(c.op, t)} ${formatConditionValue(c, t, language, unitFormatters.options)}`
                ).join(` ${t('ruleEditor.logic.and')} `) || ''
                const actStr = branch.actions.map(a =>
                  `${actionLabel(a.action, t)}(${formatActionParams(a.action, a.params as Record<string, unknown> | undefined, tags, t, language)})`
                ).join(', ')
                const prefix = branch.type === 'if' ? t('automationRules.summary.if') : branch.type === 'else' ? t('automationRules.summary.else') : t('automationRules.summary.elseIf')
                return (
                  <div key={i} className="text-sm text-neutral-500 dark:text-neutral-400">
                    <span className="font-mono text-xs">
                      {prefix}{condStr ? ` ${condStr}` : ''} → {actStr}
                    </span>
                  </div>
                )
              })}
            </div>
          ) : (
            <>
              <div className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
                <span className="font-mono text-xs">
                  {(() => {
                    const displayConditions = rule.conditions.length > 0
                      ? rule.conditions
                      : (rule.condition_groups ?? []).flatMap(g => g.conditions)
                    return `${t('automationRules.summary.if')} ${displayConditions.map((c) => `${fieldLabel(c.field, t)} ${opLabel(c.op, t)} ${formatConditionValue(c, t, language, unitFormatters.options)}`).join(` ${t('ruleEditor.logic.and')} `)}`
                  })()}
                </span>
              </div>
              <div className="mt-0.5 text-sm text-neutral-500 dark:text-neutral-400">
                <span className="font-mono text-xs">
                  {t('automationRules.summary.then')} {rule.actions.map((a) => `${actionLabel(a.action, t)}(${formatActionParams(a.action, a.params as Record<string, unknown> | undefined, tags, t, language)})`).join(', ')}
                </span>
              </div>
            </>
          )}
        </div>

        {!isReadonlySystem && (
          <div className="flex items-center gap-3 ml-4">
            {/* Toggle */}
            <Toggle
              checked={rule.is_active}
              onChange={(active) => onToggle(active)}
              aria-label={t('automationRules.card.toggleActive', { name: ruleLabel })}
            />
            {/* Backfill button: for all non-readonly system rules */}
            {!isReadonlySystem && onBackfill && (
              <button
                onClick={handleBackfillPreview}
                disabled={backfillLoading || backfillApplying}
                className="text-xs px-2 py-1 rounded border border-neutral-300 dark:border-neutral-600 text-neutral-500 dark:text-neutral-400 hover:border-orange-400 hover:text-orange-600 dark:hover:text-orange-400 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
                title={t('automationRules.card.applyToAllTitle')}
              >
                {backfillLoading ? t('automationRules.actions.checking') : t('automationRules.card.backfill')}
              </button>
            )}
            {/* Delete button: disabled with tooltip for system rules, active for user rules */}
            <button
              onClick={rule.is_system ? undefined : onDelete}
              disabled={rule.is_system}
              className="text-neutral-400 hover:text-rose-500 cursor-pointer transition-colors disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:text-neutral-400"
              title={rule.is_system ? t('automationRules.card.systemRulesCannotBeDeleted') : t('automationRules.card.deleteRule')}
            >
              <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4zM8.58 7.72a.75.75 0 00-1.5.06l.3 7.5a.75.75 0 101.5-.06l-.3-7.5zm4.34.06a.75.75 0 10-1.5-.06l-.3 7.5a.75.75 0 101.5.06l.3-7.5z" clipRule="evenodd" />
              </svg>
            </button>
          </div>
        )}
      </div>

      {/* Backfill inline preview */}
      {backfillPreview !== null && (
        <div className="mt-3 rounded-lg border border-orange-200 dark:border-orange-800 bg-orange-50 dark:bg-orange-900/10 px-3 py-2.5 space-y-2">
          <p className="text-sm text-neutral-700 dark:text-neutral-300">
            {backfillPreview.total_matches === 0
              ? t('automationRules.preview.noMatchesSingle')
              : backfillPreview.total_matches === 1
                ? t('automationRules.preview.oneMatch')
                : t('automationRules.preview.matchCount', { count: backfillPreview.total_matches })}
            {backfillPreview.truncated && t('automationRules.preview.truncated')}
          </p>
          {backfillPreview.matches.length > 0 && (
            <ul className="text-xs text-neutral-600 dark:text-neutral-400 space-y-0.5 max-h-32 overflow-y-auto">
              {backfillPreview.matches.map((m) => (
                <li key={m.activity_id} className="truncate">· {m.activity_title || m.activity_id}</li>
              ))}
            </ul>
          )}
          <div className="flex items-center gap-2 pt-1">
            <button
              onClick={() => setBackfillPreview(null)}
              className="text-xs px-2 py-1 rounded border border-neutral-300 dark:border-neutral-600 text-neutral-500 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
            >
              {t('common.cancel')}
            </button>
            {backfillPreview.total_matches > 0 && (
              <button
                onClick={handleBackfillApply}
                disabled={backfillApplying}
                className="text-xs px-2 py-1 rounded bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
              >
                {backfillApplying ? t('automationRules.actions.starting') : t('automationRules.actions.applyNow')}
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function actionLabel(action: string, t: TFunction): string {
  return hasActionLabel(action, t)
    ? t(`automationRules.summary.actions.${action}`)
    : t('automationRules.summary.unknownAction')
}

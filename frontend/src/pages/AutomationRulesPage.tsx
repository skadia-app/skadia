import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router-dom'
import { fetchRules, createRule, updateRule, deleteRule, applyRules } from '@/api/rules'
import type { AutomationRule } from '@/api/types'
import RuleEditor from '@/components/RuleEditor'

export default function AutomationRulesPage() {
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()

  // URL params: ?new=1&tag_name=foo or ?new=1&tag_id=uuid
  const urlNew = searchParams.get('new') === '1'
  const urlTagName = searchParams.get('tag_name')
  const urlTagId = searchParams.get('tag_id')

  const [creating, setCreating] = useState(urlNew)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [applyResult, setApplyResult] = useState<{ applied: number; errors: number } | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)

  // Build initial rule data from URL params
  const initialRuleData: Partial<AutomationRule> | undefined = urlNew && (urlTagName || urlTagId)
    ? {
        name: urlTagName ? `Tag "${urlTagName}" vergeben` : '',
        actions: [{ action: 'add_tag', params: { tag: urlTagName ?? '' } }],
      }
    : undefined

  const { data: rules, isLoading } = useQuery({
    queryKey: ['automation-rules'],
    queryFn: fetchRules,
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

  const systemRules = rules?.filter(r => r.is_system) ?? []
  const userRules = rules?.filter(r => !r.is_system) ?? []

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Regeln...</div>
  }

  return (
    <div className="space-y-6">
      <Link
        to={urlTagName || urlTagId ? '/workshop/tags' : '/workshop'}
        className="inline-flex items-center gap-1 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
      >
        <span>&#8592;</span> {urlTagName || urlTagId ? 'Tags' : 'Werkstatt'}
      </Link>

      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Automatisierungsregeln</h1>
        <div className="flex items-center gap-3">
          <button
            onClick={() => applyMutation.mutate()}
            disabled={applyMutation.isPending}
            className="px-4 py-2 rounded-lg text-sm font-medium text-accent-600 dark:text-orange-400 border border-orange-300 dark:border-orange-700 hover:bg-orange-50 dark:hover:bg-orange-900/20 disabled:opacity-50 transition-colors"
          >
            {applyMutation.isPending ? 'Wird angewendet...' : 'Regeln anwenden'}
          </button>
          <button
            onClick={() => setCreating(true)}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
          >
            Regel erstellen
          </button>
        </div>
      </div>

      {applyResult && (
        <div className="rounded-lg border border-emerald-300 dark:border-emerald-700 bg-emerald-50 dark:bg-emerald-900/20 px-4 py-3 text-sm">
          <span className="text-emerald-700 dark:text-emerald-400">
            {applyResult.applied} Regel(n) angewendet
            {applyResult.errors > 0 && `, ${applyResult.errors} Fehler`}
          </span>
        </div>
      )}

      {/* Create form */}
      {creating && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">Neue Regel</h2>
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
          <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-2">System-Regeln</h3>
          <div className="space-y-3">
            {systemRules.map((rule) => (
              <div key={rule.id}>
                {editingId === rule.id ? (
                  <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
                    <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">Regel bearbeiten</h2>
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
                    onEdit={() => setEditingId(rule.id)}
                    onToggle={(active) => toggleMutation.mutate({ id: rule.id, is_active: active })}
                    onDelete={() => setDeleteConfirmId(rule.id)}
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
            <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-2">Meine Regeln</h3>
          )}
          <div className="space-y-3">
            {userRules.map((rule) => (
              <div key={rule.id}>
                {editingId === rule.id ? (
                  <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
                    <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">Regel bearbeiten</h2>
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
                    onEdit={() => setEditingId(rule.id)}
                    onToggle={(active) => toggleMutation.mutate({ id: rule.id, is_active: active })}
                    onDelete={() => setDeleteConfirmId(rule.id)}
                  />
                )}
              </div>
            ))}
          </div>
        </div>
      ) : (
        !creating && systemRules.length === 0 && (
          <div className="text-center py-16 text-neutral-400">
            Noch keine Regeln vorhanden.
          </div>
        )
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
              <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Regel loschen?</h3>
              <p className="text-sm text-neutral-600 dark:text-neutral-400">
                Soll die Automatisierungsregel{' '}
                <span className="font-semibold text-neutral-900 dark:text-neutral-100">"{ruleToDelete.name}"</span>{' '}
                wirklich geloscht werden? Diese Aktion kann nicht ruckgangig gemacht werden.
              </p>
              <div className="flex items-center justify-end gap-3">
                <button
                  onClick={() => setDeleteConfirmId(null)}
                  disabled={deleteMutation.isPending}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer disabled:opacity-50"
                >
                  Abbrechen
                </button>
                <button
                  onClick={() => deleteMutation.mutate(deleteConfirmId)}
                  disabled={deleteMutation.isPending}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-rose-500 hover:bg-rose-600 disabled:opacity-50 transition-colors cursor-pointer"
                >
                  {deleteMutation.isPending ? 'Wird geloscht...' : 'Loschen'}
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
  onEdit,
  onToggle,
  onDelete,
}: {
  rule: AutomationRule
  onEdit: () => void
  onToggle: (active: boolean) => void
  onDelete: () => void
}) {
  const isReadonlySystem = rule.system_type === 'ebike_sync' || rule.system_type === 'virtual_sync'

  const readonlyDescriptions: Record<string, string> = {
    ebike_sync: 'Wird automatisch aus Gerätedaten erkannt. Diese Regel kann nicht bearbeitet werden.',
    virtual_sync: 'Wird automatisch über die Datenbankspalte is_virtual synchronisiert. Diese Regel kann nicht bearbeitet werden.',
  }
  const readonlyDescription = rule.system_type ? (readonlyDescriptions[rule.system_type] ?? 'System-Regel. Diese Regel kann nicht bearbeitet werden.') : ''

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
      <div className="flex items-start justify-between">
        <div
          className={`flex-1 min-w-0 ${!isReadonlySystem ? 'cursor-pointer' : ''}`}
          onClick={!isReadonlySystem ? onEdit : undefined}
        >
          <div className="flex items-center gap-3">
            <h3 className="text-base font-medium text-neutral-900 dark:text-neutral-100">{rule.name}</h3>
            {rule.is_system && (
              <span className="text-xs bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 px-1.5 py-0.5 rounded font-medium">
                System
              </span>
            )}
            <span className="text-xs text-neutral-400 tabular-nums">Prio {rule.priority}</span>
            <span className="text-xs text-neutral-400 tabular-nums">{rule.times_applied}x angewendet</span>
          </div>
          {isReadonlySystem ? (
            <div className="mt-1 text-sm text-neutral-500 dark:text-neutral-400 italic">
              {readonlyDescription}
            </div>
          ) : (
            <>
              <div className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
                <span className="font-mono text-xs">
                  WENN {rule.conditions.map((c) => `${c.field} ${c.op} ${JSON.stringify(c.value)}`).join(' UND ')}
                </span>
              </div>
              <div className="mt-0.5 text-sm text-neutral-500 dark:text-neutral-400">
                <span className="font-mono text-xs">
                  DANN {rule.actions.map((a) => `${actionLabel(a.action)}(${JSON.stringify(a.params)})`).join(', ')}
                </span>
              </div>
            </>
          )}
        </div>

        {!isReadonlySystem && (
          <div className="flex items-center gap-3 ml-4">
            {/* Toggle */}
            <button
              onClick={() => onToggle(!rule.is_active)}
              className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors flex-shrink-0"
              style={{ backgroundColor: rule.is_active ? '#f97316' : '#d4d4d4' }}
              role="switch"
              aria-checked={rule.is_active}
            >
              <span
                className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                  rule.is_active ? 'translate-x-6' : 'translate-x-1'
                }`}
              />
            </button>
            {/* Delete button: only for non-system rules */}
            {!rule.is_system && (
              <button
                onClick={onDelete}
                className="text-neutral-400 hover:text-rose-500 transition-colors"
                title="Regel löschen"
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4zM8.58 7.72a.75.75 0 00-1.5.06l.3 7.5a.75.75 0 101.5-.06l-.3-7.5zm4.34.06a.75.75 0 10-1.5-.06l-.3 7.5a.75.75 0 101.5.06l.3-7.5z" clipRule="evenodd" />
                </svg>
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function actionLabel(action: string): string {
  const labels: Record<string, string> = {
    set_gear: 'Ausrüstung',
    add_tag: 'Tag',
    set_visibility: 'Sichtbarkeit',
  }
  return labels[action] ?? action
}

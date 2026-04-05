import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router-dom'
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
import type { AutomationRule } from '@/api/types'
import type { ApplyResult, DryRunAllResult, DryRunSingleResult } from '@/api/rules'
import RuleEditor from '@/components/RuleEditor'
import Toggle from '@/components/Toggle'
import { useToast } from '@/lib/toast'

interface DryRunMatch {
  activity_id: string
  activity_title: string
  started_at?: string
  sport_type?: string
  distance_m?: number
  rules: { rule_name: string; action: string }[]
}

const FIELD_LABELS: Record<string, string> = {
  sport_type: 'Sportart', sub_type: 'Sub-Typ', source: 'Quelle',
  intensity: 'Intensität', is_indoor: 'Indoor', has_gps: 'GPS',
  visibility: 'Sichtbarkeit', title: 'Titel', description: 'Beschreibung',
  elapsed_time_s: 'Gesamtzeit', moving_time_s: 'Bewegungszeit',
  weekday: 'Wochentag', month: 'Monat', season: 'Jahreszeit',
  time_of_day: 'Tageszeit', distance_m: 'Distanz',
  avg_speed_ms: 'Ø Geschwindigkeit', max_speed_ms: 'Max. Geschwindigkeit',
  elevation_gain_m: 'Höhenmeter', elevation_loss_m: 'Höhenverlust',
  max_elevation_m: 'Max. Höhe', min_elevation_m: 'Min. Höhe',
  avg_heart_rate: 'Ø HF', max_heart_rate: 'Max. HF', min_heart_rate: 'Min. HF',
  avg_cadence: 'Ø Kadenz', max_cadence: 'Max. Kadenz',
  avg_power_w: 'Ø Leistung', max_power_w: 'Max. Leistung',
  normalized_power_w: 'NP', tss: 'TSS', trimp: 'TRIMP',
  avg_temperature_c: 'Ø Temperatur', min_temperature_c: 'Min. Temperatur',
  max_temperature_c: 'Max. Temperatur',
  start_city: 'Startort', start_state: 'Region', start_country: 'Land',
  end_city: 'Endort', gear_id: 'Ausrüstung',
  has_tag: 'Tag', photo_count: 'Fotos', detected_lap_count: 'Runden',
  hide_map: 'Karte verborgen', sun_elevation: 'Sonnenstand',
  start_end_distance_m: 'Start-End-Abstand',
  has_lift: 'Lift', lift_count: 'Lifte',
  climb_count: 'Anstiege', climb_category_max: 'Härteste Kategorie',
  has_summit: 'Gipfel', summit_count: 'Anz. Gipfel',
  has_pass: 'Pass', border_crossing_count: 'Grenzquerungen',
  has_gastronomy_stop: 'Einkehr',
  has_structured_intervals: 'Intervalle', has_hc_climb: 'HC-Anstieg',
  surface_paved_pct: 'Asphalt %', surface_gravel_pct: 'Gravel %',
  surface_trail_pct: 'Trail %',
  swim_stroke: 'Schwimmstil', pool_length_m: 'Bahnlänge',
  max_depth_m: 'Max. Tiefe', jump_count: 'Sprünge',
  weather_temp_c: 'Wetter-Temp.', weather_condition: 'Wetterlage',
}

const OP_DISPLAY: Record<string, string> = {
  eq: '=', neq: '≠', gt: '>', lt: '<', gte: '≥', lte: '≤',
  contains: 'enthält', not_contains: 'enthält nicht',
  starts_with: 'beginnt mit', ends_with: 'endet mit',
  between: 'zwischen', in: 'in',
}

// Sport type value → German label
const SPORT_LABELS: Record<string, string> = {
  running: 'Laufen', cycling: 'Radfahren', hiking: 'Wandern',
  trail_running: 'Trailrunning', swimming: 'Schwimmen', walking: 'Gehen',
  mountaineering: 'Bergsteigen', skiing: 'Skifahren', snowboarding: 'Snowboard',
  strength_training: 'Krafttraining', yoga: 'Yoga', diving: 'Tauchen',
  indoor_cycling: 'Indoor-Radfahren', treadmill_running: 'Laufband',
  lap_swimming: 'Bahnschwimmen', open_water_swimming: 'Freiwasserschwimmen',
  mountain_biking: 'Mountainbike', gravel_cycling: 'Gravel',
}

function formatConditionValue(c: { field: string; op: string; value: unknown }): string {
  if (typeof c.value === 'boolean') return c.value ? 'Ja' : 'Nein'
  if (c.value === null || c.value === undefined) return ''
  const v = String(c.value)
  // Sport type → German label
  if (c.field === 'sport_type') return SPORT_LABELS[v] || v
  // Distance in meters → formatted with unit
  if (c.field === 'distance_m' || c.field === 'start_end_distance_m' || c.field === 'net_distance_m') {
    const n = Number(v)
    if (!isNaN(n)) return n >= 10000 ? `${(n / 1000).toFixed(1)} km` : `${n} m`
  }
  // Elevation → with unit
  if (c.field.includes('elevation') || c.field.includes('ascent') || c.field.includes('descent')) {
    const n = Number(v)
    if (!isNaN(n)) return `${n} m`
  }
  // Between: "min,max" → formatted
  if (c.op === 'between' && v.includes(',')) {
    const [lo, hi] = v.split(',', 2)
    const loN = Number(lo), hiN = Number(hi)
    if (c.field === 'distance_m' && !isNaN(loN) && !isNaN(hiN)) {
      return `${(loN / 1000).toFixed(1)}–${(hiN / 1000).toFixed(1)} km`
    }
    return `${lo}–${hi}`
  }
  return v
}

function formatActionParams(action: string, params: Record<string, unknown> | undefined, tags?: { id: string; name: string }[]): string {
  if (!params) return ''
  if ((action === 'add_tag' || action === 'remove_tag') && params.tag_id) {
    const tag = tags?.find((t) => t.id === params.tag_id)
    return tag ? `"${tag.name}"` : String(params.tag_id)
  }
  if (action === 'set_field' && params.field) {
    const label = FIELD_LABELS[String(params.field)] || String(params.field)
    return `${label} = ${params.value}`
  }
  if (action === 'set_sport_type') return String(params.sport_type || '')
  if (action === 'set_name') return `"${params.template || ''}"`
  if (action === 'set_visibility') return String(params.visibility || '')
  if (action === 'set_intensity') return String(params.intensity || '')
  if (action === 'set_indoor') return params.indoor ? 'Ja' : 'Nein'
  if (action === 'set_gear' && params.gear_id) return String(params.gear_id)
  return JSON.stringify(params)
}

export default function AutomationRulesPage() {
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
  const [showSystemRules, setShowSystemRules] = useState(false)
  const [search, setSearch] = useState('')
  const [dryRunResults, setDryRunResults] = useState<DryRunAllResult | null>(null)
  const [dryRunLoading, setDryRunLoading] = useState(false)

  // Build initial rule data from URL params
  const initialRuleData: Partial<AutomationRule> | undefined = urlNew && (urlTagName || urlTagId)
    ? {
        name: urlTagName ? `Tag "${urlTagName}" vergeben` : '',
        actions: [{ action: 'add_tag', params: { tag: urlTagName ?? '' } }],
      }
    : undefined

  async function handleDryRunAll() {
    setDryRunLoading(true)
    try {
      const result = await dryRunAllRules()
      setDryRunResults(result)
    } catch {
      addToast({ type: 'error', title: 'Vorschau fehlgeschlagen', message: 'Regeln konnten nicht ausgewertet werden', duration: 0 })
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

  const syncSystemTypes = ['electric_sync', 'virtual_sync', 'indoor_sync']

  const visibleRules = (rules ?? []).filter(r => {
    if (!showSystemRules && r.is_system && r.system_type && syncSystemTypes.includes(r.system_type)) {
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
          <label className="flex items-center gap-2 text-sm text-neutral-500 dark:text-neutral-400 cursor-pointer select-none">
            <input
              type="checkbox"
              checked={showSystemRules}
              onChange={e => setShowSystemRules(e.target.checked)}
              className="rounded"
            />
            System-Regeln anzeigen
          </label>
          <button
            onClick={handleDryRunAll}
            disabled={dryRunLoading || applyMutation.isPending}
            className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 border border-neutral-300 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            {dryRunLoading ? 'Prüfe...' : 'Vorschau'}
          </button>
          <button
            onClick={() => applyMutation.mutate()}
            disabled={applyMutation.isPending || dryRunLoading}
            className="px-4 py-2 rounded-lg text-sm font-medium text-accent-600 dark:text-orange-400 border border-orange-300 dark:border-orange-700 hover:bg-orange-50 dark:hover:bg-orange-900/20 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            {applyMutation.isPending ? 'Wird angewendet...' : 'Regeln anwenden'}
          </button>
          <button
            onClick={() => setCreating(true)}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 cursor-pointer transition-colors"
          >
            Regel erstellen
          </button>
        </div>
      </div>

      {applyResult && (
        <div className="rounded-lg border border-emerald-300 dark:border-emerald-700 bg-emerald-50 dark:bg-emerald-900/20 px-4 py-3 text-sm">
          <span className="text-emerald-700 dark:text-emerald-400">
            {applyResult.job_id
              ? 'Regelanwendung gestartet'
              : applyResult.applied != null
                ? `${applyResult.applied} Regel(n) angewendet`
                : 'Regelanwendung abgeschlossen'}
          </span>
        </div>
      )}

      {/* Search */}
      <input
        type="text"
        placeholder="Regeln durchsuchen..."
        value={search}
        onChange={e => setSearch(e.target.value)}
        className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 placeholder:text-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
      />

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
            Noch keine Regeln vorhanden.
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
                Vorschau — Regeln anwenden
              </h3>
              <button
                onClick={() => setDryRunResults(null)}
                className="text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 cursor-pointer transition-colors"
                title="Schließen"
              >
                <svg className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                </svg>
              </button>
            </div>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">
              {dryRunResults.total_matches === 0
                ? 'Keine Aktivitäten würden von aktiven Regeln betroffen.'
                : dryRunResults.total_matches === 1
                  ? '1 Aktivität würde betroffen sein.'
                  : `${dryRunResults.total_matches} Aktivitäten würden betroffen sein.`}
              {dryRunResults.truncated && ' (Vorschau zeigt die ersten 50 Treffer.)'}
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
                        <span className="text-neutral-400 font-normal ml-2">{m.sport_type}</span>
                      )}
                      {m.distance_m != null && m.distance_m > 0 && (
                        <span className="text-neutral-400 font-normal ml-2">{(m.distance_m / 1000).toFixed(1)} km</span>
                      )}
                    </a>
                    <div className="mt-0.5 flex flex-wrap gap-1.5">
                      {m.rules.map((rm, i) => (
                        <span key={i} className="inline-flex items-center gap-1 text-xs bg-orange-50 dark:bg-orange-900/20 text-orange-700 dark:text-orange-400 border border-orange-200 dark:border-orange-800 rounded px-1.5 py-0.5">
                          <span className="font-medium">{rm.rule_name}</span>
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
                Abbrechen
              </button>
              <button
                onClick={() => { setDryRunResults(null); applyMutation.mutate() }}
                disabled={applyMutation.isPending}
                className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
              >
                Regeln anwenden
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
              <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Regel löschen?</h3>
              <p className="text-sm text-neutral-600 dark:text-neutral-400">
                Soll die Automatisierungsregel{' '}
                <span className="font-semibold text-neutral-900 dark:text-neutral-100">"{ruleToDelete.name}"</span>{' '}
                wirklich gelöscht werden? Diese Aktion kann nicht rückgängig gemacht werden.
              </p>
              <div className="flex items-center justify-end gap-3">
                <button
                  onClick={() => setDeleteConfirmId(null)}
                  disabled={deleteMutation.isPending}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Abbrechen
                </button>
                <button
                  onClick={() => deleteMutation.mutate(deleteConfirmId)}
                  disabled={deleteMutation.isPending}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-rose-500 hover:bg-rose-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
                >
                  {deleteMutation.isPending ? 'Wird gelöscht...' : 'Löschen'}
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
  tags?: { id: string; name: string }[]
  onEdit: () => void
  onToggle: (active: boolean) => void
  onDelete: () => void
  onBackfill?: ReturnType<typeof useToast>['addToast']
}) {
  const [backfillPreview, setBackfillPreview] = useState<DryRunSingleResult | null>(null)
  const [backfillLoading, setBackfillLoading] = useState(false)
  const [backfillApplying, setBackfillApplying] = useState(false)

  async function handleBackfillPreview() {
    if (!onBackfill) return
    setBackfillLoading(true)
    try {
      const result = await dryRunSingleRule(rule.id)
      setBackfillPreview(result)
    } catch {
      onBackfill({ type: 'error', title: 'Vorschau fehlgeschlagen', message: 'Regel konnte nicht ausgewertet werden', duration: 0 })
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
      onBackfill({ type: 'success', title: `Regel "${rule.name}" wird angewendet` })
    } catch {
      onBackfill({ type: 'error', title: 'Backfill fehlgeschlagen', message: 'Regel konnte nicht angewendet werden', duration: 0 })
    } finally {
      setBackfillApplying(false)
    }
  }

  const isReadonlySystem = rule.system_type === 'electric_sync' || rule.system_type === 'virtual_sync' || rule.system_type === 'indoor_sync'

  const readonlyDescriptions: Record<string, string> = {
    electric_sync: 'Wird automatisch aus Gerätedaten erkannt. Diese Regel kann nicht bearbeitet werden.',
    virtual_sync: 'Wird automatisch über die Datenbankspalte is_virtual synchronisiert. Diese Regel kann nicht bearbeitet werden.',
    indoor_sync: 'Wird automatisch aus der Sport-Taxonomy erkannt. Diese Regel kann nicht bearbeitet werden.',
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
            {rule.is_system && isReadonlySystem && (
              <span className="text-xs bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 px-1.5 py-0.5 rounded font-medium">
                System
              </span>
            )}
            <span className="text-xs text-neutral-400 tabular-nums">Prio {rule.priority}</span>
            {rule.trigger_event && rule.trigger_event !== 'on_import' && (
              <span className="text-xs text-neutral-400 tabular-nums">
                · {rule.trigger_event === 'on_enrichment_complete' ? 'Nach Enrichment' : 'Nur manuell'}
              </span>
            )}
            {rule.stop_on_match && (
              <span className="text-xs text-amber-500 tabular-nums">· Stoppt</span>
            )}
            <span className="text-xs text-neutral-400 tabular-nums">{rule.times_applied}x angewendet</span>
            {rule.last_evaluated_at && (
              <span className="text-xs text-neutral-400 tabular-nums">
                · Zuletzt: {new Date(rule.last_evaluated_at).toLocaleDateString('de-DE')}
                {rule.last_matched_count != null && (
                  <> ({rule.last_matched_count} Treffer)</>
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
                  `${FIELD_LABELS[c.field] || c.field} ${OP_DISPLAY[c.op] || c.op} ${formatConditionValue(c)}`
                ).join(' UND ') || ''
                const actStr = branch.actions.map(a =>
                  `${actionLabel(a.action)}(${formatActionParams(a.action, a.params as Record<string, unknown> | undefined, tags)})`
                ).join(', ')
                const prefix = branch.type === 'if' ? 'WENN' : branch.type === 'else' ? 'SONST' : 'SONST WENN'
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
                    return `WENN ${displayConditions.map((c) => `${FIELD_LABELS[c.field] || c.field} ${OP_DISPLAY[c.op] || c.op} ${formatConditionValue(c)}`).join(' UND ')}`
                  })()}
                </span>
              </div>
              <div className="mt-0.5 text-sm text-neutral-500 dark:text-neutral-400">
                <span className="font-mono text-xs">
                  DANN {rule.actions.map((a) => `${actionLabel(a.action)}(${formatActionParams(a.action, a.params as Record<string, unknown> | undefined, tags)})`).join(', ')}
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
              aria-label={`${rule.name} aktivieren`}
            />
            {/* Backfill button: for all non-readonly system rules */}
            {!isReadonlySystem && onBackfill && (
              <button
                onClick={handleBackfillPreview}
                disabled={backfillLoading || backfillApplying}
                className="text-xs px-2 py-1 rounded border border-neutral-300 dark:border-neutral-600 text-neutral-500 dark:text-neutral-400 hover:border-orange-400 hover:text-orange-600 dark:hover:text-orange-400 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
                title="Regel auf alle Aktivitäten anwenden"
              >
                {backfillLoading ? 'Prüfe...' : 'Backfill'}
              </button>
            )}
            {/* Delete button: only for non-system rules; spacer for alignment otherwise */}
            {!rule.is_system ? (
              <button
                onClick={onDelete}
                className="text-neutral-400 hover:text-rose-500 cursor-pointer transition-colors"
                title="Regel löschen"
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.519.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4zM8.58 7.72a.75.75 0 00-1.5.06l.3 7.5a.75.75 0 101.5-.06l-.3-7.5zm4.34.06a.75.75 0 10-1.5-.06l-.3 7.5a.75.75 0 101.5.06l.3-7.5z" clipRule="evenodd" />
                </svg>
              </button>
            ) : (
              <div className="w-4" />
            )}
          </div>
        )}
      </div>

      {/* Backfill inline preview */}
      {backfillPreview !== null && (
        <div className="mt-3 rounded-lg border border-orange-200 dark:border-orange-800 bg-orange-50 dark:bg-orange-900/10 px-3 py-2.5 space-y-2">
          <p className="text-sm text-neutral-700 dark:text-neutral-300">
            {backfillPreview.total_matches === 0
              ? 'Keine Aktivitäten würden von dieser Regel betroffen.'
              : backfillPreview.total_matches === 1
                ? '1 Aktivität würde betroffen sein.'
                : `${backfillPreview.total_matches} Aktivitäten würden betroffen sein.`}
            {backfillPreview.truncated && ' (Vorschau zeigt die ersten 50 Treffer.)'}
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
              Abbrechen
            </button>
            {backfillPreview.total_matches > 0 && (
              <button
                onClick={handleBackfillApply}
                disabled={backfillApplying}
                className="text-xs px-2 py-1 rounded bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
              >
                {backfillApplying ? 'Wird gestartet...' : 'Jetzt anwenden'}
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function actionLabel(action: string): string {
  const labels: Record<string, string> = {
    set_gear: 'Ausrüstung',
    add_tag: 'Tag',
    remove_tag: 'Tag entfernen',
    set_visibility: 'Sichtbarkeit',
    set_sport_type: 'Sportart',
    set_name: 'Titel',
    set_intensity: 'Intensität',
    set_indoor: 'Indoor',
    set_field: 'Feld setzen',
  }
  return labels[action] ?? action
}

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchGear } from '@/api/gear'
import type { AutomationRule, RuleCondition, RuleAction } from '@/api/types'

const CONDITION_FIELDS = [
  { value: 'sport_type', label: 'Sportart', type: 'text' },
  { value: 'distance_m', label: 'Distanz (m)', type: 'number' },
  { value: 'elapsed_time_s', label: 'Dauer (s)', type: 'number' },
  { value: 'is_indoor', label: 'Indoor', type: 'text' },
  { value: 'has_gps', label: 'Hat GPS', type: 'text' },
  { value: 'intensity', label: 'Intensität', type: 'text' },
  { value: 'time_of_day', label: 'Tageszeit', type: 'text' },
]

const OPERATORS = [
  { value: 'eq', label: '=' },
  { value: 'neq', label: '!=' },
  { value: 'gt', label: '>' },
  { value: 'lt', label: '<' },
  { value: 'gte', label: '>=' },
  { value: 'lte', label: '<=' },
  { value: 'contains', label: 'enthält' },
  { value: 'in', label: 'in' },
]

const ACTION_TYPES = [
  { value: 'set_gear', label: 'Ausrüstung zuweisen' },
  { value: 'add_tag', label: 'Tag hinzufügen' },
  { value: 'set_visibility', label: 'Sichtbarkeit setzen' },
]

const VISIBILITY_OPTIONS = [
  { value: 'private', label: 'Privat' },
  { value: 'bekannte', label: 'Bekannte' },
  { value: 'instanz', label: 'Instanz' },
  { value: 'federation', label: 'Federation' },
  { value: 'public', label: 'Öffentlich' },
]

interface RuleEditorProps {
  rule?: AutomationRule | null
  onSave: (data: Partial<AutomationRule>) => void
  onCancel: () => void
  isSaving?: boolean
}

export default function RuleEditor({ rule, onSave, onCancel, isSaving }: RuleEditorProps) {
  const [name, setName] = useState(rule?.name ?? '')
  const [priority, setPriority] = useState(rule?.priority ?? 0)
  const [conditions, setConditions] = useState<RuleCondition[]>(rule?.conditions ?? [])
  const [actions, setActions] = useState<RuleAction[]>(rule?.actions ?? [])

  const { data: gearList } = useQuery({
    queryKey: ['gear-list'],
    queryFn: () => fetchGear(false),
  })

  function addCondition() {
    setConditions([...conditions, { field: 'sport_type', op: 'eq', value: '' }])
  }

  function removeCondition(idx: number) {
    setConditions(conditions.filter((_, i) => i !== idx))
  }

  function updateCondition(idx: number, patch: Partial<RuleCondition>) {
    setConditions(conditions.map((c, i) => (i === idx ? { ...c, ...patch } : c)))
  }

  function addAction() {
    setActions([...actions, { action: 'add_tag', params: {} }])
  }

  function removeAction(idx: number) {
    setActions(actions.filter((_, i) => i !== idx))
  }

  function updateAction(idx: number, action: string, params: Record<string, any>) {
    setActions(actions.map((a, i) => (i === idx ? { action, params } : a)))
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim() || conditions.length === 0 || actions.length === 0) return
    onSave({
      name: name.trim(),
      priority,
      conditions,
      actions,
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {/* Name & Priority */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="sm:col-span-2">
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Name *
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-orange-500 focus:border-transparent"
            placeholder="z.B. MTB-Rides automatisch taggen"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Priorität
          </label>
          <input
            type="number"
            value={priority}
            onChange={(e) => setPriority(Number(e.target.value))}
            min={0}
            className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-orange-500 focus:border-transparent"
          />
        </div>
      </div>

      {/* Conditions */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Bedingungen *</h3>
          <button
            type="button"
            onClick={addCondition}
            className="text-sm text-orange-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300"
          >
            + Bedingung
          </button>
        </div>
        {conditions.length === 0 && (
          <p className="text-sm text-neutral-400 py-2">Mindestens eine Bedingung erforderlich.</p>
        )}
        <div className="space-y-2">
          {conditions.map((cond, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <select
                value={cond.field}
                onChange={(e) => updateCondition(idx, { field: e.target.value })}
                className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
              >
                {CONDITION_FIELDS.map((f) => (
                  <option key={f.value} value={f.value}>{f.label}</option>
                ))}
              </select>
              <select
                value={cond.op}
                onChange={(e) => updateCondition(idx, { op: e.target.value })}
                className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
              >
                {OPERATORS.map((op) => (
                  <option key={op.value} value={op.value}>{op.label}</option>
                ))}
              </select>
              <input
                type={CONDITION_FIELDS.find((f) => f.value === cond.field)?.type === 'number' ? 'number' : 'text'}
                value={cond.value ?? ''}
                onChange={(e) => {
                  const fieldDef = CONDITION_FIELDS.find((f) => f.value === cond.field)
                  const val = fieldDef?.type === 'number' ? Number(e.target.value) : e.target.value
                  updateCondition(idx, { value: val })
                }}
                placeholder="Wert"
                className="flex-1 min-w-0 border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
              />
              <button
                type="button"
                onClick={() => removeCondition(idx)}
                className="text-neutral-400 hover:text-rose-500 transition-colors"
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* Actions */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Aktionen *</h3>
          <button
            type="button"
            onClick={addAction}
            className="text-sm text-orange-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300"
          >
            + Aktion
          </button>
        </div>
        {actions.length === 0 && (
          <p className="text-sm text-neutral-400 py-2">Mindestens eine Aktion erforderlich.</p>
        )}
        <div className="space-y-2">
          {actions.map((act, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <select
                value={act.action}
                onChange={(e) => updateAction(idx, e.target.value, {})}
                className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
              >
                {ACTION_TYPES.map((a) => (
                  <option key={a.value} value={a.value}>{a.label}</option>
                ))}
              </select>
              <ActionParams
                action={act.action}
                params={act.params}
                gearList={gearList ?? []}
                onChange={(params) => updateAction(idx, act.action, params)}
              />
              <button
                type="button"
                onClick={() => removeAction(idx)}
                className="text-neutral-400 hover:text-rose-500 transition-colors"
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* Submit */}
      <div className="flex gap-3 justify-end pt-2">
        <button
          type="button"
          onClick={onCancel}
          className="px-4 py-2 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors"
        >
          Abbrechen
        </button>
        <button
          type="submit"
          disabled={isSaving || !name.trim() || conditions.length === 0 || actions.length === 0}
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}

function ActionParams({
  action,
  params,
  gearList,
  onChange,
}: {
  action: string
  params: Record<string, any>
  gearList: any[]
  onChange: (params: Record<string, any>) => void
}) {
  if (action === 'set_gear') {
    return (
      <select
        value={params.gear_id ?? ''}
        onChange={(e) => onChange({ gear_id: e.target.value })}
        className="flex-1 min-w-0 border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
      >
        <option value="">Ausrüstung wählen...</option>
        {gearList.filter((g) => !g.is_retired).map((g) => (
          <option key={g.id} value={g.id}>{g.name}</option>
        ))}
      </select>
    )
  }

  if (action === 'add_tag') {
    return (
      <input
        type="text"
        value={params.tag ?? ''}
        onChange={(e) => onChange({ tag: e.target.value })}
        placeholder="Tag-Name"
        className="flex-1 min-w-0 border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
      />
    )
  }

  if (action === 'set_visibility') {
    return (
      <select
        value={params.visibility ?? ''}
        onChange={(e) => onChange({ visibility: e.target.value })}
        className="flex-1 min-w-0 border border-neutral-300 dark:border-neutral-700 rounded-lg px-2 py-1.5 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
      >
        <option value="">Sichtbarkeit wählen...</option>
        {VISIBILITY_OPTIONS.map(({ value, label }) => (
          <option key={value} value={value}>{label}</option>
        ))}
      </select>
    )
  }

  return null
}

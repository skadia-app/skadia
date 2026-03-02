import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { fetchTags, createTag, updateTag, deleteTag } from '@/api/tags'
import type { Tag, TagCondition } from '@/api/tags'

const colorPalette = [
  { color: '#f97316', text: '#ffffff', label: 'Orange' },
  { color: '#3b82f6', text: '#ffffff', label: 'Blau' },
  { color: '#10b981', text: '#ffffff', label: 'Grün' },
  { color: '#8b5cf6', text: '#ffffff', label: 'Lila' },
  { color: '#f43f5e', text: '#ffffff', label: 'Rosa' },
  { color: '#06b6d4', text: '#ffffff', label: 'Cyan' },
  { color: '#eab308', text: '#1a1a1a', label: 'Gelb' },
  { color: '#6b7280', text: '#ffffff', label: 'Grau' },
]

const CONDITION_FIELDS = [
  { value: 'avg_temperature_c', label: 'Temperatur avg' },
  { value: 'min_temperature_c', label: 'Temperatur min' },
  { value: 'time_of_day', label: 'Tageszeit' },
  { value: 'sport_type', label: 'Sportart' },
  { value: 'distance_m', label: 'Distanz' },
  { value: 'elevation_gain_m', label: 'Höhenmeter' },
  { value: 'avg_speed_ms', label: 'Geschwindigkeit' },
  { value: 'avg_heart_rate', label: 'Puls avg' },
  { value: 'max_heart_rate', label: 'Puls max' },
  { value: 'elapsed_time_s', label: 'Dauer' },
  { value: 'is_round_trip', label: 'Rundtour' },
  { value: 'is_indoor', label: 'Indoor' },
  { value: 'start_city', label: 'Startort' },
  { value: 'start_country', label: 'Land' },
  { value: 'weekday', label: 'Wochentag' },
  { value: 'month', label: 'Monat' },
]

const CONDITION_OPS = [
  { value: 'eq', label: 'gleich' },
  { value: 'neq', label: 'ungleich' },
  { value: 'gt', label: 'größer' },
  { value: 'lt', label: 'kleiner' },
  { value: 'gte', label: '>=' },
  { value: 'lte', label: '<=' },
  { value: 'between', label: 'zwischen' },
  { value: 'contains', label: 'enthält' },
  { value: 'in', label: 'in' },
]

export default function TagsPage() {
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)

  const { data: tags, isLoading } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })

  const deleteMut = useMutation({
    mutationFn: deleteTag,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      setDeleteConfirmId(null)
    },
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Tags...</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link
            to="/workshop"
            className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
          >
            &#8592; Werkstatt
          </Link>
          <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Tags</h1>
        </div>
        <button
          onClick={() => { setShowForm(true); setEditingId(null) }}
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 transition-colors cursor-pointer"
        >
          Tag erstellen
        </button>
      </div>

      {/* Create / Edit form */}
      {showForm && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">Neuer Tag</h2>
          <TagForm
            onSave={() => { setShowForm(false); setEditingId(null) }}
            onCancel={() => { setShowForm(false); setEditingId(null) }}
          />
        </div>
      )}

      {/* Tags list */}
      {tags && tags.length > 0 ? (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-neutral-50 dark:bg-neutral-900/50 border-b border-neutral-200 dark:border-neutral-800">
                <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Tag</th>
                <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Kategorie</th>
                <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Aktivitäten</th>
                <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Aktionen</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
              {tags.map((tag) => (
                <tr key={tag.id} className="even:bg-neutral-50/50 dark:even:bg-neutral-800/20">
                  <td className="px-4 py-3">
                    {editingId === tag.id ? (
                      <TagForm
                        tag={tag}
                        onSave={() => setEditingId(null)}
                        onCancel={() => setEditingId(null)}
                        inline
                      />
                    ) : (
                      <div className="flex items-center gap-2">
                        <span
                          className="inline-block px-2.5 py-1 rounded-full text-xs font-medium"
                          style={{ backgroundColor: tag.color || '#f3f4f6', color: tag.text_color || '#374151' }}
                        >
                          {tag.name}
                        </span>
                        {tag.is_auto && (
                          <span className="px-1.5 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                            Auto
                          </span>
                        )}
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-3 text-neutral-500 dark:text-neutral-400">
                    {editingId === tag.id ? null : (tag.category || '\u2014')}
                  </td>
                  <td className="px-4 py-3 text-right text-neutral-700 dark:text-neutral-300 tabular-nums">
                    {editingId === tag.id ? null : tag.activity_count}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {editingId !== tag.id && (
                      <div className="flex items-center justify-end gap-2">
                        {tag.is_system && !tag.is_auto ? (
                          <span className="text-xs text-neutral-400">System</span>
                        ) : (
                          <>
                            <button
                              onClick={() => setEditingId(tag.id)}
                              className="text-xs text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100 cursor-pointer"
                            >
                              Bearbeiten
                            </button>
                            {!tag.is_system && (
                              <>
                                {deleteConfirmId === tag.id ? (
                                  <span className="flex items-center gap-1">
                                    <button
                                      onClick={() => deleteMut.mutate(tag.id)}
                                      className="text-xs text-rose-600 hover:text-rose-800 dark:text-rose-400 dark:hover:text-rose-300 font-medium cursor-pointer"
                                    >
                                      Ja, löschen
                                    </button>
                                    <button
                                      onClick={() => setDeleteConfirmId(null)}
                                      className="text-xs text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100 cursor-pointer"
                                    >
                                      Abbrechen
                                    </button>
                                  </span>
                                ) : (
                                  <button
                                    onClick={() => setDeleteConfirmId(tag.id)}
                                    className="text-xs text-rose-500 hover:text-rose-700 dark:text-rose-400 dark:hover:text-rose-300 cursor-pointer"
                                  >
                                    Löschen
                                  </button>
                                )}
                              </>
                            )}
                          </>
                        )}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="text-center py-16 text-neutral-400">
          Noch keine Tags vorhanden.
        </div>
      )}
    </div>
  )
}

function TagForm({ tag, onSave, onCancel, inline }: {
  tag?: Tag
  onSave: () => void
  onCancel: () => void
  inline?: boolean
}) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(tag?.name ?? '')
  const [color, setColor] = useState(tag?.color ?? colorPalette[0].color)
  const [textColor, setTextColor] = useState(tag?.text_color ?? colorPalette[0].text)
  const [description, setDescription] = useState(tag?.description ?? '')
  const [isAuto, setIsAuto] = useState(tag?.is_auto ?? false)
  const [conditions, setConditions] = useState<TagCondition[]>(tag?.conditions ?? [])
  const [conditionsLogic, setConditionsLogic] = useState<'and' | 'or'>(tag?.conditions_logic ?? 'and')

  const createMut = useMutation({
    mutationFn: () => createTag({
      name: name.trim(),
      color,
      text_color: textColor,
      description: description.trim() || undefined,
      is_auto: isAuto,
      conditions: isAuto ? conditions : undefined,
      conditions_logic: isAuto ? conditionsLogic : undefined,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      onSave()
    },
  })

  const updateMut = useMutation({
    mutationFn: () => updateTag(tag!.id, {
      name: name.trim(),
      color,
      text_color: textColor,
      description: description.trim() || undefined,
      is_auto: isAuto,
      conditions: isAuto ? conditions : undefined,
      conditions_logic: isAuto ? conditionsLogic : undefined,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      onSave()
    },
  })

  const isPending = createMut.isPending || updateMut.isPending

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    if (tag) {
      updateMut.mutate()
    } else {
      createMut.mutate()
    }
  }

  function selectColor(c: typeof colorPalette[number]) {
    setColor(c.color)
    setTextColor(c.text)
  }

  function addCondition() {
    setConditions([...conditions, { field: 'avg_temperature_c', op: 'lt', value: '' }])
  }

  function removeCondition(index: number) {
    setConditions(conditions.filter((_, i) => i !== index))
  }

  function updateCondition(index: number, patch: Partial<TagCondition>) {
    setConditions(conditions.map((c, i) => i === index ? { ...c, ...patch } : c))
  }

  if (inline) {
    return (
      <form onSubmit={handleSubmit} className="flex items-center gap-3 flex-wrap">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Tag-Name"
          className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400 w-40"
          required
        />
        <div className="flex items-center gap-1.5">
          {colorPalette.map((c) => (
            <button
              key={c.color}
              type="button"
              onClick={() => selectColor(c)}
              className="w-6 h-6 rounded-full border-2 transition-all cursor-pointer"
              style={{
                backgroundColor: c.color,
                borderColor: color === c.color ? '#f97316' : 'transparent',
                transform: color === c.color ? 'scale(1.2)' : 'scale(1)',
              }}
              title={c.label}
            />
          ))}
        </div>
        <span
          className="px-2.5 py-1 rounded-full text-xs font-medium"
          style={{ backgroundColor: color, color: textColor }}
        >
          {name || 'Vorschau'}
        </span>
        <div className="flex items-center gap-2">
          <button
            type="submit"
            disabled={isPending || !name.trim()}
            className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
          >
            {tag ? 'Speichern' : 'Erstellen'}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
          >
            Abbrechen
          </button>
        </div>
        {(createMut.isError || updateMut.isError) && (
          <p className="text-sm text-rose-500">
            Fehler: {((createMut.error ?? updateMut.error) instanceof Error) ? (createMut.error ?? updateMut.error)?.message : 'Unbekannt'}
          </p>
        )}
      </form>
    )
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {/* Name */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Name</label>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Tag-Name"
          className="w-full px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
          required
        />
      </div>

      {/* Description */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Beschreibung</label>
        <input
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optionale Beschreibung"
          className="w-full px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
        />
      </div>

      {/* Color */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Farbe</label>
        <div className="flex items-center gap-1.5">
          {colorPalette.map((c) => (
            <button
              key={c.color}
              type="button"
              onClick={() => selectColor(c)}
              className="w-6 h-6 rounded-full border-2 transition-all cursor-pointer"
              style={{
                backgroundColor: c.color,
                borderColor: color === c.color ? '#f97316' : 'transparent',
                transform: color === c.color ? 'scale(1.2)' : 'scale(1)',
              }}
              title={c.label}
            />
          ))}
        </div>
      </div>

      {/* Preview */}
      <div className="flex items-center gap-2">
        <span
          className="px-2.5 py-1 rounded-full text-xs font-medium"
          style={{ backgroundColor: color, color: textColor }}
        >
          {name || 'Vorschau'}
        </span>
      </div>

      {/* Tag type toggle */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">Tag-Typ</label>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setIsAuto(false)}
            className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors cursor-pointer ${
              !isAuto
                ? 'bg-orange-500 text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            Einfacher Tag
          </button>
          <button
            type="button"
            onClick={() => setIsAuto(true)}
            className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors cursor-pointer ${
              isAuto
                ? 'bg-orange-500 text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            Autotag
          </button>
        </div>
      </div>

      {/* Autotag condition builder */}
      {isAuto && (
        <div className="space-y-3 border border-neutral-200 dark:border-neutral-800 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Bedingungen</span>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setConditionsLogic('and')}
                className={`px-2 py-0.5 rounded text-xs font-medium cursor-pointer ${
                  conditionsLogic === 'and'
                    ? 'bg-orange-500 text-white'
                    : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500'
                }`}
              >
                UND
              </button>
              <button
                type="button"
                onClick={() => setConditionsLogic('or')}
                className={`px-2 py-0.5 rounded text-xs font-medium cursor-pointer ${
                  conditionsLogic === 'or'
                    ? 'bg-orange-500 text-white'
                    : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500'
                }`}
              >
                ODER
              </button>
            </div>
          </div>

          {conditions.map((cond, i) => (
            <div key={i} className="flex items-center gap-2">
              <select
                value={cond.field}
                onChange={(e) => updateCondition(i, { field: e.target.value })}
                className="flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
              >
                {CONDITION_FIELDS.map(f => (
                  <option key={f.value} value={f.value}>{f.label}</option>
                ))}
              </select>
              <select
                value={cond.op}
                onChange={(e) => updateCondition(i, { op: e.target.value })}
                className="w-28 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
              >
                {CONDITION_OPS.map(o => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
              <input
                value={String(cond.value ?? '')}
                onChange={(e) => updateCondition(i, { value: e.target.value })}
                placeholder="Wert"
                className="flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
              />
              <button
                type="button"
                onClick={() => removeCondition(i)}
                className="text-rose-500 hover:text-rose-700 text-sm cursor-pointer"
              >
                x
              </button>
            </div>
          ))}

          <button
            type="button"
            onClick={addCondition}
            className="text-sm text-orange-500 hover:text-orange-600 cursor-pointer"
          >
            + Bedingung hinzufügen
          </button>

          {/* Preview */}
          {conditions.length > 0 && (
            <div className="text-xs text-neutral-500 dark:text-neutral-400 bg-neutral-50 dark:bg-neutral-800/50 rounded-lg px-3 py-2">
              Tag wird vergeben wenn:{' '}
              {conditions.map((c, i) => {
                const fieldLabel = CONDITION_FIELDS.find(f => f.value === c.field)?.label ?? c.field
                const opLabel = CONDITION_OPS.find(o => o.value === c.op)?.label ?? c.op
                return (
                  <span key={i}>
                    {i > 0 && <span className="font-medium"> {conditionsLogic === 'and' ? 'UND' : 'ODER'} </span>}
                    {fieldLabel} {opLabel} {String(c.value ?? '?')}
                  </span>
                )
              })}
            </div>
          )}
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-2">
        <button
          type="submit"
          disabled={isPending || !name.trim()}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
        >
          {tag ? 'Speichern' : 'Erstellen'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
        >
          Abbrechen
        </button>
      </div>

      {(createMut.isError || updateMut.isError) && (
        <p className="text-sm text-rose-500">
          Fehler: {((createMut.error ?? updateMut.error) instanceof Error) ? (createMut.error ?? updateMut.error)?.message : 'Unbekannt'}
        </p>
      )}
    </form>
  )
}

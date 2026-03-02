import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import {
  fetchGearDetail, updateGear, deleteGear,
  createComponent, updateComponent, deleteComponent,
} from '@/api/gear'
import type { Gear, GearComponent } from '@/api/types'
import { formatDistance, formatDuration } from '@/lib/utils'
import GearForm from '@/components/GearForm'

export default function GearDetailPage() {
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)

  const { data: gear, isLoading } = useQuery({
    queryKey: ['gear-detail', id],
    queryFn: () => fetchGearDetail(id!),
    enabled: !!id,
  })

  const updateMutation = useMutation({
    mutationFn: (data: Partial<Gear>) => updateGear(id!, data),
    onSuccess: (updated) => {
      queryClient.setQueryData(['gear-detail', id], updated)
      queryClient.invalidateQueries({ queryKey: ['gear'] })
      setEditing(false)
    },
  })

  const retireMutation = useMutation({
    mutationFn: () => updateGear(id!, { is_retired: true, retired_at: new Date().toISOString() }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['gear-detail', id], updated)
      queryClient.invalidateQueries({ queryKey: ['gear'] })
    },
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Ausruestung...</div>
  }

  if (!gear) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <span className="text-neutral-400">Ausruestung nicht gefunden</span>
        <Link to="/gear" className="text-sm text-orange-500 hover:text-orange-600">Zurueck zur Uebersicht</Link>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <Link
        to="/gear"
        className="inline-flex items-center gap-1 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
      >
        <span>&#8592;</span> Alle Ausruestung
      </Link>

      {editing ? (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100 mb-4">Bearbeiten</h2>
          <GearForm
            gear={gear}
            onSave={(data) => updateMutation.mutate(data)}
            onCancel={() => setEditing(false)}
            isSaving={updateMutation.isPending}
          />
        </div>
      ) : (
        <>
          {/* Header */}
          <div className="flex items-start justify-between">
            <div>
              <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{gear.name}</h1>
              <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
                {gearTypeLabel(gear.type)}
                {gear.sub_type && ` / ${gear.sub_type}`}
                {gear.brand && ` - ${gear.brand}`}
                {gear.model && ` ${gear.model}`}
              </p>
              {gear.purchased_at && (
                <p className="text-xs text-neutral-400 mt-1">
                  Gekauft: {format(new Date(gear.purchased_at), 'dd. MMMM yyyy', { locale: de })}
                </p>
              )}
              {gear.is_retired && (
                <span className="inline-block mt-2 px-2 py-0.5 rounded text-xs font-medium bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400">
                  Ausgemustert{gear.retired_at ? ` seit ${format(new Date(gear.retired_at), 'dd.MM.yyyy', { locale: de })}` : ''}
                </span>
              )}
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => setEditing(true)}
                className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors"
              >
                Bearbeiten
              </button>
              {!gear.is_retired && (
                <button
                  onClick={() => retireMutation.mutate()}
                  disabled={retireMutation.isPending}
                  className="px-3 py-1.5 rounded-lg text-sm font-medium text-rose-600 dark:text-rose-400 bg-rose-50 dark:bg-rose-900/20 hover:bg-rose-100 dark:hover:bg-rose-900/30 transition-colors"
                >
                  Ausmustern
                </button>
              )}
            </div>
          </div>

          {/* Stats */}
          <div className="grid grid-cols-3 gap-4">
            <StatCard label="Distanz" value={formatDistance(gear.total_distance_m)} />
            <StatCard label="Zeit" value={formatDuration(gear.total_time_s)} />
            <StatCard label="Aktivitaeten" value={String(gear.total_activities)} />
          </div>

          {/* Notes */}
          {gear.notes && (
            <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4">
              <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Notizen</h3>
              <p className="text-sm text-neutral-600 dark:text-neutral-400 whitespace-pre-wrap">{gear.notes}</p>
            </div>
          )}

          {/* Components */}
          <ComponentsList gearId={gear.id} components={gear.components ?? []} />
        </>
      )}
    </div>
  )
}

// ── Stats Card ─────────────────────────────────────────────────────────────────

function StatCard({ label, value }: { label: string; value: string | null }) {
  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
      <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{label}</div>
      <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">{value ?? '---'}</div>
    </div>
  )
}

// ── Components ─────────────────────────────────────────────────────────────────

function ComponentsList({ gearId, components }: { gearId: string; components: GearComponent[] }) {
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)

  const addMutation = useMutation({
    mutationFn: (data: any) => createComponent(gearId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear-detail', gearId] })
      setAdding(false)
    },
  })

  const editMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => updateComponent(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear-detail', gearId] })
      setEditingId(null)
    },
  })

  const removeMutation = useMutation({
    mutationFn: (id: string) => deleteComponent(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear-detail', gearId] })
    },
  })

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Komponenten</h2>
        <button
          onClick={() => setAdding(true)}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-orange-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 transition-colors"
        >
          + Hinzufuegen
        </button>
      </div>

      {adding && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4 mb-4">
          <ComponentForm
            onSave={(data) => addMutation.mutate(data)}
            onCancel={() => setAdding(false)}
            isSaving={addMutation.isPending}
          />
        </div>
      )}

      {components.length > 0 ? (
        <div className="space-y-2">
          {components.map((comp) => (
            <div key={comp.id} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              {editingId === comp.id ? (
                <ComponentForm
                  component={comp}
                  onSave={(data) => editMutation.mutate({ id: comp.id, data })}
                  onCancel={() => setEditingId(null)}
                  isSaving={editMutation.isPending}
                />
              ) : (
                <div className="flex items-center justify-between">
                  <div>
                    <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                      {comp.name}
                      {comp.is_retired && (
                        <span className="ml-2 text-xs text-neutral-400">(ausgemustert)</span>
                      )}
                    </div>
                    <div className="text-xs text-neutral-500 dark:text-neutral-400">
                      {comp.type}
                      {comp.brand && ` - ${comp.brand}`}
                      {comp.model && ` ${comp.model}`}
                      {' | '}
                      {formatDistance(comp.total_distance_m)}
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => setEditingId(comp.id)}
                      className="text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300"
                    >
                      Bearbeiten
                    </button>
                    <button
                      onClick={() => removeMutation.mutate(comp.id)}
                      disabled={removeMutation.isPending}
                      className="text-xs text-rose-500 hover:text-rose-700"
                    >
                      Entfernen
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      ) : (
        !adding && (
          <p className="text-sm text-neutral-400 py-4 text-center">Keine Komponenten vorhanden.</p>
        )
      )}
    </div>
  )
}

// ── Component Form ─────────────────────────────────────────────────────────────

function ComponentForm({
  component,
  onSave,
  onCancel,
  isSaving,
}: {
  component?: GearComponent
  onSave: (data: any) => void
  onCancel: () => void
  isSaving?: boolean
}) {
  const [name, setName] = useState(component?.name ?? '')
  const [type, setType] = useState(component?.type ?? '')
  const [brand, setBrand] = useState(component?.brand ?? '')
  const [model, setModel] = useState(component?.model ?? '')

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim() || !type.trim()) return
    onSave({
      name: name.trim(),
      type: type.trim(),
      brand: brand.trim() || null,
      model: model.trim() || null,
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
          placeholder="Name *"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
        />
        <input
          type="text"
          value={type}
          onChange={(e) => setType(e.target.value)}
          required
          placeholder="Typ * (z.B. Kette, Reifen)"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
        />
        <input
          type="text"
          value={brand}
          onChange={(e) => setBrand(e.target.value)}
          placeholder="Marke"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
        />
        <input
          type="text"
          value={model}
          onChange={(e) => setModel(e.target.value)}
          placeholder="Modell"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
        />
      </div>
      <div className="flex gap-2 justify-end">
        <button
          type="button"
          onClick={onCancel}
          className="px-3 py-1.5 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        >
          Abbrechen
        </button>
        <button
          type="submit"
          disabled={isSaving || !name.trim() || !type.trim()}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 transition-colors"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}

function gearTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    bike: 'Fahrrad',
    shoes: 'Laufschuhe',
    goggles: 'Schwimmbrille',
    ski: 'Ski',
    paddle: 'Paddel',
    other: 'Sonstiges',
  }
  return labels[type] ?? type
}

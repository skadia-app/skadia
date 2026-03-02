import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import {
  fetchGearDetail, updateGear, updateGearStatus,
  createComponent, updateComponent, deleteComponent,
  transferComponent, getComponentHistory, fetchGear,
  getChainRotation, createChainRotation, swapChain, deleteChainRotation,
  listServiceEvents, createServiceEvent, deleteServiceEvent,
  fetchComponentPresets,
  listBikeSetups, createBikeSetup, updateBikeSetup, deleteBikeSetup, activateBikeSetup,
} from '@/api/gear'
import type {
  Gear, GearComponent, GearStatus,
  ComponentAssignment,
  GearServiceEvent, ComponentTypePreset,
  BikeSetup,
} from '@/api/types'
import { formatDistance, formatDuration, formatElevationGain } from '@/lib/utils'
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

  const statusMutation = useMutation({
    mutationFn: (status: GearStatus) => updateGearStatus(id!, status),
    onSuccess: (updated) => {
      queryClient.setQueryData(['gear-detail', id], updated)
      queryClient.invalidateQueries({ queryKey: ['gear'] })
    },
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Ausrüstung...</div>
  }

  if (!gear) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <span className="text-neutral-400">Ausrüstung nicht gefunden</span>
        <Link to="/gear" className="text-sm text-orange-500 hover:text-orange-600">Zurück zur Übersicht</Link>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <Link
        to="/gear"
        className="inline-flex items-center gap-1 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
      >
        <span>&#8592;</span> Alle Ausrüstung
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
              <GearStatusBadge status={gear.status} />
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => setEditing(true)}
                className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
              >
                Bearbeiten
              </button>
              <StatusMenu
                current={gear.status}
                onSelect={(s) => statusMutation.mutate(s)}
                disabled={statusMutation.isPending}
              />
            </div>
          </div>

          {/* Stats */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <StatCard label="Distanz" value={formatDistance(gear.total_distance_m)} />
            <StatCard label="Höhenmeter" value={formatElevationGain(gear.total_elevation_gain_m) ?? '\u2014'} />
            <StatCard label="Zeit" value={formatDuration(gear.total_time_s)} />
            <StatCard label="Aktivitäten" value={String(gear.total_activities)} />
          </div>

          {/* Notes */}
          {gear.notes && (
            <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4">
              <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Notizen</h3>
              <p className="text-sm text-neutral-600 dark:text-neutral-400 whitespace-pre-wrap">{gear.notes}</p>
            </div>
          )}

          {/* Components */}
          <ComponentsList gearId={gear.id} gearType={gear.type} components={gear.components ?? []} />

          {/* Chain Rotation (only for bikes) */}
          {gear.type === 'bike' && (
            <ChainRotationSection gearId={gear.id} components={gear.components ?? []} />
          )}

          {/* Bike Setups (only for bikes) */}
          {gear.type === 'bike' && (
            <BikeSetupsSection gearId={gear.id} />
          )}
        </>
      )}
    </div>
  )
}

// ── Status ──────────────────────────────────────────────────────────────────────

function GearStatusBadge({ status }: { status: GearStatus }) {
  const styles: Record<GearStatus, string> = {
    active: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
    stored: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
    retired: 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400',
  }
  const labels: Record<GearStatus, string> = {
    active: 'Aktiv',
    stored: 'Gelagert',
    retired: 'Ausgemustert',
  }
  return (
    <span className={`inline-block mt-2 px-2 py-0.5 rounded text-xs font-medium ${styles[status]}`}>
      {labels[status]}
    </span>
  )
}

function StatusMenu({ current, onSelect, disabled }: {
  current: GearStatus
  onSelect: (s: GearStatus) => void
  disabled: boolean
}) {
  const [open, setOpen] = useState(false)
  const allOptions: { value: GearStatus; label: string; color: string }[] = [
    { value: 'active', label: 'Aktivieren', color: 'text-emerald-600 dark:text-emerald-400' },
    { value: 'stored', label: 'Lagern', color: 'text-amber-600 dark:text-amber-400' },
    { value: 'retired', label: 'Ausmustern', color: 'text-rose-600 dark:text-rose-400' },
  ]
  const options = allOptions.filter(o => o.value !== current)

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        disabled={disabled}
        className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors cursor-pointer disabled:opacity-50"
      >
        Status ändern
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-10" onClick={() => setOpen(false)} />
          <div className="absolute right-0 top-full mt-1 z-20 bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg py-1 min-w-[140px]">
            {options.map(o => (
              <button
                key={o.value}
                onClick={() => { onSelect(o.value); setOpen(false) }}
                className={`w-full text-left px-3 py-1.5 text-sm hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer ${o.color}`}
              >
                {o.label}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  )
}

// ── Stats Card ──────────────────────────────────────────────────────────────────

function StatCard({ label, value }: { label: string; value: string | null }) {
  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
      <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{label}</div>
      <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">{value ?? '\u2014'}</div>
    </div>
  )
}

// ── Components ──────────────────────────────────────────────────────────────────

function ComponentsList({ gearId, gearType, components }: {
  gearId: string
  gearType: string
  components: GearComponent[]
}) {
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [transferringId, setTransferringId] = useState<string | null>(null)
  const [historyId, setHistoryId] = useState<string | null>(null)
  const [serviceId, setServiceId] = useState<string | null>(null)

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
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-orange-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 transition-colors cursor-pointer"
        >
          + Hinzufügen
        </button>
      </div>

      {adding && (
        <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4 mb-4">
          <ComponentForm
            gearType={gearType}
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
                  gearType={gearType}
                  onSave={(data) => editMutation.mutate({ id: comp.id, data })}
                  onCancel={() => setEditingId(null)}
                  isSaving={editMutation.isPending}
                />
              ) : (
                <div>
                  <div className="flex items-center justify-between">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                          {comp.name}
                        </span>
                        <ComponentStatusBadge status={comp.status} />
                      </div>
                      <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                        {comp.type}
                        {comp.brand && ` - ${comp.brand}`}
                        {comp.model && ` ${comp.model}`}
                        {' | '}
                        {formatDistance(comp.total_distance_m)}
                        {comp.total_elevation_gain_m > 0 && ` | ${formatElevationGain(comp.total_elevation_gain_m)}`}
                        {comp.weight_g != null && ` | ${comp.weight_g}g`}
                      </div>
                      {comp.serial_number && (
                        <div className="text-xs text-neutral-400 mt-0.5">SN: {comp.serial_number}</div>
                      )}
                    </div>
                    <div className="flex items-center gap-2 flex-shrink-0">
                      <button
                        onClick={() => setServiceId(serviceId === comp.id ? null : comp.id)}
                        className="text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
                      >
                        Service
                      </button>
                      <button
                        onClick={() => setHistoryId(historyId === comp.id ? null : comp.id)}
                        className="text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
                      >
                        Historie
                      </button>
                      <button
                        onClick={() => setTransferringId(comp.id)}
                        className="text-xs text-blue-500 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 cursor-pointer"
                      >
                        Transfer
                      </button>
                      <button
                        onClick={() => setEditingId(comp.id)}
                        className="text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
                      >
                        Bearbeiten
                      </button>
                      <button
                        onClick={() => removeMutation.mutate(comp.id)}
                        disabled={removeMutation.isPending}
                        className="text-xs text-rose-500 hover:text-rose-700 cursor-pointer"
                      >
                        Entfernen
                      </button>
                    </div>
                  </div>

                  {transferringId === comp.id && (
                    <TransferDialog
                      componentId={comp.id}
                      currentGearId={gearId}
                      onClose={() => setTransferringId(null)}
                    />
                  )}

                  {historyId === comp.id && (
                    <ComponentHistoryPanel componentId={comp.id} />
                  )}

                  {serviceId === comp.id && (
                    <ServiceEventsPanel componentId={comp.id} />
                  )}
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

function ComponentStatusBadge({ status }: { status: GearStatus }) {
  if (status === 'active') return null
  const styles: Record<string, string> = {
    stored: 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400',
    retired: 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400',
  }
  const labels: Record<string, string> = {
    stored: 'Gelagert',
    retired: 'Ausgemustert',
  }
  return (
    <span className={`px-1.5 py-0.5 rounded text-xs font-medium ${styles[status] ?? ''}`}>
      {labels[status] ?? status}
    </span>
  )
}

// ── Transfer Dialog ─────────────────────────────────────────────────────────────

function TransferDialog({ componentId, currentGearId, onClose }: {
  componentId: string
  currentGearId: string
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [targetId, setTargetId] = useState('')

  const { data: allGear } = useQuery({
    queryKey: ['gear-for-transfer'],
    queryFn: () => fetchGear('active'),
  })

  const mutation = useMutation({
    mutationFn: () => transferComponent(componentId, targetId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear-detail'] })
      queryClient.invalidateQueries({ queryKey: ['gear'] })
      onClose()
    },
  })

  const targets = allGear?.filter(g => g.id !== currentGearId) ?? []

  return (
    <div className="mt-3 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
      <div className="text-sm font-medium text-blue-900 dark:text-blue-300 mb-2">Komponente transferieren</div>
      <div className="flex items-center gap-2">
        <select
          value={targetId}
          onChange={(e) => setTargetId(e.target.value)}
          className="flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
        >
          <option value="">Ziel wählen...</option>
          {targets.map(g => (
            <option key={g.id} value={g.id}>{g.name} ({gearTypeLabel(g.type)})</option>
          ))}
        </select>
        <button
          onClick={() => mutation.mutate()}
          disabled={!targetId || mutation.isPending}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-blue-500 hover:bg-blue-600 disabled:opacity-50 transition-colors cursor-pointer"
        >
          {mutation.isPending ? '...' : 'Transfer'}
        </button>
        <button
          onClick={onClose}
          className="px-3 py-1.5 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
        >
          Abbrechen
        </button>
      </div>
      {mutation.isError && (
        <p className="text-xs text-rose-500 mt-1">Transfer fehlgeschlagen</p>
      )}
    </div>
  )
}

// ── Component History ───────────────────────────────────────────────────────────

function ComponentHistoryPanel({ componentId }: { componentId: string }) {
  const { data: history, isLoading } = useQuery({
    queryKey: ['component-history', componentId],
    queryFn: () => getComponentHistory(componentId),
  })

  if (isLoading) return <div className="text-xs text-neutral-400 mt-2">Lade Historie...</div>

  if (!history || history.length === 0) {
    return <div className="text-xs text-neutral-400 mt-2">Keine Transfer-Historie vorhanden.</div>
  }

  return (
    <div className="mt-3 space-y-1">
      <div className="text-xs font-medium text-neutral-600 dark:text-neutral-300">Transfer-Historie</div>
      {history.map((a: ComponentAssignment) => (
        <div key={a.id} className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
          <span className="w-2 h-2 rounded-full bg-orange-400 flex-shrink-0" />
          <span className="font-medium text-neutral-700 dark:text-neutral-300">{a.gear_name}</span>
          <span>
            {formatDateShort(a.installed_at)}
            {a.removed_at ? ` \u2013 ${formatDateShort(a.removed_at)}` : ' \u2013 jetzt'}
          </span>
        </div>
      ))}
    </div>
  )
}

// ── Service Events ──────────────────────────────────────────────────────────────

function ServiceEventsPanel({ componentId }: { componentId: string }) {
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [performedAt, setPerformedAt] = useState(new Date().toISOString().split('T')[0])
  const [description, setDescription] = useState('')
  const [cost, setCost] = useState('')
  const [notes, setNotes] = useState('')

  const { data: events, isLoading } = useQuery({
    queryKey: ['service-events', componentId],
    queryFn: () => listServiceEvents(componentId),
  })

  const addMut = useMutation({
    mutationFn: () => createServiceEvent(componentId, {
      performed_at: new Date(performedAt).toISOString(),
      description: description.trim() || null,
      cost: cost ? parseFloat(cost) : null,
      notes: notes.trim() || null,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['service-events', componentId] })
      setAdding(false)
      setDescription('')
      setCost('')
      setNotes('')
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteServiceEvent,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['service-events', componentId] })
    },
  })

  return (
    <div className="mt-3 space-y-2">
      <div className="flex items-center justify-between">
        <div className="text-xs font-medium text-neutral-600 dark:text-neutral-300">Service-Historie</div>
        <button
          onClick={() => setAdding(!adding)}
          className="text-xs text-orange-500 hover:text-orange-600 cursor-pointer"
        >
          + Service eintragen
        </button>
      </div>

      {adding && (
        <div className="p-3 bg-neutral-50 dark:bg-neutral-800/50 rounded-lg space-y-2">
          <div className="grid grid-cols-2 gap-2">
            <input
              type="date"
              value={performedAt}
              onChange={(e) => setPerformedAt(e.target.value)}
              className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
            />
            <input
              type="number"
              value={cost}
              onChange={(e) => setCost(e.target.value)}
              placeholder="Kosten (EUR)"
              step="0.01"
              className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
            />
          </div>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Beschreibung"
            className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
          />
          <input
            type="text"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Notizen"
            className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
          />
          <div className="flex gap-2 justify-end">
            <button
              onClick={() => setAdding(false)}
              className="px-2.5 py-1 rounded text-xs text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer"
            >
              Abbrechen
            </button>
            <button
              onClick={() => addMut.mutate()}
              disabled={addMut.isPending}
              className="px-2.5 py-1 rounded text-xs font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 cursor-pointer"
            >
              Speichern
            </button>
          </div>
        </div>
      )}

      {isLoading && <div className="text-xs text-neutral-400">Lade...</div>}

      {events && events.length > 0 ? (
        <div className="space-y-1">
          {events.map((e: GearServiceEvent) => (
            <div key={e.id} className="flex items-center justify-between text-xs">
              <div className="flex items-center gap-2 text-neutral-600 dark:text-neutral-400">
                <span className="text-neutral-700 dark:text-neutral-300 font-medium">
                  {formatDateShort(e.performed_at)}
                </span>
                {e.description && <span>{e.description}</span>}
                {e.cost != null && <span className="text-emerald-600 dark:text-emerald-400">{e.cost.toFixed(2)} EUR</span>}
              </div>
              <button
                onClick={() => deleteMut.mutate(e.id)}
                disabled={deleteMut.isPending}
                className="text-rose-500 hover:text-rose-700 cursor-pointer"
              >
                x
              </button>
            </div>
          ))}
        </div>
      ) : (
        !isLoading && !adding && <div className="text-xs text-neutral-400">Keine Service-Einträge.</div>
      )}
    </div>
  )
}

// ── Chain Rotation ──────────────────────────────────────────────────────────────

function ChainRotationSection({ gearId, components }: {
  gearId: string
  components: GearComponent[]
}) {
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('Ketten-Rotation')
  const [intervalKm, setIntervalKm] = useState('500')
  const [selectedComps, setSelectedComps] = useState<Set<string>>(new Set())

  const { data: rotation } = useQuery({
    queryKey: ['chain-rotation', gearId],
    queryFn: () => getChainRotation(gearId),
  })

  const createMut = useMutation({
    mutationFn: () => createChainRotation(gearId, {
      name,
      swap_interval_km: parseFloat(intervalKm) || 500,
      component_ids: Array.from(selectedComps),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['chain-rotation', gearId] })
      setCreating(false)
    },
  })

  const swapMut = useMutation({
    mutationFn: () => swapChain(gearId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['chain-rotation', gearId] })
      queryClient.invalidateQueries({ queryKey: ['gear-detail', gearId] })
    },
  })

  const deleteMut = useMutation({
    mutationFn: () => deleteChainRotation(rotation!.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['chain-rotation', gearId] })
    },
  })

  function toggleComp(id: string) {
    setSelectedComps(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const chainComponents = components.filter(c =>
    c.type.toLowerCase().includes('kette') || c.type.toLowerCase().includes('chain')
  )

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Ketten-Rotation</h2>
        {!rotation && !creating && (
          <button
            onClick={() => setCreating(true)}
            className="text-sm text-orange-500 hover:text-orange-600 cursor-pointer"
          >
            + Rotation erstellen
          </button>
        )}
      </div>

      {creating && (
        <div className="space-y-3 mb-4">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-neutral-500 mb-1">Name</label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
              />
            </div>
            <div>
              <label className="block text-xs text-neutral-500 mb-1">Wechselintervall (km)</label>
              <input
                type="number"
                value={intervalKm}
                onChange={(e) => setIntervalKm(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs text-neutral-500 mb-1">Ketten auswählen (min. 2)</label>
            {chainComponents.length > 0 ? (
              <div className="space-y-1">
                {chainComponents.map(c => (
                  <label key={c.id} className="flex items-center gap-2 text-sm cursor-pointer">
                    <input
                      type="checkbox"
                      checked={selectedComps.has(c.id)}
                      onChange={() => toggleComp(c.id)}
                      className="rounded"
                    />
                    <span className="text-neutral-700 dark:text-neutral-300">{c.name}</span>
                    <span className="text-xs text-neutral-400">{formatDistance(c.total_distance_m)}</span>
                  </label>
                ))}
              </div>
            ) : (
              <p className="text-xs text-neutral-400">
                Keine Ketten-Komponenten gefunden. Füge zuerst Ketten als Komponenten hinzu.
              </p>
            )}
          </div>

          <div className="flex gap-2 justify-end">
            <button
              onClick={() => setCreating(false)}
              className="px-3 py-1.5 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
            >
              Abbrechen
            </button>
            <button
              onClick={() => createMut.mutate()}
              disabled={selectedComps.size < 2 || createMut.isPending}
              className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 cursor-pointer"
            >
              Rotation erstellen
            </button>
          </div>
        </div>
      )}

      {rotation && (
        <div className="space-y-3">
          <div className="flex items-center justify-between text-sm">
            <span className="text-neutral-700 dark:text-neutral-300 font-medium">{rotation.name}</span>
            <span className="text-xs text-neutral-500">
              Wechsel alle {rotation.swap_interval_km} km
            </span>
          </div>

          {rotation.members && rotation.members.length > 0 && (
            <div className="space-y-1.5">
              {rotation.members.map(m => {
                const distKm = (m.distance_since_swap_m ?? 0) / 1000
                const intKm = rotation.swap_interval_km || 500
                const progress = Math.min((distKm / intKm) * 100, 100)
                const isDue = progress >= 100

                return (
                  <div key={m.id} className="flex items-center gap-3">
                    <div className={`w-2 h-2 rounded-full flex-shrink-0 ${m.is_active ? 'bg-emerald-500' : 'bg-neutral-300 dark:bg-neutral-600'}`} />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center justify-between text-xs">
                        <span className={`font-medium ${m.is_active ? 'text-neutral-900 dark:text-neutral-100' : 'text-neutral-500 dark:text-neutral-400'}`}>
                          {m.component_name}
                          {m.is_active && <span className="ml-1 text-emerald-600 dark:text-emerald-400">(montiert)</span>}
                        </span>
                        <span className={isDue ? 'text-rose-500 font-medium' : 'text-neutral-500'}>
                          {distKm.toFixed(0)} / {intKm} km
                        </span>
                      </div>
                      <div className="w-full h-1 rounded-full bg-neutral-200 dark:bg-neutral-700 mt-0.5">
                        <div
                          className={`h-full rounded-full transition-all ${isDue ? 'bg-rose-500' : 'bg-orange-400'}`}
                          style={{ width: `${progress}%` }}
                        />
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}

          <div className="flex gap-2 justify-end pt-1">
            <button
              onClick={() => deleteMut.mutate()}
              disabled={deleteMut.isPending}
              className="px-3 py-1.5 rounded-lg text-xs text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-900/20 cursor-pointer"
            >
              Rotation löschen
            </button>
            <button
              onClick={() => swapMut.mutate()}
              disabled={swapMut.isPending}
              className="px-3 py-1.5 rounded-lg text-xs font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 cursor-pointer"
            >
              {swapMut.isPending ? 'Wechsle...' : 'Nächste Kette'}
            </button>
          </div>
        </div>
      )}

      {!rotation && !creating && (
        <p className="text-sm text-neutral-400">
          Keine Ketten-Rotation konfiguriert. Erstelle eine, um mehrere Ketten gleichmäßig abzunutzen.
        </p>
      )}
    </div>
  )
}

// ── Component Form ──────────────────────────────────────────────────────────────

function ComponentForm({
  component,
  gearType,
  onSave,
  onCancel,
  isSaving,
}: {
  component?: GearComponent
  gearType: string
  onSave: (data: any) => void
  onCancel: () => void
  isSaving?: boolean
}) {
  const [usePreset, setUsePreset] = useState(!component)
  const [presetId, setPresetId] = useState('')
  const [name, setName] = useState(component?.name ?? '')
  const [type, setType] = useState(component?.type ?? '')
  const [brand, setBrand] = useState(component?.brand ?? '')
  const [model, setModel] = useState(component?.model ?? '')
  const [weightG, setWeightG] = useState(component?.weight_g?.toString() ?? '')
  const [purchasePrice, setPurchasePrice] = useState(component?.purchase_price?.toString() ?? '')
  const [purchaseDate, setPurchaseDate] = useState(component?.purchase_date?.split('T')[0] ?? '')
  const [serialNumber, setSerialNumber] = useState(component?.serial_number ?? '')

  const { data: presets } = useQuery({
    queryKey: ['component-presets', gearType],
    queryFn: () => fetchComponentPresets(gearType),
    enabled: usePreset && !component,
  })

  function selectPreset(id: string) {
    setPresetId(id)
    const preset = presets?.find(p => p.id === id)
    if (preset) {
      setName(preset.name)
      setType(preset.type || preset.category)
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim() || !type.trim()) return
    onSave({
      name: name.trim(),
      type: type.trim(),
      brand: brand.trim() || null,
      model: model.trim() || null,
      weight_g: weightG ? parseInt(weightG) : null,
      purchase_price: purchasePrice ? parseFloat(purchasePrice) : null,
      purchase_date: purchaseDate || null,
      serial_number: serialNumber.trim() || null,
    })
  }

  const groupedPresets = presets?.reduce((acc, p) => {
    const cat = p.category
    if (!acc[cat]) acc[cat] = []
    acc[cat].push(p)
    return acc
  }, {} as Record<string, ComponentTypePreset[]>) ?? {}

  const categoryLabels: Record<string, string> = {
    drivetrain: 'Antrieb',
    braking: 'Bremsen',
    wheels: 'Laufräder',
    frame: 'Rahmen & Cockpit',
    suspension: 'Federung',
    ebike: 'E-Bike',
    lighting: 'Beleuchtung',
    accessories: 'Zubehör',
    shoes: 'Schuhe',
    sole: 'Sohle',
    insole: 'Einlegesohle',
    ski: 'Ski',
    base: 'Belag',
    edge: 'Kanten',
    binding: 'Bindung',
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      {!component && presets && presets.length > 0 && (
        <div>
          <div className="flex items-center gap-2 mb-2">
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input
                type="radio"
                checked={usePreset}
                onChange={() => setUsePreset(true)}
                className="text-orange-500"
              />
              <span className="text-neutral-700 dark:text-neutral-300">Vorlage</span>
            </label>
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input
                type="radio"
                checked={!usePreset}
                onChange={() => setUsePreset(false)}
                className="text-orange-500"
              />
              <span className="text-neutral-700 dark:text-neutral-300">Eigener Typ</span>
            </label>
          </div>

          {usePreset && (
            <select
              value={presetId}
              onChange={(e) => selectPreset(e.target.value)}
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
            >
              <option value="">Vorlage wählen...</option>
              {Object.entries(groupedPresets).map(([cat, items]) => (
                <optgroup key={cat} label={categoryLabels[cat] ?? cat}>
                  {items.map(p => (
                    <option key={p.id} value={p.id}>{p.name}</option>
                  ))}
                </optgroup>
              ))}
            </select>
          )}
        </div>
      )}

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
        <input
          type="number"
          value={weightG}
          onChange={(e) => setWeightG(e.target.value)}
          placeholder="Gewicht (g)"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
        />
        <input
          type="number"
          value={purchasePrice}
          onChange={(e) => setPurchasePrice(e.target.value)}
          placeholder="Kaufpreis (EUR)"
          step="0.01"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
        />
        <input
          type="date"
          value={purchaseDate}
          onChange={(e) => setPurchaseDate(e.target.value)}
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
        />
        <input
          type="text"
          value={serialNumber}
          onChange={(e) => setSerialNumber(e.target.value)}
          placeholder="Seriennummer"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-orange-500 focus:border-transparent"
        />
      </div>
      <div className="flex gap-2 justify-end">
        <button
          type="button"
          onClick={onCancel}
          className="px-3 py-1.5 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
        >
          Abbrechen
        </button>
        <button
          type="submit"
          disabled={isSaving || !name.trim() || !type.trim()}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 transition-colors cursor-pointer"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}

// ── Bike Setups ─────────────────────────────────────────────────────────────────

interface SetupSectionProps {
  label: string
  defaultOpen?: boolean
  children: React.ReactNode
}

function SetupSection({ label, defaultOpen = false, children }: SetupSectionProps) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-lg">
      <button
        onClick={() => setOpen(!open)}
        className="w-full flex items-center justify-between px-3 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 cursor-pointer"
      >
        {label}
        <svg className={`w-4 h-4 text-neutral-400 transition-transform ${open ? '' : '-rotate-90'}`} viewBox="0 0 20 20" fill="currentColor">
          <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
        </svg>
      </button>
      {open && <div className="px-3 pb-3 space-y-2">{children}</div>}
    </div>
  )
}

function JsonField({ label, value, onChange, placeholder }: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  return (
    <div>
      <label className="block text-xs text-neutral-500 mb-1">{label}</label>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
      />
    </div>
  )
}

function BikeSetupsSection({ gearId }: { gearId: string }) {
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)

  const { data: setups } = useQuery({
    queryKey: ['bike-setups', gearId],
    queryFn: () => listBikeSetups(gearId),
  })

  const createMut = useMutation({
    mutationFn: (data: Partial<BikeSetup>) => createBikeSetup(gearId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bike-setups', gearId] })
      setCreating(false)
    },
  })

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<BikeSetup> }) => updateBikeSetup(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bike-setups', gearId] })
      setEditingId(null)
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteBikeSetup,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['bike-setups', gearId] }),
  })

  const activateMut = useMutation({
    mutationFn: activateBikeSetup,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['bike-setups', gearId] }),
  })

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Setups</h2>
        {!creating && (
          <button
            onClick={() => setCreating(true)}
            className="text-sm text-orange-500 hover:text-orange-600 cursor-pointer"
          >
            + Neues Setup
          </button>
        )}
      </div>

      {creating && (
        <BikeSetupForm
          onSave={(data) => createMut.mutate(data)}
          onCancel={() => setCreating(false)}
          isSaving={createMut.isPending}
        />
      )}

      {setups && setups.length > 0 ? (
        <div className="space-y-3">
          {setups.map((setup) => (
            <div key={setup.id} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
              {editingId === setup.id ? (
                <BikeSetupForm
                  setup={setup}
                  onSave={(data) => updateMut.mutate({ id: setup.id, data })}
                  onCancel={() => setEditingId(null)}
                  isSaving={updateMut.isPending}
                />
              ) : (
                <div>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{setup.name}</span>
                      {setup.is_active && (
                        <span className="px-1.5 py-0.5 rounded text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
                          Aktiv
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      {!setup.is_active && (
                        <button
                          onClick={() => activateMut.mutate(setup.id)}
                          disabled={activateMut.isPending}
                          className="text-xs text-emerald-500 hover:text-emerald-700 cursor-pointer"
                        >
                          Aktivieren
                        </button>
                      )}
                      <button
                        onClick={() => setEditingId(setup.id)}
                        className="text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
                      >
                        Bearbeiten
                      </button>
                      <button
                        onClick={() => deleteMut.mutate(setup.id)}
                        disabled={deleteMut.isPending}
                        className="text-xs text-rose-500 hover:text-rose-700 cursor-pointer"
                      >
                        Löschen
                      </button>
                    </div>
                  </div>
                  <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-1 space-x-3">
                    {setup.intended_use && <span>{setup.intended_use}</span>}
                    {setup.rider_weight_kg && <span>{setup.rider_weight_kg} kg</span>}
                  </div>
                  {/* Summary of configured sections */}
                  <div className="flex flex-wrap gap-1.5 mt-2">
                    {setup.suspension_front && <SetupTag label="Gabel" />}
                    {setup.suspension_rear && <SetupTag label="Dämpfer" />}
                    {setup.tires && <SetupTag label="Reifen" />}
                    {setup.contact_points && <SetupTag label="Kontaktpunkte" />}
                    {setup.brakes && <SetupTag label="Bremsen" />}
                    {setup.drivetrain && <SetupTag label="Antrieb" />}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      ) : (
        !creating && (
          <p className="text-sm text-neutral-400 py-2">
            Keine Setups vorhanden. Erstelle ein Setup um Federung, Reifen, Bremsen etc. zu dokumentieren.
          </p>
        )
      )}
    </div>
  )
}

function SetupTag({ label }: { label: string }) {
  return (
    <span className="px-1.5 py-0.5 rounded text-xs bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400">
      {label}
    </span>
  )
}

function BikeSetupForm({ setup, onSave, onCancel, isSaving }: {
  setup?: BikeSetup
  onSave: (data: Partial<BikeSetup>) => void
  onCancel: () => void
  isSaving?: boolean
}) {
  const [name, setName] = useState(setup?.name ?? '')
  const [riderWeight, setRiderWeight] = useState(setup?.rider_weight_kg?.toString() ?? '')
  const [intendedUse, setIntendedUse] = useState(setup?.intended_use ?? '')
  const [notes, setNotes] = useState(setup?.notes ?? '')
  const [isActive, setIsActive] = useState(setup?.is_active ?? false)

  // Suspension front
  const sf = setup?.suspension_front ?? {}
  const [sfTravel, setSfTravel] = useState(sf.travel_mm?.toString() ?? '')
  const [sfPressure, setSfPressure] = useState(sf.pressure_psi?.toString() ?? '')
  const [sfRebound, setSfRebound] = useState(sf.rebound?.toString() ?? '')
  const [sfCompression, setSfCompression] = useState(sf.compression?.toString() ?? '')
  const [sfSag, setSfSag] = useState(sf.sag_pct?.toString() ?? '')

  // Suspension rear
  const sr = setup?.suspension_rear ?? {}
  const [srTravel, setSrTravel] = useState(sr.travel_mm?.toString() ?? '')
  const [srPressure, setSrPressure] = useState(sr.pressure_psi?.toString() ?? '')
  const [srRebound, setSrRebound] = useState(sr.rebound?.toString() ?? '')
  const [srCompression, setSrCompression] = useState(sr.compression?.toString() ?? '')
  const [srSag, setSrSag] = useState(sr.sag_pct?.toString() ?? '')

  // Tires
  const t = setup?.tires ?? {}
  const [tireFront, setTireFront] = useState(t.front_model ?? '')
  const [tireRear, setTireRear] = useState(t.rear_model ?? '')
  const [tireFrontPsi, setTireFrontPsi] = useState(t.front_pressure_psi?.toString() ?? '')
  const [tireRearPsi, setTireRearPsi] = useState(t.rear_pressure_psi?.toString() ?? '')
  const [tireFrontWidth, setTireFrontWidth] = useState(t.front_width_mm?.toString() ?? '')
  const [tireRearWidth, setTireRearWidth] = useState(t.rear_width_mm?.toString() ?? '')

  // Contact points
  const cp = setup?.contact_points ?? {}
  const [saddleHeight, setSaddleHeight] = useState(cp.saddle_height_mm?.toString() ?? '')
  const [saddleModel, setSaddleModel] = useState(cp.saddle_model ?? '')
  const [stemLength, setStemLength] = useState(cp.stem_length_mm?.toString() ?? '')
  const [handlebarWidth, setHandlebarWidth] = useState(cp.handlebar_width_mm?.toString() ?? '')

  // Brakes
  const b = setup?.brakes ?? {}
  const [brakeFrontPad, setBrakeFrontPad] = useState(b.front_pad_type ?? '')
  const [brakeRearPad, setBrakeRearPad] = useState(b.rear_pad_type ?? '')
  const [brakeFrontRotor, setBrakeFrontRotor] = useState(b.front_rotor_mm?.toString() ?? '')
  const [brakeRearRotor, setBrakeRearRotor] = useState(b.rear_rotor_mm?.toString() ?? '')

  // Drivetrain
  const d = setup?.drivetrain ?? {}
  const [chainring, setChainring] = useState(d.chainring ?? '')
  const [cassette, setCassette] = useState(d.cassette ?? '')
  const [chainLength, setChainLength] = useState(d.chain_length?.toString() ?? '')

  function buildJson(fields: [string, string][]): Record<string, any> | null {
    const obj: Record<string, any> = {}
    let hasValue = false
    for (const [key, val] of fields) {
      if (val.trim()) {
        const num = Number(val)
        obj[key] = isNaN(num) ? val.trim() : num
        hasValue = true
      }
    }
    return hasValue ? obj : null
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return

    onSave({
      name: name.trim(),
      rider_weight_kg: riderWeight ? parseFloat(riderWeight) : null,
      intended_use: intendedUse.trim() || null,
      notes: notes.trim() || null,
      is_active: isActive,
      suspension_front: buildJson([
        ['travel_mm', sfTravel], ['pressure_psi', sfPressure],
        ['rebound', sfRebound], ['compression', sfCompression], ['sag_pct', sfSag],
      ]),
      suspension_rear: buildJson([
        ['travel_mm', srTravel], ['pressure_psi', srPressure],
        ['rebound', srRebound], ['compression', srCompression], ['sag_pct', srSag],
      ]),
      tires: buildJson([
        ['front_model', tireFront], ['rear_model', tireRear],
        ['front_pressure_psi', tireFrontPsi], ['rear_pressure_psi', tireRearPsi],
        ['front_width_mm', tireFrontWidth], ['rear_width_mm', tireRearWidth],
      ]),
      contact_points: buildJson([
        ['saddle_height_mm', saddleHeight], ['saddle_model', saddleModel],
        ['stem_length_mm', stemLength], ['handlebar_width_mm', handlebarWidth],
      ]),
      brakes: buildJson([
        ['front_pad_type', brakeFrontPad], ['rear_pad_type', brakeRearPad],
        ['front_rotor_mm', brakeFrontRotor], ['rear_rotor_mm', brakeRearRotor],
      ]),
      drivetrain: buildJson([
        ['chainring', chainring], ['cassette', cassette], ['chain_length', chainLength],
      ]),
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3 mb-4">
      <div className="grid grid-cols-2 gap-3">
        <div className="col-span-2">
          <label className="block text-xs text-neutral-500 mb-1">Name *</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            placeholder="z.B. Trail Setup, Bikepark, Winter"
            className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
          />
        </div>
        <JsonField label="Fahrergewicht (kg)" value={riderWeight} onChange={setRiderWeight} placeholder="75" />
        <JsonField label="Einsatzzweck" value={intendedUse} onChange={setIntendedUse} placeholder="Trail, Bikepark, Tour" />
      </div>

      <SetupSection label="Federung vorne (Gabel)">
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <JsonField label="Federweg (mm)" value={sfTravel} onChange={setSfTravel} placeholder="150" />
          <JsonField label="Druck (PSI)" value={sfPressure} onChange={setSfPressure} placeholder="65" />
          <JsonField label="Rebound (Klicks)" value={sfRebound} onChange={setSfRebound} placeholder="8" />
          <JsonField label="Compression (Klicks)" value={sfCompression} onChange={setSfCompression} placeholder="5" />
          <JsonField label="Sag (%)" value={sfSag} onChange={setSfSag} placeholder="25" />
        </div>
      </SetupSection>

      <SetupSection label="Federung hinten (Dämpfer)">
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <JsonField label="Federweg (mm)" value={srTravel} onChange={setSrTravel} placeholder="140" />
          <JsonField label="Druck (PSI)" value={srPressure} onChange={setSrPressure} placeholder="180" />
          <JsonField label="Rebound (Klicks)" value={srRebound} onChange={setSrRebound} placeholder="6" />
          <JsonField label="Compression (Klicks)" value={srCompression} onChange={setSrCompression} placeholder="4" />
          <JsonField label="Sag (%)" value={srSag} onChange={setSrSag} placeholder="30" />
        </div>
      </SetupSection>

      <SetupSection label="Reifen">
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <JsonField label="Vorne: Modell" value={tireFront} onChange={setTireFront} placeholder="Maxxis Minion DHF" />
          <JsonField label="Vorne: Breite (mm)" value={tireFrontWidth} onChange={setTireFrontWidth} placeholder="2.5" />
          <JsonField label="Vorne: Druck (PSI)" value={tireFrontPsi} onChange={setTireFrontPsi} placeholder="24" />
          <JsonField label="Hinten: Modell" value={tireRear} onChange={setTireRear} placeholder="Maxxis Dissector" />
          <JsonField label="Hinten: Breite (mm)" value={tireRearWidth} onChange={setTireRearWidth} placeholder="2.4" />
          <JsonField label="Hinten: Druck (PSI)" value={tireRearPsi} onChange={setTireRearPsi} placeholder="26" />
        </div>
      </SetupSection>

      <SetupSection label="Kontaktpunkte">
        <div className="grid grid-cols-2 gap-2">
          <JsonField label="Sattelhöhe (mm)" value={saddleHeight} onChange={setSaddleHeight} placeholder="720" />
          <JsonField label="Sattel-Modell" value={saddleModel} onChange={setSaddleModel} placeholder="Ergon SM Sport" />
          <JsonField label="Vorbaulänge (mm)" value={stemLength} onChange={setStemLength} placeholder="50" />
          <JsonField label="Lenkerbreite (mm)" value={handlebarWidth} onChange={setHandlebarWidth} placeholder="780" />
        </div>
      </SetupSection>

      <SetupSection label="Bremsen">
        <div className="grid grid-cols-2 gap-2">
          <JsonField label="Vorne: Belag-Typ" value={brakeFrontPad} onChange={setBrakeFrontPad} placeholder="Sinter, Organisch" />
          <JsonField label="Vorne: Scheibe (mm)" value={brakeFrontRotor} onChange={setBrakeFrontRotor} placeholder="203" />
          <JsonField label="Hinten: Belag-Typ" value={brakeRearPad} onChange={setBrakeRearPad} placeholder="Sinter, Organisch" />
          <JsonField label="Hinten: Scheibe (mm)" value={brakeRearRotor} onChange={setBrakeRearRotor} placeholder="180" />
        </div>
      </SetupSection>

      <SetupSection label="Antrieb">
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <JsonField label="Kettenblatt" value={chainring} onChange={setChainring} placeholder="32T" />
          <JsonField label="Kassette" value={cassette} onChange={setCassette} placeholder="10-52T" />
          <JsonField label="Kettenlänge (Glieder)" value={chainLength} onChange={setChainLength} placeholder="116" />
        </div>
      </SetupSection>

      <div>
        <label className="block text-xs text-neutral-500 mb-1">Notizen</label>
        <input
          type="text"
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          placeholder="Sonstige Anmerkungen"
          className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
        />
      </div>

      <label className="flex items-center gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={isActive}
          onChange={(e) => setIsActive(e.target.checked)}
          className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-orange-500 focus:ring-orange-500"
        />
        <span className="text-sm text-neutral-700 dark:text-neutral-300">Als aktives Setup setzen</span>
      </label>

      <div className="flex gap-2 justify-end">
        <button
          type="button"
          onClick={onCancel}
          className="px-3 py-1.5 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
        >
          Abbrechen
        </button>
        <button
          type="submit"
          disabled={isSaving || !name.trim()}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 cursor-pointer"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}

// ── Helpers ─────────────────────────────────────────────────────────────────────

function formatDateShort(dateStr: string): string {
  try {
    return format(new Date(dateStr), 'dd.MM.yy', { locale: de })
  } catch {
    return dateStr
  }
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

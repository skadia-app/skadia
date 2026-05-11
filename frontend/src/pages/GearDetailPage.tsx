import { useState, useCallback, useEffect } from 'react'
import { toLocalISO } from '@/lib/dateUtils'
import { useParams, Link, useSearchParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { CircleHelp } from 'lucide-react'
import { getActiveSchema } from '@/lib/gearSchema'
import { gearTypeAttributes, gearTypeLabel as schemaGearTypeLabel, gearTypeScope } from '@/lib/gearSchemaHelpers'
import { type GearCategoryScope } from '@/lib/gearSchemaTypes'
import {
  fetchGearDetail, updateGear, updateGearStatus,
  createComponent, updateComponent, deleteComponent,
  transferComponent, getComponentHistory, fetchGear,
  getChainRotation, createChainRotation, updateChainRotation, swapChain, deleteChainRotation,
  listServiceEvents, createServiceEvent, deleteServiceEvent,
  listServiceIntervals, createServiceInterval, deleteServiceInterval,
  fetchComponentPresets,
  listBikeSetups, createBikeSetup, updateBikeSetup, deleteBikeSetup, activateBikeSetup,
  type BikeSetupCreatePayload, type BikeSetupUpdatePayload,
  fetchGearDocuments, uploadGearDocument, deleteGearDocument,
  downloadGearDocumentBlob,
  uploadComponentDocument, listComponentDocuments,
  getGearCosts,
} from '@/api/gear'
import type {
  Gear, GearComponent, GearStatus,
  ComponentAssignment, ChainRotationMember,
  GearServiceEvent, GearServiceInterval, ComponentTypePreset,
  BikeSetup, GearDocument,
  GearCostSummary, GearDefaultSportPredicate,
} from '@/api/types'
import { formatDistance, formatDuration, formatElevationGain, formatElevationLoss } from '@/lib/utils'
import { PRODUCTIVE_SERVICE_METRICS, USAGE_METRIC_LABELS, formatUsageMetricValue, type ServiceMetricKey } from '@/lib/gearUsageMetrics'
import { useModal } from '@/lib/modal'
import GearForm from '@/components/GearForm'
import GearDocumentSection from '@/components/GearDocumentSection'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import GearGeometryTab from '@/components/gear/GearGeometryTab'
import GearSportsTab from '@/components/gear/GearSportsTab'
import GearComponentsTab from '@/components/gear/GearComponentsTab'
import GearWearTab from '@/components/gear/GearWearTab'

// Valid tab values for URL-param sync.
type GearDetailTab = 'stammdaten' | 'geometry' | 'sports' | 'components' | 'setups' | 'wear'
const VALID_TABS: GearDetailTab[] = ['stammdaten', 'geometry', 'sports', 'components', 'setups', 'wear']

function isValidTab(v: string | null): v is GearDetailTab {
  return v !== null && (VALID_TABS as string[]).includes(v)
}

export default function GearDetailPage() {
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [searchParams, setSearchParams] = useSearchParams()

  const rawTab = searchParams.get('tab')
  const activeTab: GearDetailTab = isValidTab(rawTab) ? rawTab : 'stammdaten'

  function switchTab(tab: GearDetailTab) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (tab === 'stammdaten') {
        next.delete('tab')
      } else {
        next.set('tab', tab)
      }
      return next
    })
  }

  const { data: gear, isLoading } = useQuery({
    queryKey: ['gear-detail', id],
    queryFn: () => fetchGearDetail(id!),
    enabled: !!id,
  })

  const { data: costSummary } = useQuery({
    queryKey: ['gear-costs', id],
    queryFn: () => getGearCosts(id!),
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
        <Link to="/gear" className="text-sm text-accent-DEFAULT hover:text-accent-600">Zurück zur Übersicht</Link>
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
            categoryScope={categoryScopeForGear(gear)}
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
                {gearTypeLabelResolved(gear.type)}
                {gear.sub_type && ` / ${subTypeLabel(gear.sub_type)}`}
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
          <GearDetailTabs
            gear={gear}
            costSummary={costSummary ?? null}
            activeTab={activeTab}
            onTabChange={switchTab}
            onSportsSave={(predicates) =>
              updateMutation.mutate({ applicable_sport_predicates: predicates })
            }
          />
        </>
      )}
    </div>
  )
}

// ── GearDetailTabs ───────────────────────────────────────────────────────────────

interface GearDetailTabsProps {
  gear: Gear
  costSummary: GearCostSummary | null
  activeTab: GearDetailTab
  onTabChange: (tab: GearDetailTab) => void
  onSportsSave: (predicates: GearDefaultSportPredicate[]) => void
}

function GearDetailTabs({ gear, costSummary, activeTab, onTabChange, onSportsSave }: GearDetailTabsProps) {
  const schema = getActiveSchema()
  const attrs = gearTypeAttributes(schema, gear.type)
  const hasGeometry = attrs.some(
    (a) => a.valueType === 'geometry' && a.subfields && a.subfields.length > 0,
  )
  const isBike = gearTypeScope(schema, gear.type, gear.scope) === 'bike'
  const components = gear.components ?? []
  const hasComponentHierarchy = components.some((component) => component.parent_component_id)
  const displayedTab = activeTab === 'setups' && !isBike ? 'components' : activeTab

  return (
    <Tabs value={displayedTab} onValueChange={(v) => onTabChange(v as GearDetailTab)}>
      <TabsList className="flex-wrap h-auto">
        <TabsTrigger value="stammdaten">Stammdaten</TabsTrigger>
        {hasGeometry && <TabsTrigger value="geometry">Geometrie</TabsTrigger>}
        <TabsTrigger value="sports">Sport</TabsTrigger>
        <TabsTrigger value="components">Komponenten</TabsTrigger>
        {isBike && <TabsTrigger value="setups">Setups</TabsTrigger>}
        <TabsTrigger value="wear">Verschleiß</TabsTrigger>
      </TabsList>

      {/* Stammdaten tab — stats, notes, images, documents, costs */}
      <TabsContent value="stammdaten" className="space-y-6">
        {/* Tiefenmeter is the lead metric for ski / downhill-MTB gear; keep it
            visible at the same level as Höhenmeter so neither hides in a sub-tab. */}
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4">
          <StatCard label="Distanz" value={formatDistance(gear.total_distance_m)} />
          <StatCard label="Höhenmeter" value={formatElevationGain(gear.total_elevation_gain_m) ?? '—'} />
          <StatCard label="Tiefenmeter" value={formatElevationLoss(gear.total_elevation_loss_m) ?? '—'} />
          <StatCard label="Zeit" value={formatDuration(gear.total_time_s)} />
          <StatCard label="Aktivitäten" value={String(gear.total_activities)} />
        </div>

        {gear.weight_g != null && (
          <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4">
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Gewicht</h3>
            <p className="text-sm text-neutral-600 dark:text-neutral-400">{gear.weight_g} g</p>
          </div>
        )}

        {gear.notes && (
          <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4">
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">Notizen</h3>
            <p className="text-sm text-neutral-600 dark:text-neutral-400 whitespace-pre-wrap">{gear.notes}</p>
          </div>
        )}

        <GearImagesSection gearId={gear.id} />
        <GearDocumentSection gearId={gear.id} />

        {costSummary && costSummary.total_cost > 0 && (
          <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 space-y-4">
            <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Kosten</h2>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
              <div>
                <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Gesamtkosten</div>
                <div className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
                  {costSummary.total_cost.toFixed(2)} {costSummary.currency}
                </div>
              </div>
              {costSummary.cost_per_km != null && (
                <div>
                  <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Kosten/km</div>
                  <div className="text-lg font-semibold text-neutral-700 dark:text-neutral-300">
                    {costSummary.cost_per_km.toFixed(3)} {costSummary.currency}/km
                  </div>
                </div>
              )}
              {costSummary.cost_per_hour != null && (
                <div>
                  <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Kosten/Stunde</div>
                  <div className="text-lg font-semibold text-neutral-700 dark:text-neutral-300">
                    {costSummary.cost_per_hour.toFixed(2)} {costSummary.currency}/h
                  </div>
                </div>
              )}
            </div>
          </section>
        )}
      </TabsContent>

      {/* Geometrie tab — schema-driven subfield editor */}
      {hasGeometry && (
        <TabsContent value="geometry">
          <GearGeometryTab item={gear} schema={schema} onChange={() => {}} />
        </TabsContent>
      )}

      {/* Sport tab — applicable_sport_predicates editor */}
      <TabsContent value="sports">
        <GearSportsTab item={gear} onSave={onSportsSave} />
      </TabsContent>

      {/* Komponenten tab — chain rotation + editable component list first.
          The read-only hierarchy is only useful when parent-child links exist. */}
      <TabsContent value="components" className="space-y-6">
        {isBike && (
          <ChainRotationSection gearId={gear.id} components={components} />
        )}
        <section data-testid="components-crud">
          <ComponentsList gearId={gear.id} gearType={gear.type} components={components} />
        </section>
        {hasComponentHierarchy && (
          <section data-testid="components-hierarchy">
            <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 mb-2 uppercase tracking-wide">
              Hierarchie
            </h3>
            <GearComponentsTab components={components} />
          </section>
        )}
      </TabsContent>

      {isBike && (
        <TabsContent value="setups">
          <BikeSetupsSection gearId={gear.id} />
        </TabsContent>
      )}

      {/* Verschleiß tab — usage aggregates + service events placeholder */}
      <TabsContent value="wear">
        <GearWearTab item={gear} />
      </TabsContent>
    </Tabs>
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
        className="px-3 py-1.5 rounded-lg text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
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
  const [expandedId, setExpandedId] = useState<string | null>(null)

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
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-accent-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 transition-colors cursor-pointer"
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
                    <button
                      className="cursor-pointer min-w-0 flex-1 text-left group"
                      onClick={() => setExpandedId(expandedId === comp.id ? null : comp.id)}
                    >
                      <div className="flex items-center gap-2">
                        <svg
                          className={`w-3.5 h-3.5 text-neutral-400 transition-transform flex-shrink-0 ${expandedId === comp.id ? 'rotate-90' : ''}`}
                          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}
                        >
                          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                        </svg>
                        <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
                          {comp.name}
                        </span>
                        <ComponentStatusBadge status={comp.status} />
                      </div>
                      <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5 ml-5 flex flex-wrap gap-x-2 gap-y-0.5">
                        {[
                          comp.type,
                          [comp.brand, comp.model].filter(Boolean).join(' '),
                          formatComponentDistance(comp.total_distance_m),
                          formatComponentDuration(comp.total_time_s),
                          comp.total_elevation_gain_m > 0 ? formatElevationGain(comp.total_elevation_gain_m) : '',
                          comp.total_elevation_loss_m > 0 ? formatElevationLoss(comp.total_elevation_loss_m) : '',
                          comp.weight_g != null ? `${comp.weight_g} g` : '',
                        ].filter(Boolean).map((part, index) => (
                          <span key={`${part}-${index}`}>{part}</span>
                        ))}
                      </div>
                    </button>
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
                        Übertragen
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
                        className="text-xs text-rose-500 hover:text-rose-700 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        Entfernen
                      </button>
                    </div>
                  </div>

                  {expandedId === comp.id && (
                    <div className="mt-2 ml-5 space-y-1">
                      {comp.serial_number && (
                        <div className="text-xs text-neutral-400">SN: {comp.serial_number}</div>
                      )}
                      <ComponentImagesSection componentId={comp.id} />
                    </div>
                  )}

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
                    <ServiceEventsPanel componentId={comp.id} status={comp.status} />
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

function formatComponentDistance(meters: number): string {
  return `${Math.round(Math.max(0, meters) / 1000)} km`
}

function formatComponentDuration(seconds: number): string {
  const minutes = Math.round(Math.max(0, seconds) / 60)
  if (minutes < 60) return `${minutes} min`
  return `${Math.round(minutes / 60)} h`
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
            <option key={g.id} value={g.id}>{g.name} ({schemaGearTypeLabel(getActiveSchema(), g.type)})</option>
          ))}
        </select>
        <button
          onClick={() => mutation.mutate()}
          disabled={!targetId || mutation.isPending}
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
        >
          {mutation.isPending ? '...' : 'Übertragen'}
        </button>
        <button
          onClick={onClose}
          className="px-3 py-1.5 rounded-lg text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
        >
          Abbrechen
        </button>
      </div>
      {mutation.isError && (
        <p className="text-xs text-rose-500 mt-1">Übertragung fehlgeschlagen</p>
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

  // Resolve gear_id → human-readable name; gear-list is shared across query cache
  const { data: gearList } = useQuery({
    queryKey: ['gear-list'],
    queryFn: () => fetchGear(),
    staleTime: 60_000,
  })

  const gearName = (gearId: string) =>
    gearList?.find((g) => g.id === gearId)?.name ?? gearId.slice(0, 8)

  if (isLoading) return <div className="text-xs text-neutral-400 mt-2">Lade Historie...</div>

  if (!history || history.length === 0) {
    return <div className="text-xs text-neutral-400 mt-2">Keine Übertragungs-Historie vorhanden.</div>
  }

  return (
    <div className="mt-3 space-y-1">
      <div className="text-xs font-medium text-neutral-600 dark:text-neutral-300">Übertragungs-Historie</div>
      {history.map((a: ComponentAssignment) => (
        <div key={a.id} className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
          <span className="w-2 h-2 rounded-full bg-accent-400 flex-shrink-0" />
          <span className="font-medium text-neutral-700 dark:text-neutral-300">{gearName(a.gear_id)}</span>
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

function ServiceEventsPanel({ componentId, status }: { componentId: string; status: GearStatus }) {
  const isActive = status === 'active'
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [addingInterval, setAddingInterval] = useState(false)
  const [performedAt, setPerformedAt] = useState(toLocalISO(new Date()))
  const [description, setDescription] = useState('')
  const [cost, setCost] = useState('')
  const [notes, setNotes] = useState('')
  const [selectedIntervalId, setSelectedIntervalId] = useState('')
  const [intervalName, setIntervalName] = useState('')
  const [intervalMetric, setIntervalMetric] = useState<ServiceMetricKey>('moving_time_s')
  const [intervalValue, setIntervalValue] = useState('')

  const { data: events, isLoading } = useQuery({
    queryKey: ['service-events', componentId],
    queryFn: () => listServiceEvents(componentId),
  })

  const { data: intervals } = useQuery({
    queryKey: ['service-intervals', componentId],
    queryFn: () => listServiceIntervals(componentId),
  })

  const addMut = useMutation({
    mutationFn: () => createServiceEvent(componentId, {
      performed_at: new Date(performedAt).toISOString(),
      interval_id: selectedIntervalId || null,
      description: description.trim() || null,
      cost: cost ? parseFloat(cost) : null,
      notes: notes.trim() || null,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['service-events', componentId] })
      queryClient.invalidateQueries({ queryKey: ['service-intervals', componentId] })
      setAdding(false)
      setDescription('')
      setCost('')
      setNotes('')
      setSelectedIntervalId('')
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteServiceEvent,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['service-events', componentId] })
    },
  })

  const addIntervalMut = useMutation({
    mutationFn: () => createServiceInterval(componentId, {
      name: intervalName.trim() || USAGE_METRIC_LABELS[intervalMetric],
      metric_key: intervalMetric,
      interval_type: intervalMetric,
      interval_value: parseFloat(intervalValue) || 0,
      is_recurring: true,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['service-intervals', componentId] })
      setAddingInterval(false)
      setIntervalName('')
      setIntervalValue('')
    },
  })

  const deleteIntervalMut = useMutation({
    mutationFn: deleteServiceInterval,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['service-intervals', componentId] })
    },
  })

  return (
    <div className="mt-3 space-y-2">
      <div className="flex items-center justify-between">
        <div className="text-xs font-medium text-neutral-600 dark:text-neutral-300">Service-Historie</div>
        {isActive && (
          <div className="flex items-center gap-3">
            <button
              onClick={() => setAddingInterval(!addingInterval)}
              className="text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
            >
              + Intervall
            </button>
            <button
              onClick={() => setAdding(!adding)}
              className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
            >
              + Service eintragen
            </button>
          </div>
        )}
      </div>

      {intervals && intervals.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {intervals.map((interval: GearServiceInterval) => (
            <span key={interval.id} className="inline-flex items-center gap-1 rounded bg-neutral-100 dark:bg-neutral-800 px-2 py-1 text-xs text-neutral-600 dark:text-neutral-300">
              {interval.name}: {formatUsageMetricValue(interval.metric_key, interval.interval_value)}
              <button
                onClick={() => deleteIntervalMut.mutate(interval.id)}
                disabled={deleteIntervalMut.isPending}
                className="text-neutral-400 hover:text-rose-500 disabled:opacity-50 cursor-pointer"
                title="Intervall löschen"
              >
                x
              </button>
            </span>
          ))}
        </div>
      )}

      {isActive && addingInterval && (
        <div className="p-3 bg-neutral-50 dark:bg-neutral-800/50 rounded-lg space-y-2">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
            <input
              type="text"
              value={intervalName}
              onChange={(e) => setIntervalName(e.target.value)}
              placeholder="Name"
              className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
            />
            <select
              value={intervalMetric}
              onChange={(e) => setIntervalMetric(e.target.value as ServiceMetricKey)}
              className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
            >
              {PRODUCTIVE_SERVICE_METRICS.map(metric => (
                <option key={metric} value={metric}>{USAGE_METRIC_LABELS[metric]}</option>
              ))}
            </select>
            <input
              type="number"
              value={intervalValue}
              onChange={(e) => setIntervalValue(e.target.value)}
              placeholder="Wert"
              className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
            />
          </div>
          <div className="flex gap-2 justify-end">
            <button
              onClick={() => setAddingInterval(false)}
              className="px-2.5 py-1 rounded text-xs text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer"
            >
              Abbrechen
            </button>
            <button
              onClick={() => addIntervalMut.mutate()}
              disabled={!intervalValue || addIntervalMut.isPending}
              className="px-2.5 py-1 rounded text-xs font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
            >
              Speichern
            </button>
          </div>
        </div>
      )}

      {isActive && adding && (
        <div className="p-3 bg-neutral-50 dark:bg-neutral-800/50 rounded-lg space-y-2">
          <div className="grid grid-cols-2 gap-2">
            <input
              type="date"
              value={performedAt}
              onChange={(e) => setPerformedAt(e.target.value)}
              className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
            />
            {intervals && intervals.length > 0 && (
              <select
                value={selectedIntervalId}
                onChange={(e) => setSelectedIntervalId(e.target.value)}
                className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
              >
                <option value="">Kein Intervall-Reset</option>
                {intervals.map(interval => (
                  <option key={interval.id} value={interval.id}>
                    {interval.name}
                  </option>
                ))}
              </select>
            )}
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
              className="px-2.5 py-1 rounded text-xs font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
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
                className="text-rose-500 hover:text-rose-700 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
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

// ── Chain-rotation metric helpers ──────────────────────────────────────────────

type ChainRotationMetricKey = 'distance_m' | 'moving_time_s' | 'elapsed_time_s' | 'elevation_gain_m' | 'elevation_loss_m'

const CHAIN_ROTATION_METRICS: ChainRotationMetricKey[] = [
  'distance_m',
  'moving_time_s',
  'elapsed_time_s',
  'elevation_gain_m',
  'elevation_loss_m',
]

/** Convert a user-facing display value to base units (metres / seconds). */
function metricInputToBase(metric: ChainRotationMetricKey, value: string): number {
  const parsed = parseFloat(value)
  if (!Number.isFinite(parsed) || parsed <= 0) return metric === 'distance_m' ? 500000 : 0
  if (metric === 'distance_m') return parsed * 1000
  if (metric === 'moving_time_s' || metric === 'elapsed_time_s') return parsed * 3600
  return parsed
}

/** Return the member's accumulated value for the given metric. */
function chainRotationMemberValue(member: ChainRotationMember, metric: ChainRotationMetricKey): number {
  switch (metric) {
    case 'distance_m':      return member.total_distance_m ?? 0
    case 'moving_time_s':   return member.total_moving_time_s ?? 0
    case 'elapsed_time_s':  return member.total_time_s ?? 0
    case 'elevation_gain_m': return member.total_elevation_gain_m ?? 0
    case 'elevation_loss_m': return member.total_elevation_loss_m ?? 0
  }
}

function ChainRotationSection({ gearId, components }: {
  gearId: string
  components: GearComponent[]
}) {
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('Ketten-Rotation')
  const [rotationMetric, setRotationMetric] = useState<ChainRotationMetricKey>('distance_m')
  const [rotationIntervalValue, setRotationIntervalValue] = useState('500')
  const [sortMetric, setSortMetric] = useState<ChainRotationMetricKey>('distance_m')
  const [selectedComps, setSelectedComps] = useState<Set<string>>(new Set())

  // Edit-form state for modifying an existing rotation's swap interval.
  const [editing, setEditing] = useState(false)
  const [editName, setEditName] = useState('')
  const [editMetric, setEditMetric] = useState<ChainRotationMetricKey>('distance_m')
  const [editIntervalValue, setEditIntervalValue] = useState('')
  const [editSuccess, setEditSuccess] = useState(false)

  const { data: rotation } = useQuery({
    queryKey: ['chain-rotation', gearId],
    queryFn: () => getChainRotation(gearId),
  })

  const createMut = useMutation({
    mutationFn: () => createChainRotation(gearId, {
      name,
      swap_interval_metric_key: rotationMetric,
      swap_interval_value: metricInputToBase(rotationMetric, rotationIntervalValue),
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

  const updateMut = useMutation({
    mutationFn: () => {
      const trimmedName = editName.trim()
      // Always include name — backend trims + rejects empty + caps at 100 chars.
      // We guard client-side too, but send the trimmed value unconditionally so
      // the server always has the latest value even if only the interval changed.
      return updateChainRotation(rotation!.id, {
        name: trimmedName,
        swap_interval_metric_key: editMetric,
        swap_interval_value: metricInputToBase(editMetric, editIntervalValue),
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['chain-rotation', gearId] })
      setEditing(false)
      setEditSuccess(true)
      // Clear success indicator after a short delay.
      setTimeout(() => setEditSuccess(false), 2500)
    },
  })

  /** Open the edit form pre-filled with the current rotation values. */
  function openEdit() {
    if (!rotation) return
    const metric = (rotation.swap_interval_metric_key || 'distance_m') as ChainRotationMetricKey
    const baseValue = rotation.swap_interval_value ?? ((rotation.swap_interval_km ?? 500) * 1000)
    // Convert base units back to user-facing display units for the input.
    let displayValue: string
    if (metric === 'distance_m') displayValue = String(baseValue / 1000)
    else if (metric === 'moving_time_s' || metric === 'elapsed_time_s') displayValue = String(baseValue / 3600)
    else displayValue = String(baseValue)
    setEditName(rotation.name)
    setEditMetric(metric)
    setEditIntervalValue(displayValue)
    setEditing(true)
    setEditSuccess(false)
  }

  function toggleComp(id: string) {
    setSelectedComps(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  // Match on both type and name so that components whose preset type was
  // stored as "drivetrain" (API bug: no separate type column) are still
  // recognised when their name contains "kette" or "chain".
  function isChainLike(c: GearComponent) {
    const haystack = `${c.type} ${c.name}`.toLowerCase()
    return haystack.includes('kette') || haystack.includes('chain')
  }

  const chainComponents = components.filter(isChainLike)
  // If the heuristic finds nothing, fall back to all active components so
  // the user is never left with an empty list.
  const selectableComponents = chainComponents.length > 0 ? chainComponents : components.filter(c => !c.is_retired)
  const usingFallback = chainComponents.length === 0 && selectableComponents.length > 0

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Ketten-Rotation</h2>
        {!rotation && !creating && (
          <button
            onClick={() => setCreating(true)}
            className="text-sm text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
          >
            + Rotation erstellen
          </button>
        )}
      </div>

      {creating && (
        <div className="space-y-3 mb-4">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
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
              <label className="block text-xs text-neutral-500 mb-1">Metrik</label>
              <select
                value={rotationMetric}
                onChange={(e) => setRotationMetric(e.target.value as ChainRotationMetricKey)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
              >
                {CHAIN_ROTATION_METRICS.map(metric => (
                  <option key={metric} value={metric}>{USAGE_METRIC_LABELS[metric]}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs text-neutral-500 mb-1">
                Wechselintervall ({rotationMetric === 'distance_m' ? 'km' : rotationMetric.endsWith('_time_s') ? 'h' : 'm'})
              </label>
              <input
                type="number"
                value={rotationIntervalValue}
                onChange={(e) => setRotationIntervalValue(e.target.value)}
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs text-neutral-500 mb-1">
              Ketten auswählen (min. 2)
              {usingFallback && (
                <span className="ml-1.5 text-accent-DEFAULT">
                  — alle Komponenten gezeigt, da keine Ketten-Komponenten erkannt wurden
                </span>
              )}
            </label>
            {selectableComponents.length > 0 ? (
              <div className="space-y-1">
                {selectableComponents.map(c => (
                  <label key={c.id} className="flex items-center gap-2 text-sm cursor-pointer">
                    <input
                      type="checkbox"
                      checked={selectedComps.has(c.id)}
                      onChange={() => toggleComp(c.id)}
                      className="rounded"
                    />
                    <span className="text-neutral-700 dark:text-neutral-300">{c.name}</span>
                    {c.type && (
                      <span className="text-xs text-neutral-400">({c.type})</span>
                    )}
                    <span className="text-xs text-neutral-400">{formatDistance(c.total_distance_m)}</span>
                  </label>
                ))}
              </div>
            ) : (
              <p className="text-xs text-neutral-400">
                Keine Komponenten vorhanden. Füge zuerst Ketten als Komponenten hinzu.
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
              className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
            >
              {selectedComps.size < 2
                ? `Rotation erstellen (${selectedComps.size}/2 gewählt)`
                : 'Rotation erstellen'}
            </button>
          </div>
        </div>
      )}

      {rotation && (
        <div className="space-y-3">
          {(() => {
            const metric = (rotation.swap_interval_metric_key || 'distance_m') as ChainRotationMetricKey
            const intervalValue = rotation.swap_interval_value ?? ((rotation.swap_interval_km ?? 500) * 1000)
            // Sort members by their value for the selected sort metric (descending)
            const sortedMembers = rotation.members
              ? [...rotation.members].sort((a, b) => chainRotationMemberValue(b, sortMetric) - chainRotationMemberValue(a, sortMetric))
              : []
            return (
              <>
                {/* Header row: name, swap-interval display, edit button */}
                <div className="flex items-center justify-between text-sm">
                  <span className="text-neutral-700 dark:text-neutral-300 font-medium">{rotation.name}</span>
                  <div className="flex items-center gap-2">
                    {editSuccess && (
                      <span className="text-xs text-emerald-600 dark:text-emerald-400">Gespeichert</span>
                    )}
                    <span className="text-xs text-neutral-500">
                      Wechsel alle {formatUsageMetricValue(metric, intervalValue)}
                    </span>
                    {!editing && (
                      <button
                        onClick={openEdit}
                        title="Bearbeiten"
                        aria-label="Intervall bearbeiten"
                        className="text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 cursor-pointer"
                      >
                        {/* Pencil icon */}
                        <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M15.232 5.232l3.536 3.536M9 13l6.586-6.586a2 2 0 112.828 2.828L11.828 15.828A4 4 0 019 17H7v-2a4 4 0 012.172-2.828z" />
                        </svg>
                      </button>
                    )}
                  </div>
                </div>

                {/* Inline edit form for swap interval */}
                {editing && (
                  <div className="p-3 bg-neutral-50 dark:bg-neutral-800/50 rounded-lg space-y-2">
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
                      <div>
                        <label className="block text-xs text-neutral-500 mb-1">Bezeichnung</label>
                        <input
                          type="text"
                          value={editName}
                          onChange={(e) => setEditName(e.target.value)}
                          className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
                        />
                      </div>
                      <div>
                        <label htmlFor="edit-chain-metric" className="block text-xs text-neutral-500 mb-1">Metrik</label>
                        <select
                          id="edit-chain-metric"
                          value={editMetric}
                          onChange={(e) => setEditMetric(e.target.value as ChainRotationMetricKey)}
                          className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
                        >
                          {CHAIN_ROTATION_METRICS.map(m => (
                            <option key={m} value={m}>{USAGE_METRIC_LABELS[m]}</option>
                          ))}
                        </select>
                      </div>
                      <div>
                        <label className="block text-xs text-neutral-500 mb-1">
                          Wechselintervall ({editMetric === 'distance_m' ? 'km' : editMetric.endsWith('_time_s') ? 'h' : 'm'})
                        </label>
                        <input
                          type="number"
                          value={editIntervalValue}
                          onChange={(e) => setEditIntervalValue(e.target.value)}
                          className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
                        />
                      </div>
                    </div>
                    <div className="flex gap-2 justify-end">
                      <button
                        onClick={() => setEditing(false)}
                        className="px-2.5 py-1 rounded text-xs text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer"
                      >
                        Abbrechen
                      </button>
                      <button
                        onClick={() => updateMut.mutate()}
                        disabled={!editIntervalValue || !editName.trim() || updateMut.isPending}
                        className="px-2.5 py-1 rounded text-xs font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
                      >
                        {updateMut.isPending ? '...' : 'Speichern'}
                      </button>
                    </div>
                    {editName.trim() === '' && (
                      <p className="text-xs text-rose-500">Name darf nicht leer sein.</p>
                    )}
                    {updateMut.isError && (
                      <p className="text-xs text-rose-500">Speichern fehlgeschlagen</p>
                    )}
                  </div>
                )}

                {rotation.members && rotation.members.length > 0 && (
                  <div className="space-y-1.5">
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-xs text-neutral-400">Sortierung</span>
                      <select
                        value={sortMetric}
                        onChange={(e) => setSortMetric(e.target.value as ChainRotationMetricKey)}
                        className="text-xs rounded border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-1.5 py-0.5 text-neutral-700 dark:text-neutral-300"
                      >
                        {CHAIN_ROTATION_METRICS.map(m => (
                          <option key={m} value={m}>{USAGE_METRIC_LABELS[m]}</option>
                        ))}
                      </select>
                    </div>
                    {sortedMembers.map(m => {
                      const currentValue = chainRotationMemberValue(m, metric)
                      const progress = intervalValue > 0 ? Math.min((currentValue / intervalValue) * 100, 100) : 0
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
                                {formatUsageMetricValue(metric, currentValue)} / {formatUsageMetricValue(metric, intervalValue)}
                              </span>
                            </div>
                            <div className="w-full h-1 rounded-full bg-neutral-200 dark:bg-neutral-700 mt-0.5">
                              <div
                                className={`h-full rounded-full transition-all ${isDue ? 'bg-rose-500' : 'bg-accent-400'}`}
                                style={{ width: `${progress}%` }}
                              />
                            </div>
                          </div>
                        </div>
                      )
                    })}
                  </div>
                )}
              </>
            )
          })()}

          <div className="flex gap-2 justify-end pt-1">
            <button
              onClick={() => deleteMut.mutate()}
              disabled={deleteMut.isPending}
              className="px-3 py-1.5 rounded-lg text-xs text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-900/20 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Rotation löschen
            </button>
            <button
              onClick={() => swapMut.mutate()}
              disabled={swapMut.isPending}
              className="px-3 py-1.5 rounded-lg text-xs font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
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

function isUsefulComponentPreset(
  schema: ReturnType<typeof getActiveSchema>,
  gearType: string,
  presetName: string,
): boolean {
  const name = presetName.trim().toLocaleLowerCase('de')
  if (!name) return false

  const scope = gearTypeScope(schema, gearType)
  const componentLabels = new Set((schema.componentTypes ?? []).map(ct => ct.labelDe.toLocaleLowerCase('de')))
  const gearTypeLabelsInScope = new Set(
    schema.gearTypes
      .filter(gt => gearTypeScope(schema, gt.key) === scope)
      .map(gt => gt.labelDe.toLocaleLowerCase('de')),
  )

  return !gearTypeLabelsInScope.has(name) || componentLabels.has(name)
}

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
  const schema = getActiveSchema()

  const { data: presets } = useQuery({
    queryKey: ['component-presets', gearType],
    queryFn: () => fetchComponentPresets(gearType),
    enabled: usePreset && !component,
  })

  function selectPreset(id: string) {
    setPresetId(id)
    const preset = presets?.find(p => p.id === id)
    if (preset) {
      setType(preset.name)
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmedType = type.trim()
    if (!trimmedType) return
    onSave({
      name: name.trim() || trimmedType,
      type: trimmedType,
      brand: brand.trim() || null,
      model: model.trim() || null,
      weight_g: weightG ? parseInt(weightG) : null,
      purchase_price: purchasePrice ? parseFloat(purchasePrice) : null,
      purchase_date: purchaseDate || null,
      serial_number: serialNumber.trim() || null,
    })
  }

  const filteredPresets = (presets ?? []).filter(p => isUsefulComponentPreset(schema, gearType, p.name))
  const groupedPresets = filteredPresets.reduce((acc, p) => {
    const cat = p.category
    if (!acc[cat]) acc[cat] = []
    acc[cat].push(p)
    return acc
  }, {} as Record<string, ComponentTypePreset[]>)

  const categoryLabels: Record<string, string> = {
    antrieb: 'Antrieb',
    drivetrain: 'Antrieb',
    bremsen: 'Bremsen',
    braking: 'Bremsen',
    reifen: 'Reifen',
    wheels: 'Laufräder',
    frame: 'Rahmen & Cockpit',
    fahrwerk: 'Fahrwerk',
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
      {!component && filteredPresets.length > 0 && (
        <div>
          <div className="flex items-center gap-2 mb-2">
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input
                type="radio"
                checked={usePreset}
                onChange={() => setUsePreset(true)}
                className="text-accent-DEFAULT"
              />
              <span className="text-neutral-700 dark:text-neutral-300">Vorlage</span>
            </label>
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input
                type="radio"
                checked={!usePreset}
                onChange={() => setUsePreset(false)}
                className="text-accent-DEFAULT"
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
          placeholder="Name"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        <input
          type="text"
          value={type}
          onChange={(e) => setType(e.target.value)}
          required
          placeholder="Komponententyp *"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        <input
          type="text"
          value={brand}
          onChange={(e) => setBrand(e.target.value)}
          placeholder="Marke"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        <input
          type="text"
          value={model}
          onChange={(e) => setModel(e.target.value)}
          placeholder="Modell"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        <input
          type="number"
          value={weightG}
          onChange={(e) => setWeightG(e.target.value)}
          placeholder="Gewicht (g)"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        <input
          type="number"
          value={purchasePrice}
          onChange={(e) => setPurchasePrice(e.target.value)}
          placeholder="Kaufpreis (EUR)"
          step="0.01"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        <input
          type="date"
          value={purchaseDate}
          onChange={(e) => setPurchaseDate(e.target.value)}
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        <input
          type="text"
          value={serialNumber}
          onChange={(e) => setSerialNumber(e.target.value)}
          placeholder="Seriennummer"
          className="border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
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
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
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
        type="button"
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
    <label className="block">
      <span className="block text-xs text-neutral-500 mb-1">{label}</span>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
      />
    </label>
  )
}

function SelectField({ label, value, onChange, options }: {
  label: string
  value: string
  onChange: (v: string) => void
  options: Array<{ value: string; label: string }>
}) {
  return (
    <label className="block">
      <span className="block text-xs text-neutral-500 mb-1">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100"
      >
        {options.map(option => (
          <option key={option.value} value={option.value}>{option.label}</option>
        ))}
      </select>
    </label>
  )
}

function jsonValue(source: Record<string, any>, ...keys: string[]): string {
  for (const key of keys) {
    const value = source[key]
    if (value !== undefined && value !== null && value !== '') return String(value)
  }
  return ''
}

function jsonString(source: Record<string, any>, key: string): string {
  const value = source[key]
  return typeof value === 'string' ? value : ''
}

function tirePressureBarValue(source: Record<string, any>, barKey: string, psiKey: string): string {
  const barValue = source[barKey]
  if (barValue !== undefined && barValue !== null && barValue !== '') return String(barValue)

  const psi = Number(source[psiKey])
  if (!Number.isFinite(psi)) return ''
  const bar = psi / 14.5037738
  return String(Math.round(bar * 100) / 100)
}

function BikeSetupsSection({ gearId }: { gearId: string }) {
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [snapshotOpen, setSnapshotOpen] = useState(false)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)

  const { data: setups } = useQuery({
    queryKey: ['bike-setups', gearId],
    queryFn: () => listBikeSetups(gearId),
  })

  // Keep selectedId in sync: default to active setup or first setup.
  const resolvedSelectedId = (() => {
    if (!setups || setups.length === 0) return null
    if (selectedId && setups.some(s => s.id === selectedId)) return selectedId
    const active = setups.find(s => s.is_active)
    return active ? active.id : setups[0].id
  })()

  const selectedSetup = setups?.find(s => s.id === resolvedSelectedId) ?? null

  const createMut = useMutation({
    mutationFn: (data: BikeSetupCreatePayload) => createBikeSetup(gearId, data),
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: ['bike-setups', gearId] })
      setCreating(false)
      setSelectedId(created.id)
    },
  })

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: BikeSetupUpdatePayload }) => updateBikeSetup(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bike-setups', gearId] })
      setEditingId(null)
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteBikeSetup,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bike-setups', gearId] })
      setSelectedId(null)
      setDeleteConfirmId(null)
    },
  })

  const activateMut = useMutation({
    mutationFn: activateBikeSetup,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['bike-setups', gearId] }),
  })

  const canDelete = selectedSetup != null &&
    (setups?.length ?? 0) > 1 &&
    !selectedSetup.is_active

  if (!setups || setups.length === 0) {
    return (
      <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">Setups</h2>
        {creating ? (
          <BikeSetupForm
            onSave={(data) => createMut.mutate(data)}
            onCancel={() => setCreating(false)}
            isSaving={createMut.isPending}
          />
        ) : (
          <div className="text-center py-4">
            <p className="text-sm text-neutral-400 mb-3">Noch kein Setup angelegt</p>
            <button
              onClick={() => setCreating(true)}
              className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 cursor-pointer"
            >
              + Neues Setup
            </button>
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-4">
      {/* Header row: dropdown + action buttons */}
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mr-1">Setups</h2>

        {/* Setup selector dropdown */}
        <select
          value={resolvedSelectedId ?? ''}
          onChange={(e) => {
            setSelectedId(e.target.value)
            setEditingId(null)
            setSnapshotOpen(false)
          }}
          className="flex-1 min-w-0 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-900 dark:text-neutral-100 cursor-pointer"
          aria-label="Setup auswählen"
        >
          {setups.map(s => (
            <option key={s.id} value={s.id}>
              {s.is_active ? `✓ ${s.name} (aktiv)` : s.name}
            </option>
          ))}
        </select>

        {/* Bearbeiten */}
        {editingId !== resolvedSelectedId && (
          <button
            onClick={() => setEditingId(resolvedSelectedId)}
            className="text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
          >
            Bearbeiten
          </button>
        )}

        {/* + Neu */}
        {!creating && (
          <button
            onClick={() => { setCreating(true); setEditingId(null) }}
            className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
          >
            + Neu
          </button>
        )}

        {/* Aktivieren — only when not already active */}
        {selectedSetup && !selectedSetup.is_active && (
          <button
            onClick={() => activateMut.mutate(selectedSetup.id)}
            disabled={activateMut.isPending}
            className="text-xs text-emerald-500 hover:text-emerald-700 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Aktivieren
          </button>
        )}

        {/* Löschen — disabled when only setup or active */}
        {deleteConfirmId === resolvedSelectedId ? (
          <span className="flex items-center gap-1 text-xs">
            <span className="text-neutral-500">Sicher?</span>
            <button
              onClick={() => selectedSetup && deleteMut.mutate(selectedSetup.id)}
              disabled={deleteMut.isPending}
              className="text-rose-600 hover:text-rose-800 cursor-pointer disabled:opacity-50"
            >
              Ja
            </button>
            <button
              onClick={() => setDeleteConfirmId(null)}
              className="text-neutral-500 hover:text-neutral-700 cursor-pointer"
            >
              Nein
            </button>
          </span>
        ) : (
          <button
            onClick={() => canDelete && setDeleteConfirmId(resolvedSelectedId)}
            disabled={!canDelete}
            title={!canDelete ? (selectedSetup?.is_active ? 'Aktives Setup kann nicht gelöscht werden' : 'Mindestens ein Setup muss vorhanden sein') : undefined}
            className="text-xs text-rose-500 hover:text-rose-700 cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
          >
            Löschen
          </button>
        )}
      </div>

      {/* Create form */}
      {creating && (
        <div className="mb-4 rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
          <BikeSetupForm
            onSave={(data) => createMut.mutate(data)}
            onCancel={() => setCreating(false)}
            isSaving={createMut.isPending}
          />
        </div>
      )}

      {/* Detail panel for selected setup */}
      {selectedSetup && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
          {editingId === selectedSetup.id ? (
            <BikeSetupForm
              setup={selectedSetup}
              onSave={(data) => updateMut.mutate({ id: selectedSetup.id, data })}
              onCancel={() => setEditingId(null)}
              isSaving={updateMut.isPending}
            />
          ) : (
            <div className="space-y-2 text-sm">
              <div className="flex items-center gap-2">
                <span className="font-medium text-neutral-900 dark:text-neutral-100">{selectedSetup.name}</span>
                {selectedSetup.is_active && (
                  <span className="px-1.5 py-0.5 rounded text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
                    Aktiv
                  </span>
                )}
              </div>

              {(selectedSetup.intended_use || selectedSetup.rider_weight_kg) && (
                <div className="text-xs text-neutral-500 dark:text-neutral-400 flex gap-3">
                  {selectedSetup.intended_use && <span>Nutzung: {selectedSetup.intended_use}</span>}
                  {selectedSetup.rider_weight_kg && <span>Fahrergewicht: {selectedSetup.rider_weight_kg} kg</span>}
                </div>
              )}

              {/* JSONB section tags */}
              <div className="flex flex-wrap gap-1.5">
                {selectedSetup.suspension_front && <SetupTag label="Gabel" />}
                {selectedSetup.suspension_rear && <SetupTag label="Dämpfer" />}
                {selectedSetup.tires && <SetupTag label="Reifen" />}
                {selectedSetup.contact_points && <SetupTag label="Kontaktpunkte" />}
                {selectedSetup.brakes && <SetupTag label="Bremsen" />}
                {selectedSetup.drivetrain && <SetupTag label="Antrieb" />}
              </div>

              {/* Component snapshot section */}
              {selectedSetup.component_snapshot && (
                <div className="mt-2">
                  <button
                    type="button"
                    onClick={() => setSnapshotOpen(o => !o)}
                    className="flex items-center gap-1 text-xs text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 cursor-pointer"
                    aria-expanded={snapshotOpen}
                  >
                    <span>{snapshotOpen ? '▼' : '▶'}</span>
                    <span>
                      Komponenten zum Zeitpunkt der Speicherung ({selectedSetup.component_snapshot.components.length})
                    </span>
                  </button>
                  {snapshotOpen && (
                    <div className="mt-1.5 rounded-lg border border-neutral-100 dark:border-neutral-800 p-2 space-y-1">
                      <p
                        className="text-xs text-neutral-500 dark:text-neutral-400 flex items-start gap-1"
                        role="note"
                        aria-label="Nur Snapshot — keine Auswirkung auf aktuell verbaute Komponenten."
                      >
                        <span>ℹ</span>
                        <span>Nur Snapshot — keine Auswirkung auf aktuell verbaute Komponenten.</span>
                      </p>
                      <ul className="space-y-0.5">
                        {selectedSetup.component_snapshot.components.map(c => (
                          <li key={c.id} className="text-xs text-neutral-700 dark:text-neutral-300">
                            <span className="font-medium">{c.name}</span>
                            <span className="text-neutral-500 dark:text-neutral-500"> · {c.type}</span>
                            {(c.brand || c.model) && (
                              <span className="text-neutral-500 dark:text-neutral-500">
                                {' · '}{[c.brand, c.model].filter(Boolean).join(' ')}
                              </span>
                            )}
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              )}

              {selectedSetup.notes && (
                <p className="text-xs text-neutral-500 dark:text-neutral-400 italic">{selectedSetup.notes}</p>
              )}
            </div>
          )}
        </div>
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
  onSave: (data: BikeSetupCreatePayload | BikeSetupUpdatePayload) => void
  onCancel: () => void
  isSaving?: boolean
}) {
  const [name, setName] = useState(setup?.name ?? '')
  const [riderWeight, setRiderWeight] = useState(setup?.rider_weight_kg?.toString() ?? '')
  const [intendedUse, setIntendedUse] = useState(setup?.intended_use ?? '')
  const [notes, setNotes] = useState(setup?.notes ?? '')
  const [isActive, setIsActive] = useState(setup?.is_active ?? false)
  const [captureComponents, setCaptureComponents] = useState(false)

  // Gabel (Federung vorne)
  const sf = setup?.suspension_front ?? {}
  const [sfTravel, setSfTravel] = useState(sf.travel_mm?.toString() ?? '')
  const [sfPressure, setSfPressure] = useState(sf.pressure_psi?.toString() ?? '')
  const [sfLsr, setSfLsr] = useState(sf.lsr?.toString() ?? '')
  const [sfHsr, setSfHsr] = useState(sf.hsr?.toString() ?? '')
  const [sfLsc, setSfLsc] = useState(sf.lsc?.toString() ?? '')
  const [sfHsc, setSfHsc] = useState(sf.hsc?.toString() ?? '')
  const [sfSag, setSfSag] = useState(sf.sag_pct?.toString() ?? '')

  // Dämpfer (Federung hinten)
  const sr = setup?.suspension_rear ?? {}
  const [srTravel, setSrTravel] = useState(sr.travel_mm?.toString() ?? '')
  const [srPressure, setSrPressure] = useState(sr.pressure_psi?.toString() ?? '')
  const [srLsr, setSrLsr] = useState(sr.lsr?.toString() ?? '')
  const [srHsr, setSrHsr] = useState(sr.hsr?.toString() ?? '')
  const [srLsc, setSrLsc] = useState(sr.lsc?.toString() ?? '')
  const [srHsc, setSrHsc] = useState(sr.hsc?.toString() ?? '')
  const [srSag, setSrSag] = useState(sr.sag_pct?.toString() ?? '')
  const [flipchipPosition, setFlipchipPosition] = useState(jsonString(sr, 'flipchip_position'))

  // Tires
  const t = setup?.tires ?? {}
  const [tireFront, setTireFront] = useState(t.front_model ?? '')
  const [tireRear, setTireRear] = useState(t.rear_model ?? '')
  const [tireFrontBar, setTireFrontBar] = useState(tirePressureBarValue(t, 'front_pressure_bar', 'front_pressure_psi'))
  const [tireRearBar, setTireRearBar] = useState(tirePressureBarValue(t, 'rear_pressure_bar', 'rear_pressure_psi'))
  const [tireFrontWidth, setTireFrontWidth] = useState(jsonValue(t, 'front_width_in', 'front_width_mm'))
  const [tireRearWidth, setTireRearWidth] = useState(jsonValue(t, 'rear_width_in', 'rear_width_mm'))

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
      capture_components: captureComponents,
      suspension_front: buildJson([
        ['travel_mm', sfTravel], ['pressure_psi', sfPressure],
        ['lsr', sfLsr], ['hsr', sfHsr],
        ['lsc', sfLsc], ['hsc', sfHsc],
        ['sag_pct', sfSag],
      ]),
      suspension_rear: buildJson([
        ['travel_mm', srTravel], ['pressure_psi', srPressure],
        ['lsr', srLsr], ['hsr', srHsr],
        ['lsc', srLsc], ['hsc', srHsc],
        ['sag_pct', srSag], ['flipchip_position', flipchipPosition],
      ]),
      tires: buildJson([
        ['front_model', tireFront], ['rear_model', tireRear],
        ['front_pressure_bar', tireFrontBar], ['rear_pressure_bar', tireRearBar],
        ['front_width_in', tireFrontWidth], ['rear_width_in', tireRearWidth],
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

      <SetupSection label="Gabel">
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <JsonField label="Federweg (mm)" value={sfTravel} onChange={setSfTravel} placeholder="150" />
          <JsonField label="Druck (PSI)" value={sfPressure} onChange={setSfPressure} placeholder="65" />
          <JsonField label="Sag (%)" value={sfSag} onChange={setSfSag} placeholder="25" />
          <JsonField label="LSR – Low Speed Rebound (Klicks)" value={sfLsr} onChange={setSfLsr} placeholder="8" />
          <JsonField label="HSR – High Speed Rebound (Klicks)" value={sfHsr} onChange={setSfHsr} placeholder="3" />
          <JsonField label="LSC – Low Speed Compression (Klicks)" value={sfLsc} onChange={setSfLsc} placeholder="5" />
          <JsonField label="HSC – High Speed Compression (Klicks)" value={sfHsc} onChange={setSfHsc} placeholder="2" />
        </div>
      </SetupSection>

      <SetupSection label="Dämpfer">
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <JsonField label="Federweg (mm)" value={srTravel} onChange={setSrTravel} placeholder="140" />
          <JsonField label="Druck (PSI)" value={srPressure} onChange={setSrPressure} placeholder="180" />
          <JsonField label="Sag (%)" value={srSag} onChange={setSrSag} placeholder="30" />
          <JsonField label="LSR – Low Speed Rebound (Klicks)" value={srLsr} onChange={setSrLsr} placeholder="6" />
          <JsonField label="HSR – High Speed Rebound (Klicks)" value={srHsr} onChange={setSrHsr} placeholder="2" />
          <JsonField label="LSC – Low Speed Compression (Klicks)" value={srLsc} onChange={setSrLsc} placeholder="4" />
          <JsonField label="HSC – High Speed Compression (Klicks)" value={srHsc} onChange={setSrHsc} placeholder="1" />
          <SelectField
            label="Flipchip-Position"
            value={flipchipPosition}
            onChange={setFlipchipPosition}
            options={[
              { value: '', label: 'Nicht gesetzt' },
              { value: 'high', label: 'High' },
              { value: 'low', label: 'Low' },
            ]}
          />
        </div>
      </SetupSection>

      <SetupSection label="Reifen">
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          <JsonField label="Vorne: Modell" value={tireFront} onChange={setTireFront} placeholder="Maxxis Minion DHF" />
          <JsonField label="Vorne: Breite (Zoll)" value={tireFrontWidth} onChange={setTireFrontWidth} placeholder="2.5" />
          <JsonField label="Vorne: Druck (bar)" value={tireFrontBar} onChange={setTireFrontBar} placeholder="1.7" />
          <JsonField label="Hinten: Modell" value={tireRear} onChange={setTireRear} placeholder="Maxxis Dissector" />
          <JsonField label="Hinten: Breite (Zoll)" value={tireRearWidth} onChange={setTireRearWidth} placeholder="2.4" />
          <JsonField label="Hinten: Druck (bar)" value={tireRearBar} onChange={setTireRearBar} placeholder="1.8" />
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
          className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT"
        />
        <span className="text-sm text-neutral-700 dark:text-neutral-300">Als aktives Setup setzen</span>
      </label>

      <label className="flex items-start gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={captureComponents}
          onChange={(e) => setCaptureComponents(e.target.checked)}
          className="mt-0.5 w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT"
        />
        <span className="text-sm text-neutral-700 dark:text-neutral-300">
          Komponenten mitsichern{' '}
          <span
            className="inline-block align-[-2px] text-neutral-400 cursor-help"
            title="Komponenten-Snapshot dient nur als Verlaufs-Information — die aktuell verbauten Komponenten werden beim Aktivieren eines Setups nicht verändert."
            aria-label="Komponenten-Snapshot dient nur als Verlaufs-Information — die aktuell verbauten Komponenten werden beim Aktivieren eines Setups nicht verändert."
          >
            <CircleHelp aria-hidden="true" size={14} />
          </span>
        </span>
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
          className="px-3 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
        >
          {isSaving ? 'Speichert...' : 'Speichern'}
        </button>
      </div>
    </form>
  )
}

// ── Tab-slot definitions (Phase 6.4 wiring) ──────────────────────────────────
//
// Tabs for GearDetailPage (wired in Phase 6.4):
//   0 – Stammdaten    (edit form, notes, weight, cost, images, documents)
//   1 – Komponenten   (ComponentsList + ComponentForm)
//   2 – Geometrie     (GearGeometryTab — Phase 6.1)
//   3 – Sportarten    (GearSportsTab — Phase 6.2, applicable_sport_predicates)
//   4 – Verschleiß    (ServiceEventsPanel — Phase 6.3)
//
// No behaviour change in Phase 5; the sections remain flat (unboxed into tabs).

// ── Helpers ─────────────────────────────────────────────────────────────────────

function formatDateShort(dateStr: string): string {
  try {
    return format(new Date(dateStr), 'dd.MM.yy', { locale: de })
  } catch {
    return dateStr
  }
}

// Schema-driven type label; falls back to hardcoded table for unknown keys.
function gearTypeLabelResolved(type: string): string {
  return schemaGearTypeLabel(getActiveSchema(), type)
}

function categoryScopeForGear(gear: Gear): GearCategoryScope {
  return gearTypeScope(getActiveSchema(), gear.type, gear.scope)
}

function subTypeLabel(subType: string): string {
  const labels: Record<string, string> = {
    mountain: 'MTB',
    road: 'Rennrad',
    gravel: 'Gravel',
    cyclocross: 'Cyclocross',
    enduro: 'Enduro',
    downhill: 'Downhill',
    cross_country: 'Cross-Country',
    trail: 'Trail',
    touring: 'Touring',
    bmx: 'BMX',
    fixie: 'Fixie',
    urban: 'Urban',
    e_bike: 'E-Bike',
    hybrid: 'Hybrid',
  }
  return labels[subType] ?? subType
}

// ── Authenticated Image ──────────────────────────────────────────────────────────

/** Renders a gear document image using an authenticated fetch (Blob URL). */
function AuthenticatedGearImage({
  docId,
  className,
  alt,
}: {
  docId: string
  className?: string
  alt?: string
}) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [error, setError] = useState(false)

  useEffect(() => {
    let url: string | null = null
    downloadGearDocumentBlob(docId)
      .then(u => { url = u; setBlobUrl(u) })
      .catch(() => setError(true))
    return () => {
      if (url) URL.revokeObjectURL(url)
    }
  }, [docId])

  if (error) {
    return (
      <div className={`flex items-center justify-center bg-neutral-100 dark:bg-neutral-800 text-neutral-400 text-xs ${className ?? ''}`}>
        Fehler
      </div>
    )
  }
  if (!blobUrl) {
    return (
      <div className={`flex items-center justify-center bg-neutral-100 dark:bg-neutral-800 ${className ?? ''}`}>
        <svg className="animate-spin h-5 w-5 text-neutral-300" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
      </div>
    )
  }
  return <img src={blobUrl} alt={alt ?? ''} className={className} />
}

// ── Component Images ─────────────────────────────────────────────────────────────

function ComponentImagesSection({ componentId }: { componentId: string }) {
  const queryClient = useQueryClient()
  const { openModal } = useModal()
  const [uploading, setUploading] = useState(false)
  const [lightboxIdx, setLightboxIdx] = useState<number | null>(null)

  const { data: documents = [] } = useQuery({
    queryKey: ['component-documents', componentId],
    queryFn: () => listComponentDocuments(componentId),
    enabled: !!componentId,
  })

  const images = documents.filter(d => d.content_type.startsWith('image/'))

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadComponentDocument(componentId, file),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['component-documents', componentId] })
      setUploading(false)
    },
    onError: () => setUploading(false),
  })

  const deleteMutation = useMutation({
    mutationFn: (docId: string) => deleteGearDocument(docId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['component-documents', componentId] }),
  })

  if (images.length === 0 && !uploading) {
    return (
      <label className="text-xs text-neutral-400 hover:text-accent-DEFAULT cursor-pointer transition-colors">
        + Foto
        <input
          type="file"
          accept="image/*"
          className="hidden"
          onChange={(e) => {
            const file = e.target.files?.[0]
            if (!file) return
            setUploading(true)
            uploadMutation.mutate(file)
          }}
        />
      </label>
    )
  }

  return (
    <div className="mt-2">
      <div className="flex items-center gap-2 flex-wrap">
        {images.map((doc, idx) => (
          <div
            key={doc.id}
            className="relative group w-16 h-16 rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 cursor-pointer"
            onClick={() => setLightboxIdx(idx)}
          >
            <AuthenticatedGearImage
              docId={doc.id}
              className="w-full h-full object-cover transition-transform group-hover:scale-105"
              alt={doc.title ?? doc.original_filename}
            />
            {/* Zoom-Indikator */}
            <div className="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity bg-black/30">
              <svg className="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35M11 19a8 8 0 100-16 8 8 0 000 16z" />
                <path strokeLinecap="round" strokeLinejoin="round" d="M11 8v6M8 11h6" />
              </svg>
            </div>
            <button
              onClick={(e) => { e.stopPropagation(); openModal({
                title: 'Foto löschen?',
                message: 'Das Foto wird unwiderruflich gelöscht.',
                buttons: [
                  { label: 'Abbrechen', variant: 'default', onClick: () => {} },
                  { label: 'Löschen', variant: 'destructive', onClick: () => deleteMutation.mutate(doc.id) },
                ],
              }) }}
              className="absolute top-0.5 right-0.5 w-5 h-5 rounded-full bg-black/60 text-white text-xs flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
            >
              ×
            </button>
          </div>
        ))}
        <label className="w-16 h-16 rounded-lg border-2 border-dashed border-neutral-200 dark:border-neutral-700 flex items-center justify-center cursor-pointer hover:border-accent-400 transition-colors text-neutral-400 hover:text-accent-DEFAULT">
          {uploading ? (
            <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
          ) : (
            <span className="text-lg">+</span>
          )}
          <input
            type="file"
            accept="image/*"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (!file) return
              setUploading(true)
              uploadMutation.mutate(file)
            }}
            disabled={uploading}
          />
        </label>
      </div>

      {lightboxIdx !== null && images.length > 0 && (
        <ImageLightbox
          images={images}
          currentIndex={lightboxIdx}
          onClose={() => setLightboxIdx(null)}
          onChange={setLightboxIdx}
        />
      )}
    </div>
  )
}

// ── Gear Images ─────────────────────────────────────────────────────────────────

function GearImagesSection({ gearId }: { gearId: string }) {
  const queryClient = useQueryClient()
  const { openModal } = useModal()
  const [lightboxIdx, setLightboxIdx] = useState<number | null>(null)
  const [uploading, setUploading] = useState(false)
  const [dragOver, setDragOver] = useState(false)

  const { data: documents = [] } = useQuery({
    queryKey: ['gear-documents', gearId],
    queryFn: () => fetchGearDocuments(gearId),
    enabled: !!gearId,
  })

  const images = documents.filter(d => d.content_type.startsWith('image/'))

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadGearDocument(gearId, file),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gear-documents', gearId] })
      setUploading(false)
    },
    onError: () => setUploading(false),
  })

  const deleteMutation = useMutation({
    mutationFn: (docId: string) => deleteGearDocument(docId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['gear-documents', gearId] }),
  })

  const handleFiles = useCallback((files: FileList | null) => {
    if (!files || files.length === 0) return
    setUploading(true)
    uploadMutation.mutate(files[0])
  }, [uploadMutation])

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    handleFiles(e.dataTransfer.files)
  }, [handleFiles])

  const handleDelete = useCallback((docId: string, e: React.MouseEvent) => {
    e.stopPropagation()
    openModal({
      title: 'Foto löschen?',
      message: 'Das Foto wird unwiderruflich gelöscht.',
      buttons: [
        { label: 'Abbrechen', variant: 'default', onClick: () => {} },
        { label: 'Löschen', variant: 'destructive', onClick: () => deleteMutation.mutate(docId) },
      ],
    })
  }, [deleteMutation, openModal])

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Bilder</h2>
        <label className="px-3 py-1.5 rounded-lg text-sm font-medium text-accent-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 transition-colors cursor-pointer">
          + Foto hinzufügen
          <input
            type="file"
            accept="image/*"
            className="hidden"
            onChange={(e) => handleFiles(e.target.files)}
            disabled={uploading}
          />
        </label>
      </div>

      {/* Drop zone + gallery */}
      <div
        onDragOver={(e) => { e.preventDefault(); setDragOver(true) }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        className={`rounded-xl border-2 border-dashed transition-colors p-3 ${
          dragOver
            ? 'border-accent-400 bg-orange-50 dark:bg-orange-900/10'
            : 'border-neutral-200 dark:border-neutral-800'
        }`}
      >
        {uploading && (
          <div className="flex items-center justify-center py-4 text-sm text-neutral-400">
            <svg className="animate-spin h-5 w-5 mr-2 text-accent-DEFAULT" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            Wird hochgeladen...
          </div>
        )}

        {images.length > 0 ? (
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
            {images.map((doc, idx) => (
              <div
                key={doc.id}
                className="relative group aspect-square rounded-lg overflow-hidden cursor-pointer"
                onClick={() => setLightboxIdx(idx)}
              >
                <AuthenticatedGearImage
                  docId={doc.id}
                  className="w-full h-full object-cover"
                  alt={doc.title ?? doc.original_filename}
                />
                <button
                  onClick={(e) => handleDelete(doc.id, e)}
                  className="absolute top-1 right-1 w-6 h-6 rounded-full bg-black/60 text-white text-xs flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
                >
                  X
                </button>
              </div>
            ))}
          </div>
        ) : !uploading ? (
          <div className="text-center py-8 text-sm text-neutral-400">
            Noch keine Bilder. Fotos hierher ziehen oder Button nutzen.
          </div>
        ) : null}
      </div>

      {uploadMutation.isError && (
        <p className="text-sm text-rose-500 mt-2">
          Upload fehlgeschlagen: {uploadMutation.error instanceof Error ? uploadMutation.error.message : 'Unbekannt'}
        </p>
      )}

      {/* Lightbox */}
      {lightboxIdx !== null && images.length > 0 && (
        <ImageLightbox
          images={images}
          currentIndex={lightboxIdx}
          onClose={() => setLightboxIdx(null)}
          onChange={setLightboxIdx}
        />
      )}
    </div>
  )
}

function ImageLightbox({ images, currentIndex, onClose, onChange }: {
  images: GearDocument[]
  currentIndex: number
  onClose: () => void
  onChange: (idx: number) => void
}) {
  const doc = images[currentIndex]

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      if (e.key === 'ArrowLeft' && currentIndex > 0) onChange(currentIndex - 1)
      if (e.key === 'ArrowRight' && currentIndex < images.length - 1) onChange(currentIndex + 1)
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [currentIndex, images.length, onClose, onChange])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80"
      onClick={onClose}
    >
      <button
        onClick={onClose}
        className="absolute top-4 right-4 w-10 h-10 rounded-full bg-white/10 text-white text-xl flex items-center justify-center hover:bg-white/20 transition-colors cursor-pointer z-10"
      >
        X
      </button>

      {currentIndex > 0 && (
        <button
          onClick={(e) => { e.stopPropagation(); onChange(currentIndex - 1) }}
          className="absolute left-4 top-1/2 -translate-y-1/2 w-10 h-10 rounded-full bg-white/10 text-white text-xl flex items-center justify-center hover:bg-white/20 transition-colors cursor-pointer z-10"
        >
          &#8249;
        </button>
      )}

      {currentIndex < images.length - 1 && (
        <button
          onClick={(e) => { e.stopPropagation(); onChange(currentIndex + 1) }}
          className="absolute right-4 top-1/2 -translate-y-1/2 w-10 h-10 rounded-full bg-white/10 text-white text-xl flex items-center justify-center hover:bg-white/20 transition-colors cursor-pointer z-10"
        >
          &#8250;
        </button>
      )}

      <AuthenticatedGearImage
        docId={doc.id}
        className="max-h-[90vh] max-w-[90vw] object-contain rounded-lg"
        alt={doc.title ?? doc.original_filename}
      />

      {images.length > 1 && (
        <div className="absolute bottom-4 text-sm text-white/60">
          {currentIndex + 1} / {images.length}
        </div>
      )}
    </div>
  )
}

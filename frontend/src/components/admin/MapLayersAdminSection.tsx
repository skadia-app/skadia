// Admin section for managing system map_layers (CRUD + bulk sort via up/down buttons).
import { useState, useCallback } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useToast } from '@/lib/toast'
import { useAdminMapLayers } from '@/hooks/useMapLayers'
import { useCapabilities } from '@/hooks/useCapabilities'
import {
  createMapLayer,
  patchMapLayer,
  deleteMapLayer,
  bulkSortMapLayers,
  type MapLayer,
  type MapLayerRole,
  type MapLayerSourceKind,
} from '@/api/mapLayers'
import { ModalShell } from '@/components/ui/ModalShell'
import ConfirmModal from '@/components/ConfirmModal'
import { ApiError } from '@/api/client'
import { featureAvailable } from '@/lib/deploymentCapabilities'

function mapLayerLabels(t: TFunction) {
  return {
    title: t('mapLayersAdmin.title'),
    description: t('mapLayersAdmin.description'),
    addButton: t('mapLayersAdmin.addButton'),
    editButton: t('mapLayersAdmin.editButton'),
    deleteButton: t('mapLayersAdmin.deleteButton'),
    upButton: t('mapLayersAdmin.upButton'),
    downButton: t('mapLayersAdmin.downButton'),
    systemBadge: t('mapLayersAdmin.systemBadge'),
    disabledBadge: t('mapLayersAdmin.disabledBadge'),
    capWarning: t('mapLayersAdmin.capWarning'),
    modalAddTitle: t('mapLayersAdmin.modalAddTitle'),
    modalEditTitle: t('mapLayersAdmin.modalEditTitle'),
    modalSave: t('mapLayersAdmin.modalSave'),
    modalSaving: t('mapLayersAdmin.modalSaving'),
    modalCancel: t('common.cancel'),
    deleteConfirmTitle: t('mapLayersAdmin.deleteConfirmTitle'),
    deleteConfirmMsg: (name: string) => t('mapLayersAdmin.deleteConfirmMsg', { name }),
    deleteConfirmLabel: t('mapLayersAdmin.deleteConfirmLabel'),
    toastCreated: t('mapLayersAdmin.toasts.created'),
    toastUpdated: t('mapLayersAdmin.toasts.updated'),
    toastDeleted: t('mapLayersAdmin.toasts.deleted'),
    toastSortSaved: t('mapLayersAdmin.toasts.sortSaved'),
    toastError: t('common.viewLoadError'),
    toastOutdated: t('mapLayersAdmin.toasts.outdated'),
    toastFieldImmutable: t('mapLayersAdmin.toasts.fieldImmutable'),
    toastPrivateAddress: t('mapLayersAdmin.toasts.privateAddress'),
    fieldName: t('mapLayersAdmin.fields.name'),
    fieldRole: t('mapLayersAdmin.fields.role'),
    fieldRoleBase: t('mapLayersAdmin.fields.roleBase'),
    fieldRoleOverlay: t('mapLayersAdmin.fields.roleOverlay'),
    fieldSourceKind: t('mapLayersAdmin.fields.sourceKind'),
    sourceKindRasterTiles: t('mapLayersAdmin.sourceKinds.rasterTiles'),
    sourceKindVectorStyle: t('mapLayersAdmin.sourceKinds.vectorStyle'),
    sourceKindProtomapsOffline: t('mapLayersAdmin.sourceKinds.protomapsOffline'),
    fieldUrlTemplate: t('mapLayersAdmin.fields.urlTemplate'),
    fieldStyleUrl: t('mapLayersAdmin.fields.styleUrl'),
    fieldAttribution: t('mapLayersAdmin.fields.attribution'),
    fieldMinZoom: t('mapLayersAdmin.fields.minZoom'),
    fieldMaxZoom: t('mapLayersAdmin.fields.maxZoom'),
    fieldDefaultOpacity: t('mapLayersAdmin.fields.defaultOpacity'),
    fieldEnabled: t('mapLayersAdmin.fields.enabled'),
    fieldRequiresCap: t('mapLayersAdmin.fields.requiresCapability'),
    fieldCapNone: t('mapLayersAdmin.fields.capabilityNone'),
    loading: t('mapLayersAdmin.loading'),
    systemDeleteDisabled: t('mapLayersAdmin.systemDeleteDisabled'),
    requiresCapabilityTitle: (capability: string | null | undefined) => t('mapLayersAdmin.requiresCapabilityTitle', { capability: capability ?? '' }),
  }
}

type MapLayerLabels = ReturnType<typeof mapLayerLabels>

// ── Layer form state ──────────────────────────────────────────────────────────

interface LayerFormState {
  name: string
  role: MapLayerRole
  source_kind: MapLayerSourceKind
  url_template: string
  style_url: string
  attribution: string
  min_zoom: string
  max_zoom: string
  default_opacity_pct: string  // 0–100, converted on submit
  enabled: boolean
  requires_capability: string  // empty = none
}

function emptyForm(): LayerFormState {
  return {
    name: '',
    role: 'base',
    source_kind: 'raster_tiles',
    url_template: '',
    style_url: '',
    attribution: '',
    min_zoom: '0',
    max_zoom: '22',
    default_opacity_pct: '100',
    enabled: true,
    requires_capability: '',
  }
}

function layerToForm(l: MapLayer): LayerFormState {
  return {
    name: l.name,
    role: l.role,
    source_kind: l.source_kind,
    url_template: l.url_template ?? '',
    style_url: l.style_url ?? '',
    attribution: l.attribution ?? '',
    min_zoom: l.min_zoom != null ? String(l.min_zoom) : '',
    max_zoom: l.max_zoom != null ? String(l.max_zoom) : '',
    default_opacity_pct: String(l.default_opacity ?? 100),
    enabled: l.enabled,
    requires_capability: l.requires_capability ?? '',
  }
}

function formToBody(f: LayerFormState): Partial<MapLayer> {
  const urlTemplate = f.url_template.trim()
  const styleUrl = f.style_url.trim()
  const body: Partial<MapLayer> = {
    name: f.name,
    role: f.role,
    source_kind: f.source_kind,
    attribution: f.attribution || undefined,
    min_zoom: f.min_zoom !== '' ? Number(f.min_zoom) : undefined,
    max_zoom: f.max_zoom !== '' ? Number(f.max_zoom) : undefined,
    default_opacity: Number(f.default_opacity_pct),
    enabled: f.enabled,
    requires_capability: f.requires_capability || undefined,
  }
  if (f.source_kind === 'raster_tiles') {
    body.url_template = urlTemplate || undefined
    body.style_url = undefined
  } else {
    body.style_url = styleUrl || undefined
    body.url_template = undefined
  }
  return body
}

// ── Layer modal ───────────────────────────────────────────────────────────────

interface LayerModalProps {
  layer: MapLayer | null  // null = create mode
  capabilityKeys: string[]
  labels: MapLayerLabels
  onClose: () => void
  onSaved: () => void
}

function LayerModal({ layer, capabilityKeys, labels, onClose, onSaved }: LayerModalProps) {
  const { addToast } = useToast()
  const qc = useQueryClient()
  const [form, setForm] = useState<LayerFormState>(() =>
    layer ? layerToForm(layer) : emptyForm(),
  )

  function patch(update: Partial<LayerFormState>) {
    setForm(prev => ({ ...prev, ...update }))
  }

  const saveMut = useMutation({
    mutationFn: async () => {
      if (layer) {
        // Build a diff-only patch: compare form values against the original
        // layer and send only changed fields. key, source_kind, and role are
        // immutable server-side — always strip them from PATCH bodies.
        const full = formToBody(form)
        const original = layerToForm(layer)
        const patch: Partial<MapLayer> = {}

        if (form.name !== original.name) patch.name = full.name
        if (form.attribution !== original.attribution) patch.attribution = full.attribution
        if (form.min_zoom !== original.min_zoom) patch.min_zoom = full.min_zoom
        if (form.max_zoom !== original.max_zoom) patch.max_zoom = full.max_zoom
        if (form.default_opacity_pct !== original.default_opacity_pct) patch.default_opacity = full.default_opacity
        if (form.enabled !== original.enabled) patch.enabled = full.enabled
        if (form.requires_capability !== original.requires_capability) patch.requires_capability = full.requires_capability

        // URL fields require the If-Match header on change
        const urlChanged =
          form.url_template.trim() !== (layer.url_template ?? '') ||
          form.style_url.trim() !== (layer.style_url ?? '')
        if (urlChanged) {
          patch.url_template = full.url_template
          patch.style_url = full.style_url
        }

        // Only send the request when there is actually something to change.
        // An empty PATCH body would either 200 no-op or 400 depending on backend.
        if (Object.keys(patch).length > 0) {
          await patchMapLayer(layer.id, patch, urlChanged ? layer.updated_at : undefined)
        }
      } else {
        await createMapLayer(formToBody(form))
      }
    },
    onSuccess: () => {
      addToast({ type: 'success', title: layer ? labels.toastUpdated : labels.toastCreated })
      qc.invalidateQueries({ queryKey: ['map-layers'] })
      qc.invalidateQueries({ queryKey: ['admin-map-layers'] })
      onSaved()
      onClose()
    },
    onError: (err: unknown) => {
      if (err instanceof ApiError) {
        // Use structured JSON body for precise discrimination
        const code = typeof err.json?.error === 'string' ? err.json.error : ''
        if (err.status === 412 || err.status === 428) {
          addToast({ type: 'error', title: labels.toastOutdated, duration: 0 })
          qc.invalidateQueries({ queryKey: ['map-layers'] })
          qc.invalidateQueries({ queryKey: ['admin-map-layers'] })
          onClose()
          return
        }
        if (code === 'field_immutable') {
          addToast({ type: 'error', title: labels.toastFieldImmutable, duration: 0 })
          return
        }
        if (code === 'private_address_blocked') {
          addToast({ type: 'error', title: labels.toastPrivateAddress, duration: 0 })
          return
        }
        addToast({ type: 'error', title: labels.toastError, message: err.body, duration: 0 })
        return
      }
      // Fallback: string-match for non-ApiError throws (network errors etc.)
      const msg = err instanceof Error ? err.message : String(err)
      if (msg.includes('field_immutable')) {
        addToast({ type: 'error', title: labels.toastFieldImmutable, duration: 0 })
        return
      }
      addToast({ type: 'error', title: labels.toastError, message: msg, duration: 0 })
    },
  })

  const isVectorSource = form.source_kind !== 'raster_tiles'

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-lg">
      <div className="p-5 space-y-4">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {layer ? labels.modalEditTitle : labels.modalAddTitle}
        </h2>

        {/* Name */}
        <div className="space-y-1">
          <label htmlFor="layer-modal-name" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {labels.fieldName}
          </label>
          <input
            id="layer-modal-name"
            type="text"
            value={form.name}
            onChange={e => patch({ name: e.target.value })}
            className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
          />
        </div>

        {/* Role */}
        <fieldset className="space-y-1">
          <legend className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {labels.fieldRole}
          </legend>
          <div className="flex gap-4">
            {(['base', 'overlay'] as MapLayerRole[]).map(r => (
              <label key={r} className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="role"
                  value={r}
                  checked={form.role === r}
                  onChange={() => patch({ role: r })}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-sm text-neutral-700 dark:text-neutral-300">
                  {r === 'base' ? labels.fieldRoleBase : labels.fieldRoleOverlay}
                </span>
              </label>
            ))}
          </div>
        </fieldset>

        {/* Source kind */}
        <div className="space-y-1">
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {labels.fieldSourceKind}
          </label>
          <select
            value={form.source_kind}
            onChange={e => patch({ source_kind: e.target.value as MapLayerSourceKind })}
            className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
          >
            <option value="raster_tiles">{labels.sourceKindRasterTiles}</option>
            <option value="vector_style">{labels.sourceKindVectorStyle}</option>
            <option value="protomaps_offline">{labels.sourceKindProtomapsOffline}</option>
          </select>
        </div>

        {/* URL — conditional on source_kind */}
        {!isVectorSource ? (
          <div className="space-y-1">
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {labels.fieldUrlTemplate}
            </label>
            <input
              type="url"
              value={form.url_template}
              onChange={e => patch({ url_template: e.target.value })}
              placeholder="https://tile.example.com/{z}/{x}/{y}.png"
              className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
            />
          </div>
        ) : (
          <div className="space-y-1">
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {labels.fieldStyleUrl}
            </label>
            <input
              type="url"
              value={form.style_url}
              onChange={e => patch({ style_url: e.target.value })}
              placeholder="https://example.com/style.json"
              className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
            />
          </div>
        )}

        {/* Attribution */}
        <div className="space-y-1">
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {labels.fieldAttribution}
          </label>
          <textarea
            value={form.attribution}
            onChange={e => patch({ attribution: e.target.value })}
            rows={2}
            className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT resize-none"
          />
        </div>

        {/* Zoom range */}
        <div className="flex gap-3">
          <div className="flex-1 space-y-1">
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {labels.fieldMinZoom}
            </label>
            <input
              type="number"
              min={0}
              max={24}
              value={form.min_zoom}
              onChange={e => patch({ min_zoom: e.target.value })}
              className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
            />
          </div>
          <div className="flex-1 space-y-1">
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {labels.fieldMaxZoom}
            </label>
            <input
              type="number"
              min={0}
              max={24}
              value={form.max_zoom}
              onChange={e => patch({ max_zoom: e.target.value })}
              className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
            />
          </div>
        </div>

        {/* Default opacity */}
        <div className="space-y-1">
          <label className="flex items-center justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300">
            <span>{labels.fieldDefaultOpacity}</span>
            <span className="tabular-nums text-neutral-500">{form.default_opacity_pct}%</span>
          </label>
          <input
            type="range"
            min={0}
            max={100}
            value={form.default_opacity_pct}
            onChange={e => patch({ default_opacity_pct: e.target.value })}
            className="w-full accent-accent-DEFAULT"
          />
        </div>

        {/* Requires capability */}
        <div className="space-y-1">
          <label htmlFor="layer-modal-cap" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {labels.fieldRequiresCap}
          </label>
          <select
            id="layer-modal-cap"
            value={form.requires_capability}
            onChange={e => patch({ requires_capability: e.target.value })}
            className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
          >
            <option value="">{labels.fieldCapNone}</option>
            {capabilityKeys.map(k => (
              <option key={k} value={k}>{k}</option>
            ))}
          </select>
        </div>

        {/* Enabled */}
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={form.enabled}
            onChange={e => patch({ enabled: e.target.checked })}
            className="h-4 w-4 rounded border-neutral-300 accent-accent-DEFAULT"
          />
          <span className="text-sm text-neutral-700 dark:text-neutral-300">{labels.fieldEnabled}</span>
        </label>

        {/* Actions */}
        <div className="flex gap-2 pt-2 border-t border-neutral-200 dark:border-neutral-700">
          <button
            type="button"
            onClick={() => saveMut.mutate()}
            disabled={saveMut.isPending || !form.name.trim()}
            className="cursor-pointer rounded-lg px-4 py-2 text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {saveMut.isPending ? labels.modalSaving : labels.modalSave}
          </button>
          <button
            type="button"
            onClick={onClose}
            disabled={saveMut.isPending}
            className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {labels.modalCancel}
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function MapLayersAdminSection() {
  const { t } = useTranslation()
  const labels = mapLayerLabels(t)
  const { addToast } = useToast()
  const qc = useQueryClient()
  const { raw: layers, isLoading } = useAdminMapLayers()
  const { data: caps } = useCapabilities()

  const [showModal, setShowModal] = useState(false)
  const [modalLayer, setModalLayer] = useState<MapLayer | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<MapLayer | null>(null)

  // Sorted copy for display
  const sorted = [...layers].sort((a, b) => a.sort_order - b.sort_order)

  const capabilityKeys = caps
    ? Array.from(new Set([
        ...Object.keys(caps.features ?? {}),
        ...Object.keys(caps).filter(k => typeof caps[k] === 'boolean'),
      ])).sort()
    : []

  const deleteMut = useMutation({
    mutationFn: (id: number) => deleteMapLayer(id),
    onSuccess: () => {
      addToast({ type: 'success', title: labels.toastDeleted })
      qc.invalidateQueries({ queryKey: ['map-layers'] })
      qc.invalidateQueries({ queryKey: ['admin-map-layers'] })
      setDeleteTarget(null)
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : String(err)
      addToast({ type: 'error', title: labels.toastError, message: msg, duration: 0 })
    },
  })

  const sortMut = useMutation({
    mutationFn: bulkSortMapLayers,
    onSuccess: () => {
      addToast({ type: 'success', title: labels.toastSortSaved })
      qc.invalidateQueries({ queryKey: ['map-layers'] })
      qc.invalidateQueries({ queryKey: ['admin-map-layers'] })
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : String(err)
      addToast({ type: 'error', title: labels.toastError, message: msg, duration: 0 })
    },
  })

  const moveLayer = useCallback(
    (idx: number, direction: -1 | 1) => {
      const next = [...sorted]
      const swapIdx = idx + direction
      if (swapIdx < 0 || swapIdx >= next.length) return
      // Swap sort orders
      const aOrder = next[idx].sort_order
      const bOrder = next[swapIdx].sort_order
      next[idx] = { ...next[idx], sort_order: bOrder }
      next[swapIdx] = { ...next[swapIdx], sort_order: aOrder }
      sortMut.mutate([
        { id: next[idx].id, sort_order: bOrder },
        { id: next[swapIdx].id, sort_order: aOrder },
      ])
    },
    [sorted, sortMut],
  )

  function capSatisfied(l: MapLayer): boolean {
    if (!l.requires_capability) return true
    if (!caps) return false
    return featureAvailable(caps, l.requires_capability)
  }

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <p className="text-sm text-neutral-400">{labels.loading}</p>
      </div>
    )
  }

  return (
    <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{labels.title}</h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">{labels.description}</p>
        </div>
        <button
          type="button"
          onClick={() => { setModalLayer(null); setShowModal(true) }}
          className="shrink-0 cursor-pointer rounded-lg px-3 py-1.5 text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
        >
          {labels.addButton}
        </button>
      </div>

      {/* Layer list */}
      <ul className="space-y-2">
        {sorted.map((layer, idx) => {
          const satisfied = capSatisfied(layer)
          return (
            <li
              key={layer.id}
              className="flex items-center gap-2 rounded-lg border border-neutral-200 dark:border-neutral-700 px-3 py-2"
            >
              {/* Sort buttons */}
              <div className="flex flex-col gap-0.5 shrink-0">
                <button
                  type="button"
                  disabled={idx === 0 || sortMut.isPending}
                  onClick={() => moveLayer(idx, -1)}
                  aria-label={labels.upButton}
                  className="p-0.5 text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 disabled:opacity-30 disabled:cursor-not-allowed cursor-pointer"
                >
                  ▲
                </button>
                <button
                  type="button"
                  disabled={idx === sorted.length - 1 || sortMut.isPending}
                  onClick={() => moveLayer(idx, 1)}
                  aria-label={labels.downButton}
                  className="p-0.5 text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 disabled:opacity-30 disabled:cursor-not-allowed cursor-pointer"
                >
                  ▼
                </button>
              </div>

              {/* Layer info */}
              <div className="flex-1 min-w-0 space-y-0.5">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                    {layer.name}
                  </span>
                  {layer.is_system && (
                    <span className="text-xs px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-500">
                      {labels.systemBadge}
                    </span>
                  )}
                  {!layer.enabled && (
                    <span className="text-xs px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400">
                      {labels.disabledBadge}
                    </span>
                  )}
                  {!satisfied && (
                    <span
                      className="text-xs px-1.5 py-0.5 rounded bg-red-100 dark:bg-red-900/30 text-red-600 dark:text-red-400"
                      title={labels.requiresCapabilityTitle(layer.requires_capability)}
                    >
                      ⚠️ {labels.capWarning}
                    </span>
                  )}
                  {satisfied && layer.requires_capability && (
                    <span className="text-xs px-1.5 py-0.5 rounded bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400">
                      ✅ {layer.requires_capability}
                    </span>
                  )}
                </div>
                <p className="text-xs text-neutral-400 dark:text-neutral-500 truncate">
                  {layer.source_kind} · {layer.role} · {layer.url_template ?? layer.style_url ?? '—'}
                </p>
              </div>

              {/* Actions */}
              <div className="flex items-center gap-1 shrink-0">
                <button
                  type="button"
                  onClick={() => { setModalLayer(layer); setShowModal(true) }}
                  className="cursor-pointer rounded px-2 py-1 text-xs font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
                >
                  {labels.editButton}
                </button>
                <button
                  type="button"
                  disabled={layer.is_system}
                  onClick={() => setDeleteTarget(layer)}
                  title={layer.is_system ? labels.systemDeleteDisabled : undefined}
                  className="cursor-pointer rounded px-2 py-1 text-xs font-medium text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  {labels.deleteButton}
                </button>
              </div>
            </li>
          )
        })}
      </ul>

      {/* Edit / Create modal */}
      {showModal && (
        <LayerModal
          layer={modalLayer}
          capabilityKeys={capabilityKeys}
          labels={labels}
          onClose={() => setShowModal(false)}
          onSaved={() => setShowModal(false)}
        />
      )}

      {/* Delete confirm */}
      {deleteTarget && (
        <ConfirmModal
          title={labels.deleteConfirmTitle}
          message={labels.deleteConfirmMsg(deleteTarget.name)}
          confirmLabel={labels.deleteConfirmLabel}
          isPending={deleteMut.isPending}
          onConfirm={() => deleteMut.mutate(deleteTarget.id)}
          onClose={() => setDeleteTarget(null)}
        />
      )}
    </section>
  )
}

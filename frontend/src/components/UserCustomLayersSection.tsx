// User-managed custom tile-URL layers section in Settings → Karte tab.
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useToast } from '@/lib/toast'
import { useUserMapLayers, useUserMapLayersMutations } from '@/hooks/useUserMapLayers'
import { useSettingsLimits } from '@/hooks/useSettingsLimits'
import { type UserMapLayer } from '@/api/mapLayers'
import { ModalShell } from '@/components/ui/ModalShell'
import ConfirmModal from '@/components/ConfirmModal'
import { ApiError } from '@/api/client'

// ── i18n-ready labels ─────────────────────────────────────────────────────────

function useCustomLayerLabels() {
  const { t } = useTranslation()
  return {
    title: t('components.userCustomLayers.title'),
    description: t('components.userCustomLayers.description'),
    limitDisplay: (used: number, max: number) => t('components.userCustomLayers.limitDisplay', { used, max }),
    addButton: t('components.userCustomLayers.addButton'),
    editButton: t('components.userCustomLayers.editButton'),
    deleteButton: t('common.delete'),
    upButton: t('components.userCustomLayers.moveUp'),
    downButton: t('components.userCustomLayers.moveDown'),
    modalAddTitle: t('components.userCustomLayers.modal.addTitle'),
    modalEditTitle: t('components.userCustomLayers.modal.editTitle'),
    modalSave: t('common.save'),
    modalSaving: t('components.userCustomLayers.modal.saving'),
    modalCancel: t('common.cancel'),
    fieldName: t('components.userCustomLayers.fields.name'),
    fieldRole: t('components.userCustomLayers.fields.role'),
    fieldRoleBase: t('components.userCustomLayers.roles.base'),
    fieldRoleBaseHint: t('components.userCustomLayers.roles.baseHint'),
    fieldRoleOverlay: t('components.userCustomLayers.roles.overlay'),
    fieldRoleOverlayHint: t('components.userCustomLayers.roles.overlayHint'),
    fieldSourceKind: t('components.userCustomLayers.fields.sourceKind'),
    fieldUrlTemplate: t('components.userCustomLayers.fields.urlTemplate'),
    fieldUrlTemplatePlaceholder: t('components.userCustomLayers.placeholders.urlTemplate'),
    fieldStyleUrl: t('components.userCustomLayers.fields.styleUrl'),
    fieldStyleUrlPlaceholder: t('components.userCustomLayers.placeholders.styleUrl'),
    fieldUrlEmptyHint: t('components.userCustomLayers.validation.urlRequired'),
    fieldAttribution: t('components.userCustomLayers.fields.attribution'),
    fieldMinZoom: t('components.userCustomLayers.fields.minZoom'),
    fieldMaxZoom: t('components.userCustomLayers.fields.maxZoom'),
    fieldDefaultOpacity: t('components.userCustomLayers.fields.defaultOpacity'),
    privacyTitle: t('components.userCustomLayers.privacy.title'),
    privacyLine1: t('components.userCustomLayers.privacy.line1'),
    privacyLine3: t('components.userCustomLayers.privacy.line2'),
    deleteConfirmTitle: t('components.userCustomLayers.deleteConfirm.title'),
    deleteConfirmMsg: (name: string) => t('components.userCustomLayers.deleteConfirm.message', { name }),
    deleteConfirmLabel: t('common.delete'),
    toastCreated: t('components.userCustomLayers.toasts.created'),
    toastUpdated: t('components.userCustomLayers.toasts.updated'),
    toastDeleted: t('components.userCustomLayers.toasts.deleted'),
    toastError: t('common.error'),
    toast422PrivateIP: t('components.userCustomLayers.toasts.privateIp'),
    toast409Duplicate: t('components.userCustomLayers.toasts.duplicate'),
    toastFieldImmutable: t('components.userCustomLayers.toasts.fieldImmutable'),
    toast422LimitReached: (used: number, max: number) => t('components.userCustomLayers.toasts.limitReached', { used, max }),
    toast429RateLimit: (retryAfter: string) => t('components.userCustomLayers.toasts.rateLimit', { retryAfter }),
    limitReachedHint: t('components.userCustomLayers.limitReachedHint'),
    toggleActiveLabel: (name: string) => t('components.userCustomLayers.toggleActive', { name }),
    badgeBase: t('components.userCustomLayers.badges.base'),
    badgeOverlay: t('components.userCustomLayers.badges.overlay'),
    badgeRaster: t('components.userCustomLayers.badges.raster'),
    badgeVector: t('components.userCustomLayers.badges.vector'),
    sourceRaster: t('components.userCustomLayers.sourceKinds.raster'),
    sourceVector: t('components.userCustomLayers.sourceKinds.vector'),
    loading: t('components.userCustomLayers.loading'),
  }
}

// ── Form state ────────────────────────────────────────────────────────────────

interface LayerFormState {
  name: string
  role: 'base' | 'overlay'
  source_kind: 'raster_tiles' | 'vector_style'
  url_template: string
  style_url: string
  attribution: string
  min_zoom: string
  max_zoom: string
  default_opacity_pct: string
}

function emptyForm(): LayerFormState {
  return {
    name: '',
    role: 'overlay',
    source_kind: 'raster_tiles',
    url_template: '',
    style_url: '',
    attribution: '',
    min_zoom: '0',
    max_zoom: '22',
    default_opacity_pct: '100',
  }
}

function layerToForm(l: UserMapLayer): LayerFormState {
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
  }
}

function formToBody(f: LayerFormState): Partial<UserMapLayer> {
  const body: Partial<UserMapLayer> = {
    name: f.name,
    role: f.role,
    source_kind: f.source_kind,
    attribution: f.attribution || undefined,
    min_zoom: f.min_zoom !== '' ? Number(f.min_zoom) : undefined,
    max_zoom: f.max_zoom !== '' ? Number(f.max_zoom) : undefined,
    default_opacity: Number(f.default_opacity_pct),
  }
  // Send only the relevant URL field; clear the other so the backend stays consistent.
  if (f.source_kind === 'raster_tiles') {
    body.url_template = f.url_template.trim() || undefined
    body.style_url = undefined
  } else {
    body.style_url = f.style_url.trim() || undefined
    body.url_template = undefined
  }
  return body
}

// ── Error classification ──────────────────────────────────────────────────────

// Server error-body shape produced by `mapLayerErrBody` and the limit_reached
// handler in services/api/internal/handler/user_map_layers.go: the discriminator
// lives in the `error` field, not `error_code`.
interface MapLayerErrorBody {
  error?: string
  limit?: number
  message?: string
}

function classifyError(err: unknown): {
  kind: 'private_ip' | 'duplicate' | 'limit_reached' | 'rate_limit' | 'field_immutable' | 'generic'
  retryAfter?: string
  limit?: number
} {
  // Preferred path: ApiError carries structured `status` + parsed `json` body.
  if (err instanceof ApiError) {
    const body = (err.json ?? {}) as MapLayerErrorBody
    const code = typeof body.error === 'string' ? body.error : ''
    if (err.status === 422 && code === 'private_address_blocked') {
      return { kind: 'private_ip' }
    }
    if (err.status === 409 || code === 'duplicate_name') {
      return { kind: 'duplicate' }
    }
    if (err.status === 422 && code === 'limit_reached') {
      return {
        kind: 'limit_reached',
        limit: typeof body.limit === 'number' ? body.limit : undefined,
      }
    }
    if (err.status === 422 && code === 'field_immutable') {
      return { kind: 'field_immutable' }
    }
    if (err.status === 429) {
      // Retry-After header isn't currently exposed on ApiError; fall back to
      // a sane default. If/when the client surfaces headers, plumb it here.
      return { kind: 'rate_limit', retryAfter: '60' }
    }
    return { kind: 'generic' }
  }
  // Fallback for non-ApiError throws (network errors, legacy call sites that
  // wrap with plain Error). Keep the substring matchers so we degrade
  // gracefully rather than swallowing the signal.
  const msg = err instanceof Error ? err.message : String(err)
  if (msg.includes('422') && msg.includes('private_address_blocked')) {
    return { kind: 'private_ip' }
  }
  if (msg.includes('409') || msg.includes('duplicate_name') || msg.includes('Name bereits')) {
    return { kind: 'duplicate' }
  }
  if (msg.includes('422') && msg.includes('limit_reached')) {
    return { kind: 'limit_reached' }
  }
  if (msg.includes('field_immutable')) {
    return { kind: 'field_immutable' }
  }
  if (msg.includes('429') || msg.includes('rate limit')) {
    const m = msg.match(/Retry-After[:\s]+(\d+)/i)
    return { kind: 'rate_limit', retryAfter: m ? m[1] : '60' }
  }
  return { kind: 'generic' }
}

// ── Layer modal ───────────────────────────────────────────────────────────────

interface LayerModalProps {
  layer: UserMapLayer | null
  usedCount: number
  maxCount: number
  onClose: () => void
}

function LayerModal({ layer, usedCount, maxCount, onClose }: LayerModalProps) {
  const LABELS = useCustomLayerLabels()
  const { addToast } = useToast()
  const qc = useQueryClient()
  const mutations = useUserMapLayersMutations()
  const [form, setForm] = useState<LayerFormState>(() =>
    layer ? layerToForm(layer) : emptyForm(),
  )

  function patch(update: Partial<LayerFormState>) {
    setForm(prev => ({ ...prev, ...update }))
  }

  function handleError(err: unknown) {
    const classification = classifyError(err)
    switch (classification.kind) {
      case 'private_ip':
        addToast({ type: 'error', title: LABELS.toast422PrivateIP, duration: 0 })
        break
      case 'duplicate':
        addToast({ type: 'error', title: LABELS.toast409Duplicate, duration: 0 })
        break
      case 'limit_reached': {
        const lim = classification.limit ?? maxCount
        addToast({ type: 'error', title: LABELS.toast422LimitReached(usedCount, lim), duration: 0 })
        onClose()
        break
      }
      case 'rate_limit':
        addToast({
          type: 'error',
          title: LABELS.toast429RateLimit(classification.retryAfter ?? '60'),
          duration: 0,
        })
        break
      case 'field_immutable':
        addToast({ type: 'error', title: LABELS.toastFieldImmutable, duration: 0 })
        break
      default: {
        const msg = err instanceof Error ? err.message : String(err)
        addToast({ type: 'error', title: LABELS.toastError, message: msg, duration: 0 })
      }
    }
  }

  const isPending = mutations.create.isPending || mutations.update.isPending

  // Determine which URL field is active and whether it's filled.
  const isVectorSource = form.source_kind === 'vector_style'
  const activeUrlEmpty = isVectorSource ? !form.style_url.trim() : !form.url_template.trim()
  const canSubmit = !isPending && form.name.trim().length > 0 && !activeUrlEmpty

  function handleSave() {
    const body = formToBody(form)
    if (layer) {
      mutations.update.mutate(
        { id: layer.id, body },
        {
          onSuccess: () => {
            addToast({ type: 'success', title: LABELS.toastUpdated })
            qc.invalidateQueries({ queryKey: ['user-map-layers'] })
            onClose()
          },
          onError: handleError,
        },
      )
    } else {
      // New layers default to enabled so they appear on the map immediately.
      mutations.create.mutate({ ...body, enabled: true }, {
        onSuccess: () => {
          addToast({ type: 'success', title: LABELS.toastCreated })
          qc.invalidateQueries({ queryKey: ['user-map-layers'] })
          onClose()
        },
        onError: handleError,
      })
    }
  }

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md">
      <div className="p-5 space-y-4">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {layer ? LABELS.modalEditTitle : LABELS.modalAddTitle}
        </h2>

        {/* Privacy hint — always visible in add/edit dialog */}
        <div className="rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 px-3 py-2.5 space-y-1">
          <p className="text-xs font-semibold text-amber-800 dark:text-amber-400">{LABELS.privacyTitle}</p>
          <p className="text-xs text-amber-700 dark:text-amber-300 leading-relaxed">{LABELS.privacyLine1}</p>
          <p className="text-xs text-amber-700 dark:text-amber-300 leading-relaxed">{LABELS.privacyLine3}</p>
        </div>

        {/* Name */}
        <div className="space-y-1">
          <label htmlFor="user-layer-modal-name" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {LABELS.fieldName}
          </label>
          <input
            id="user-layer-modal-name"
            type="text"
            value={form.name}
            onChange={e => patch({ name: e.target.value })}
            className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
          />
        </div>

        {/* Role — radio group */}
        <fieldset className="space-y-1">
          <legend className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {LABELS.fieldRole}
          </legend>
          <div className="flex gap-4">
            {(['overlay', 'base'] as const).map(r => (
              <label key={r} className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="user-layer-role"
                  value={r}
                  checked={form.role === r}
                  onChange={() => patch({ role: r })}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-sm text-neutral-700 dark:text-neutral-300">
                  {r === 'base' ? LABELS.fieldRoleBase : LABELS.fieldRoleOverlay}
                </span>
              </label>
            ))}
          </div>
          <p className="text-xs text-neutral-400 dark:text-neutral-500">
            {form.role === 'base' ? LABELS.fieldRoleBaseHint : LABELS.fieldRoleOverlayHint}
          </p>
        </fieldset>

        {/* Source kind — dropdown */}
        <div className="space-y-1">
          <label htmlFor="user-layer-modal-source-kind" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {LABELS.fieldSourceKind}
          </label>
          <select
            id="user-layer-modal-source-kind"
            value={form.source_kind}
            onChange={e => {
              // Clear both URL fields when switching to avoid stale values carrying over.
              patch({
                source_kind: e.target.value as 'raster_tiles' | 'vector_style',
                url_template: '',
                style_url: '',
              })
            }}
            className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
          >
            <option value="raster_tiles">{LABELS.sourceRaster}</option>
            <option value="vector_style">{LABELS.sourceVector}</option>
          </select>
        </div>

        {/* URL — switches based on source_kind */}
        {!isVectorSource ? (
          <div className="space-y-1">
            <label htmlFor="user-layer-modal-url-template" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {LABELS.fieldUrlTemplate}
            </label>
            <input
              id="user-layer-modal-url-template"
              type="url"
              value={form.url_template}
              onChange={e => patch({ url_template: e.target.value })}
              placeholder={LABELS.fieldUrlTemplatePlaceholder}
              className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
            />
            {!form.url_template.trim() && (
              <p className="text-xs text-red-500 dark:text-red-400">{LABELS.fieldUrlEmptyHint}</p>
            )}
          </div>
        ) : (
          <div className="space-y-1">
            <label htmlFor="user-layer-modal-style-url" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {LABELS.fieldStyleUrl}
            </label>
            <input
              id="user-layer-modal-style-url"
              type="url"
              value={form.style_url}
              onChange={e => patch({ style_url: e.target.value })}
              placeholder={LABELS.fieldStyleUrlPlaceholder}
              className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
            />
            {!form.style_url.trim() && (
              <p className="text-xs text-red-500 dark:text-red-400">{LABELS.fieldUrlEmptyHint}</p>
            )}
          </div>
        )}

        {/* Attribution */}
        <div className="space-y-1">
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {LABELS.fieldAttribution}
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
              {LABELS.fieldMinZoom}
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
              {LABELS.fieldMaxZoom}
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

        {/* Opacity */}
        <div className="space-y-1">
          <label className="flex items-center justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300">
            <span>{LABELS.fieldDefaultOpacity}</span>
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

        {/* Actions */}
        <div className="flex gap-2 pt-2 border-t border-neutral-200 dark:border-neutral-700">
          <button
            type="button"
            onClick={handleSave}
            disabled={!canSubmit}
            className="cursor-pointer rounded-lg px-4 py-2 text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {isPending ? LABELS.modalSaving : LABELS.modalSave}
          </button>
          <button
            type="button"
            onClick={onClose}
            disabled={isPending}
            className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {LABELS.modalCancel}
          </button>
        </div>
      </div>
    </ModalShell>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function UserCustomLayersSection() {
  const LABELS = useCustomLayerLabels()
  const { addToast } = useToast()
  const qc = useQueryClient()
  const mutations = useUserMapLayersMutations()
  const { data: userLayers = [], isLoading } = useUserMapLayers()

  // Read per-user limit from public settings/limits endpoint (not admin-gated).
  const { limits } = useSettingsLimits()
  const maxCount = limits.user_map_layer_limit
  const usedCount = userLayers.length

  const [showModal, setShowModal] = useState(false)
  const [editingLayer, setEditingLayer] = useState<UserMapLayer | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<UserMapLayer | null>(null)

  const sorted = [...userLayers].sort((a, b) => a.sort_order - b.sort_order)
  const atLimit = usedCount >= maxCount

  function handleDelete(layer: UserMapLayer) {
    mutations.remove.mutate(layer.id, {
      onSuccess: () => {
        addToast({ type: 'success', title: LABELS.toastDeleted })
        qc.invalidateQueries({ queryKey: ['user-map-layers'] })
        setDeleteTarget(null)
      },
      onError: (err: unknown) => {
        const msg = err instanceof Error ? err.message : String(err)
        addToast({ type: 'error', title: LABELS.toastError, message: msg, duration: 0 })
        setDeleteTarget(null)
      },
    })
  }

  function handleToggleEnabled(layer: UserMapLayer, enabled: boolean) {
    mutations.update.mutate(
      { id: layer.id, body: { enabled } },
      {
        onError: (err: unknown) => {
          const msg = err instanceof Error ? err.message : String(err)
          addToast({ type: 'error', title: LABELS.toastError, message: msg, duration: 0 })
        },
      },
    )
  }

  function moveLayer(idx: number, direction: -1 | 1) {
    const next = [...sorted]
    const swapIdx = idx + direction
    if (swapIdx < 0 || swapIdx >= next.length) return
    const aOrder = next[idx].sort_order
    const bOrder = next[swapIdx].sort_order
    mutations.sort.mutate([
      { id: next[idx].id, sort_order: bOrder },
      { id: next[swapIdx].id, sort_order: aOrder },
    ])
  }

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <p className="text-sm text-neutral-400">{LABELS.loading}</p>
      </div>
    )
  }

  return (
    <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
      {/* Header + limit display */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{LABELS.title}</h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">{LABELS.description}</p>
          <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-1">
            {LABELS.limitDisplay(usedCount, maxCount)}
          </p>
        </div>
        <button
          type="button"
          disabled={atLimit}
          onClick={() => { setEditingLayer(null); setShowModal(true) }}
          title={atLimit ? LABELS.limitReachedHint : undefined}
          className="shrink-0 cursor-pointer rounded-lg px-3 py-1.5 text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          aria-disabled={atLimit}
        >
          {LABELS.addButton}
        </button>
      </div>

      {atLimit && (
        <p className="text-xs text-amber-600 dark:text-amber-400">{LABELS.limitReachedHint}</p>
      )}

      {/* Layer list */}
      {sorted.length > 0 && (
        <ul className="space-y-2">
          {sorted.map((layer, idx) => (
            <li
              key={layer.id}
              className="flex items-center gap-2 rounded-lg border border-neutral-200 dark:border-neutral-700 px-3 py-2"
            >
              {/* Sort buttons */}
              <div className="flex flex-col gap-0.5 shrink-0">
                <button
                  type="button"
                  disabled={idx === 0 || mutations.sort.isPending}
                  onClick={() => moveLayer(idx, -1)}
                  aria-label={LABELS.upButton}
                  className="p-0.5 text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 disabled:opacity-30 disabled:cursor-not-allowed cursor-pointer"
                >
                  ▲
                </button>
                <button
                  type="button"
                  disabled={idx === sorted.length - 1 || mutations.sort.isPending}
                  onClick={() => moveLayer(idx, 1)}
                  aria-label={LABELS.downButton}
                  className="p-0.5 text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 disabled:opacity-30 disabled:cursor-not-allowed cursor-pointer"
                >
                  ▼
                </button>
              </div>

              {/* Active toggle */}
              <div className="flex items-center shrink-0">
                <input
                  type="checkbox"
                  checked={layer.enabled}
                  onChange={e => handleToggleEnabled(layer, e.target.checked)}
                  disabled={mutations.update.isPending}
                  className="h-4 w-4 rounded accent-accent-DEFAULT cursor-pointer disabled:cursor-not-allowed"
                  aria-label={LABELS.toggleActiveLabel(layer.name)}
                />
              </div>

              {/* Info */}
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-1.5 min-w-0">
                  <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                    {layer.name}
                  </p>
                  <span className="shrink-0 text-[10px] uppercase font-medium px-1.5 py-0.5 rounded bg-neutral-200 dark:bg-neutral-700 text-neutral-600 dark:text-neutral-300">
                    {layer.role === 'base' ? LABELS.badgeBase : LABELS.badgeOverlay}
                  </span>
                  <span className="shrink-0 text-[10px] uppercase font-medium px-1.5 py-0.5 rounded bg-neutral-200 dark:bg-neutral-700 text-neutral-600 dark:text-neutral-300">
                    {layer.source_kind === 'raster_tiles' ? LABELS.badgeRaster : LABELS.badgeVector}
                  </span>
                </div>
                <p className="text-xs text-neutral-400 truncate">
                  {layer.source_kind === 'raster_tiles' ? layer.url_template : layer.style_url}
                </p>
              </div>

              {/* Actions */}
              <div className="flex items-center gap-1 shrink-0">
                <button
                  type="button"
                  onClick={() => { setEditingLayer(layer); setShowModal(true) }}
                  className="cursor-pointer rounded px-2 py-1 text-xs font-medium text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
                >
                  {LABELS.editButton}
                </button>
                <button
                  type="button"
                  onClick={() => setDeleteTarget(layer)}
                  className="cursor-pointer rounded px-2 py-1 text-xs font-medium text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors"
                >
                  {LABELS.deleteButton}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {/* Add / Edit modal */}
      {showModal && (
        <LayerModal
          layer={editingLayer}
          usedCount={usedCount}
          maxCount={maxCount}
          onClose={() => setShowModal(false)}
        />
      )}

      {/* Delete confirm */}
      {deleteTarget && (
        <ConfirmModal
          title={LABELS.deleteConfirmTitle}
          message={LABELS.deleteConfirmMsg(deleteTarget.name)}
          confirmLabel={LABELS.deleteConfirmLabel}
          isPending={mutations.remove.isPending}
          onConfirm={() => handleDelete(deleteTarget)}
          onClose={() => setDeleteTarget(null)}
        />
      )}
    </section>
  )
}

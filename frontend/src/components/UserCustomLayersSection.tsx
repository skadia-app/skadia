// User-managed custom tile-URL layers section in Settings → Karte tab.
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/lib/toast'
import { useUserMapLayers, useUserMapLayersMutations } from '@/hooks/useUserMapLayers'
import { useSettingsLimits } from '@/hooks/useSettingsLimits'
import { type UserMapLayer } from '@/api/mapLayers'
import { ModalShell } from '@/components/ui/ModalShell'
import ConfirmModal from '@/components/ConfirmModal'

// ── i18n-ready labels ─────────────────────────────────────────────────────────

const LABELS = {
  title: 'Eigene Karten-Layer',
  description: 'Eigene Tile-Server als zusätzliche Layer hinzufügen.',
  limitDisplay: (used: number, max: number) => `${used} / ${max} verwendet`,
  addButton: 'Layer hinzufügen',
  editButton: 'Bearbeiten',
  deleteButton: 'Löschen',
  upButton: 'Nach oben',
  downButton: 'Nach unten',
  modalAddTitle: 'Layer hinzufügen',
  modalEditTitle: 'Layer bearbeiten',
  modalSave: 'Speichern',
  modalSaving: 'Speichere…',
  modalCancel: 'Abbrechen',
  fieldName: 'Name',
  fieldUrl: 'Tile-URL-Vorlage',
  fieldUrlPlaceholder: 'https://tile.example.com/{z}/{x}/{y}.png',
  fieldAttribution: 'Attribution',
  fieldMinZoom: 'Min. Zoom',
  fieldMaxZoom: 'Max. Zoom',
  fieldDefaultOpacity: 'Standard-Deckkraft (%)',
  privacyTitle: 'Hinweise zum Datenschutz',
  privacyLine1:
    'Der Tile-Anbieter sieht deine IP-Adresse und besuchte Kartenregionen.',
  privacyLine2:
    'Hinweis: Der Server prüft URL-IPs, aber DNS-Auflösung passiert im Browser — ein bösartig konfigurierter Hostname kann theoretisch auf interne Adressen zeigen. Trage nur Server ein, denen du vertraust.',
  deleteConfirmTitle: 'Layer löschen?',
  deleteConfirmMsg: (name: string) => `Soll "${name}" dauerhaft gelöscht werden?`,
  deleteConfirmLabel: 'Löschen',
  toastCreated: 'Layer hinzugefügt',
  toastUpdated: 'Layer gespeichert',
  toastDeleted: 'Layer gelöscht',
  toastError: 'Fehler',
  toast422PrivateIP: 'Private Adressen sind nicht erlaubt',
  toast409Duplicate: 'Name bereits vergeben',
  toast422LimitReached: (used: number, max: number) => `Limit erreicht (${used} von ${max})`,
  toast429RateLimit: (retryAfter: string) => `Zu viele Anfragen, Retry-After: ${retryAfter}s`,
  limitReachedHint: 'Limit erreicht — vorhandene Layer löschen, um Platz zu schaffen.',
  loading: 'Lade Layer…',
}

// ── Form state ────────────────────────────────────────────────────────────────

interface LayerFormState {
  name: string
  url_template: string
  attribution: string
  min_zoom: string
  max_zoom: string
  default_opacity_pct: string
}

function emptyForm(): LayerFormState {
  return {
    name: '',
    url_template: '',
    attribution: '',
    min_zoom: '',
    max_zoom: '',
    default_opacity_pct: '100',
  }
}

function layerToForm(l: UserMapLayer): LayerFormState {
  return {
    name: l.name,
    url_template: l.url_template,
    attribution: l.attribution ?? '',
    min_zoom: l.min_zoom != null ? String(l.min_zoom) : '',
    max_zoom: l.max_zoom != null ? String(l.max_zoom) : '',
    default_opacity_pct: String(l.default_opacity ?? 100),
  }
}

function formToBody(f: LayerFormState): Partial<UserMapLayer> {
  return {
    name: f.name,
    url_template: f.url_template,
    attribution: f.attribution || undefined,
    min_zoom: f.min_zoom !== '' ? Number(f.min_zoom) : undefined,
    max_zoom: f.max_zoom !== '' ? Number(f.max_zoom) : undefined,
    default_opacity: Number(f.default_opacity_pct),
  }
}

// ── Error classification ──────────────────────────────────────────────────────

interface ApiErrorBody {
  error?: string
  limit?: number
  message?: string
}

function classifyError(err: unknown): {
  kind: 'private_ip' | 'duplicate' | 'limit_reached' | 'rate_limit' | 'generic'
  retryAfter?: string
  limit?: number
} {
  const msg = err instanceof Error ? err.message : String(err)
  if (msg.includes('422') && msg.includes('private_address_blocked')) {
    return { kind: 'private_ip' }
  }
  if (msg.includes('409') || msg.includes('duplicate_name') || msg.includes('Name bereits')) {
    return { kind: 'duplicate' }
  }
  if (msg.includes('422') && msg.includes('limit_reached')) {
    // Extract limit from JSON if embedded
    try {
      const json = JSON.parse(msg.replace(/^[^{]*/, '')) as ApiErrorBody
      return { kind: 'limit_reached', limit: json.limit }
    } catch {
      return { kind: 'limit_reached' }
    }
  }
  if (msg.includes('429') || msg.includes('rate limit')) {
    // Try to extract Retry-After
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
      default: {
        const msg = err instanceof Error ? err.message : String(err)
        addToast({ type: 'error', title: LABELS.toastError, message: msg, duration: 0 })
      }
    }
  }

  const isPending = mutations.create.isPending || mutations.update.isPending

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
      mutations.create.mutate(body, {
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
          <p className="text-xs text-amber-700 dark:text-amber-300 leading-relaxed">{LABELS.privacyLine2}</p>
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

        {/* Tile URL */}
        <div className="space-y-1">
          <label htmlFor="user-layer-modal-url" className="block text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {LABELS.fieldUrl}
          </label>
          <input
            id="user-layer-modal-url"
            type="url"
            value={form.url_template}
            onChange={e => patch({ url_template: e.target.value })}
            placeholder={LABELS.fieldUrlPlaceholder}
            className="w-full rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
          />
        </div>

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
            disabled={isPending || !form.name.trim() || !form.url_template.trim()}
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

              {/* Info */}
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                  {layer.name}
                </p>
                <p className="text-xs text-neutral-400 truncate">{layer.url_template}</p>
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

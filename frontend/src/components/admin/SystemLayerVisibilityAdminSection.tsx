// Admin section for configuring default visibility of the 14 fixed system overlay keys.
import { useState, useRef, useCallback } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/lib/toast'
import { useMapLayers } from '@/hooks/useMapLayers'
import { useSystemLayerKeys } from '@/hooks/useSystemLayerKeys'
import { patchSystemLayerVisibility, type SystemLayerVisibility } from '@/api/mapLayers'

// ── i18n-ready labels ─────────────────────────────────────────────────────────

const LABELS = {
  title: 'System-Overlay-Sichtbarkeit',
  description: 'Aktivierung und Standardwerte der 14 festen System-Layer für alle Benutzer.',
  groupVector: 'Vektorkacheln',
  groupCoverage: 'Abdeckungs-Layer',
  colEnabled: 'Aktiv',
  colDefaultNew: 'Neuen Usern aktiv',
  colOpacity: 'Standard-Deckkraft (%)',
  colLabel: 'Label-Überschreibung',
  placeholderLabel: '(Standard-Label)',
  toastSaved: 'Sichtbarkeit gespeichert',
  toastError: 'Fehler beim Speichern',
  loading: 'Lade Einstellungen…',
  labelMaxLength: 100,
}

// ── Debounce helper ───────────────────────────────────────────────────────────

function useDebounced<T extends unknown[]>(fn: (...args: T) => void, ms: number) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  return useCallback(
    (...args: T) => {
      if (timerRef.current) clearTimeout(timerRef.current)
      timerRef.current = setTimeout(() => fn(...args), ms)
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [fn, ms],
  )
}

// ── Single row ────────────────────────────────────────────────────────────────

interface RowProps {
  layerKey: string
  defaultLabel: string
  isCoverage: boolean
  visibility: SystemLayerVisibility | undefined
  onPatch: (key: string, body: Partial<SystemLayerVisibility>) => void
}

function VisibilityRow({ layerKey, defaultLabel, isCoverage, visibility, onPatch }: RowProps) {
  // Local state mirrors DB value; parent patches debounced
  const [enabled, setEnabled] = useState(visibility?.enabled ?? true)
  const [defaultEnabled, setDefaultEnabled] = useState(
    visibility?.default_enabled_for_new_users ?? false,
  )
  const [opacity, setOpacity] = useState<number>(
    visibility?.default_opacity ?? 100,
  )
  const [label, setLabel] = useState(visibility?.label_override ?? '')

  function handleEnabled(val: boolean) {
    setEnabled(val)
    onPatch(layerKey, { enabled: val })
  }

  function handleDefaultEnabled(val: boolean) {
    setDefaultEnabled(val)
    onPatch(layerKey, { default_enabled_for_new_users: val })
  }

  function handleOpacity(val: number) {
    setOpacity(val)
    onPatch(layerKey, { default_opacity: val })
  }

  function handleLabel(val: string) {
    const trimmed = val.slice(0, LABELS.labelMaxLength)
    setLabel(trimmed)
    onPatch(layerKey, { label_override: trimmed || undefined })
  }

  return (
    <li className="grid grid-cols-[1fr_auto_auto_auto] gap-3 items-center px-3 py-2.5 rounded-lg border border-neutral-200 dark:border-neutral-700">
      {/* Key / label */}
      <div className="min-w-0">
        <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
          {defaultLabel}
        </p>
        <p className="text-xs text-neutral-400 font-mono">{layerKey}</p>
      </div>

      {/* Enabled toggle */}
      <label className="flex flex-col items-center gap-1 cursor-pointer" title={LABELS.colEnabled}>
        <span className="text-xs text-neutral-500">{LABELS.colEnabled}</span>
        <input
          type="checkbox"
          checked={enabled}
          onChange={e => handleEnabled(e.target.checked)}
          className="h-4 w-4 rounded accent-accent-DEFAULT"
          aria-label={`${layerKey} ${LABELS.colEnabled}`}
        />
      </label>

      {/* Default for new users toggle */}
      <label className="flex flex-col items-center gap-1 cursor-pointer" title={LABELS.colDefaultNew}>
        <span className="text-xs text-neutral-500 text-center w-16">{LABELS.colDefaultNew}</span>
        <input
          type="checkbox"
          checked={defaultEnabled}
          onChange={e => handleDefaultEnabled(e.target.checked)}
          className="h-4 w-4 rounded accent-accent-DEFAULT"
          aria-label={`${layerKey} ${LABELS.colDefaultNew}`}
        />
      </label>

      {/* Coverage-only: opacity slider + label override */}
      {isCoverage && (
        <div className="flex flex-col items-start gap-1 w-40">
          <label className="text-xs text-neutral-500" htmlFor={`opacity-${layerKey}`}>
            {LABELS.colOpacity}: {opacity}%
          </label>
          <input
            id={`opacity-${layerKey}`}
            type="range"
            min={0}
            max={100}
            value={opacity}
            onChange={e => handleOpacity(Number(e.target.value))}
            className="w-full accent-accent-DEFAULT"
            aria-label={`${layerKey} ${LABELS.colOpacity}`}
          />
          <input
            type="text"
            value={label}
            maxLength={LABELS.labelMaxLength}
            placeholder={LABELS.placeholderLabel}
            onChange={e => handleLabel(e.target.value)}
            className="w-full rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
            aria-label={`${layerKey} ${LABELS.colLabel}`}
          />
        </div>
      )}

      {/* Non-coverage: label override only (no opacity) */}
      {!isCoverage && (
        <div className="flex flex-col items-start gap-1 w-40">
          <label className="text-xs text-neutral-500" htmlFor={`label-${layerKey}`}>
            {LABELS.colLabel}
          </label>
          <input
            id={`label-${layerKey}`}
            type="text"
            value={label}
            maxLength={LABELS.labelMaxLength}
            placeholder={LABELS.placeholderLabel}
            onChange={e => handleLabel(e.target.value)}
            className="w-full rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
            aria-label={`${layerKey} ${LABELS.colLabel}`}
          />
        </div>
      )}
    </li>
  )
}

// ── Collapsible group ─────────────────────────────────────────────────────────

interface GroupProps {
  title: string
  defaultOpen?: boolean
  children: React.ReactNode
}

function LayerGroup({ title, defaultOpen = true, children }: GroupProps) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="space-y-2">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className="flex items-center gap-2 text-sm font-semibold text-neutral-700 dark:text-neutral-300 cursor-pointer"
      >
        <span className="text-xs">{open ? '▼' : '▶'}</span>
        {title}
      </button>
      {open && <ul className="space-y-1.5">{children}</ul>}
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function SystemLayerVisibilityAdminSection() {
  const { addToast } = useToast()
  const qc = useQueryClient()
  const { systemVisibility, isLoading } = useMapLayers()
  const { keys, isLoading: keysLoading } = useSystemLayerKeys()

  const patchMut = useMutation({
    mutationFn: ({ key, body }: { key: string; body: Partial<SystemLayerVisibility> }) =>
      patchSystemLayerVisibility(key, body),
    onSuccess: () => {
      addToast({ type: 'success', title: LABELS.toastSaved })
      qc.invalidateQueries({ queryKey: ['map-layers'] })
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : String(err)
      addToast({ type: 'error', title: LABELS.toastError, message: msg, duration: 0 })
    },
  })

  // Debounce individual key patches by 400ms (sliders fire frequently)
  const debouncedPatch = useDebounced(
    (key: string, body: Partial<SystemLayerVisibility>) => patchMut.mutate({ key, body }),
    400,
  )

  if (isLoading || keysLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <p className="text-sm text-neutral-400">{LABELS.loading}</p>
      </div>
    )
  }

  const vectorKeys = keys.filter(k => k.category === 'vector')
  const coverageKeys = keys.filter(k => k.category === 'coverage')

  return (
    <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-5">
      <div>
        <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{LABELS.title}</h3>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">{LABELS.description}</p>
      </div>

      <LayerGroup title={LABELS.groupVector}>
        {vectorKeys.map(descriptor => (
          <VisibilityRow
            key={descriptor.key}
            layerKey={descriptor.key}
            defaultLabel={descriptor.default_label}
            isCoverage={false}
            visibility={systemVisibility[descriptor.key]}
            onPatch={debouncedPatch}
          />
        ))}
      </LayerGroup>

      <LayerGroup title={LABELS.groupCoverage}>
        {coverageKeys.map(descriptor => (
          <VisibilityRow
            key={descriptor.key}
            layerKey={descriptor.key}
            defaultLabel={descriptor.default_label}
            isCoverage={true}
            visibility={systemVisibility[descriptor.key]}
            onPatch={debouncedPatch}
          />
        ))}
      </LayerGroup>
    </section>
  )
}

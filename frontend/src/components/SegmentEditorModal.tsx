// frontend/src/components/SegmentEditorModal.tsx
// Task 8.4a: Modal Shell + Portal + Escape/Dirty-Close + Focus-Trap + beforeunload
import { createPortal } from 'react-dom'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { SegmentMetadata } from './segment-editor/SegmentSidebar'
import type { TrackPoint } from './segment-editor/TrackModeBottomBar'
import { BIDIRECTIONAL_DEFAULT, deriveTrailDifficultyColor, type SegType } from './segment-editor/constants'

export interface SegmentEditorModalProps {
  mode: 'track' | 'draw' | 'lap'
  trackPoints?: TrackPoint[]
  sourceActivityId?: string
  defaultSportGroup?: string
  onSuccess: () => void
  onClose: () => void
}

export default function SegmentEditorModal(props: SegmentEditorModalProps) {
  const { mode, trackPoints = [], onClose } = props

  const [meta, setMeta] = useState<SegmentMetadata>(() => ({
    name: '',
    segType: (mode === 'lap' ? 'lap' : 'standard') as SegType,
    sportTypes: null,
    sacScale: null,
    mtbScale: null,
    surface: null,
    difficultyColor: null,
    bidirectional: BIDIRECTIONAL_DEFAULT[(mode === 'lap' ? 'lap' : 'standard') as SegType],
    userEditedBidirectional: false,
    isPublic: true,
  }))

  const patchMeta = useCallback((patch: Partial<SegmentMetadata>) => {
    setMeta(prev => {
      const next = { ...prev, ...patch }
      if (next.segType === 'trail') {
        next.difficultyColor = deriveTrailDifficultyColor(next.sacScale, next.mtbScale)
      }
      return next
    })
  }, [])

  const isDirty = meta.name.length > 0

  const handleClose = useCallback(() => {
    if (isDirty && !window.confirm('Ungespeicherte Änderungen verwerfen?')) return
    onClose()
  }, [isDirty, onClose])

  // Escape-Handler
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape') handleClose() }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [handleClose])

  // beforeunload: schütze dirty state auch bei Tab-Close / Browser-Back-Button
  // Browser ignoriert den Custom-String und zeigt seinen eigenen Dialog;
  // e.preventDefault() reicht.
  useEffect(() => {
    if (!isDirty) return
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = '' // legacy-required by some browsers
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [isDirty])

  // Focus-Trap: clamp Tab-navigation to elements within the modal root.
  const modalRootRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const trap = (e: KeyboardEvent) => {
      if (e.key !== 'Tab') return
      const root = modalRootRef.current
      if (!root) return
      const focusable = root.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )
      if (focusable.length === 0) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      const active = document.activeElement as HTMLElement | null
      if (e.shiftKey && active === first) { e.preventDefault(); last.focus() }
      else if (!e.shiftKey && active === last) { e.preventDefault(); first.focus() }
    }
    window.addEventListener('keydown', trap)
    return () => window.removeEventListener('keydown', trap)
  }, [])

  // Silence unused-var warnings until later sub-tasks wire them.
  void patchMeta; void trackPoints

  const content = (
    <div ref={modalRootRef} className="fixed inset-0 z-[200] flex flex-col bg-neutral-950 text-white">
      {/* Top-Bar */}
      <div className="flex items-center gap-2 border-b border-neutral-800 bg-neutral-900 px-4 py-2 text-sm">
        <h2 className="font-semibold">
          {mode === 'track' ? 'Segment aus Track' : mode === 'draw' ? 'Segment zeichnen' : 'Rundkurs definieren'}
        </h2>
        <div className="flex-1" />
        {/* Map-Style + Heatmap toggles → filled in Task 8.4b */}
        <button
          type="button"
          onClick={handleClose}
          aria-label="Schließen"
          className="rounded bg-neutral-800 px-2 py-1 text-neutral-300 hover:bg-neutral-700 hover:text-white"
        >
          ✕
        </button>
      </div>

      {/* Middle (sidebar+map) — filled in Task 8.4b */}
      <div className="flex min-h-0 flex-1">
        <div className="w-80 shrink-0 border-r border-neutral-800 bg-neutral-900 p-3">
          {/* SegmentSidebar placeholder — wired in 8.4e */}
          <p className="text-xs text-neutral-400">Sidebar (TODO 8.4e)</p>
        </div>
        <div className="relative flex-1 bg-neutral-800">
          <p className="p-4 text-xs text-neutral-400">Map (TODO 8.4b)</p>
        </div>
      </div>

      {/* Bottom — filled in Tasks 8.4c / 8.4d / 8.4e */}
      <div className="border-t border-neutral-800 bg-neutral-900 p-2 text-xs text-neutral-400">
        Bottom-Bar (TODO 8.4c/d/e)
      </div>
    </div>
  )

  return createPortal(content, document.body)
}

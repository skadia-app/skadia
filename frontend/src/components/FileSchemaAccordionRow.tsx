// frontend/src/components/FileSchemaAccordionRow.tsx
//
// Shared accordion row for file-schema configuration, used by both the user
// settings tab (FileSchemaSettings) and the admin settings tab
// (AdminLibrarySettingsTab).  Each row shows a collapsible panel with:
//   - Schema-template input + Save button (accent-orange)
//   - Variable chips (typ-spezifisch) with insert-on-click
//   - Live preview of the template with example values
//   - Optional extra actions slot (used by admin tab for Bulk-Rename)

import { useRef, useMemo, useEffect, type ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'
import type { FileType } from '@/api/shadowfs'
import { TYPE_VARIABLES, EXAMPLE_VALUES, LIB_LABELS } from '@/lib/libraryLabels'

export interface FileSchemaAccordionRowProps {
  /** The file type this row represents. */
  fileType: FileType
  /** Human-readable label shown in the collapsed header. */
  label: string
  /** Current value of the template input (controlled by parent). */
  currentValue: string
  /** Called whenever the user edits the input. */
  onChange: (value: string) => void
  /** Called when the user clicks Save. */
  onSave: () => void
  /** Whether the save mutation is in flight. */
  isSaving: boolean
  /** Whether the value has changed from the saved state. */
  isDirty: boolean
  /** Whether editing is disabled (e.g. admin-managed mode). */
  disabled?: boolean
  /** Hint text shown below the input when disabled. */
  disabledHint?: string
  /** Hint shown below the input when there is an admin-provided default. */
  adminDefaultHint?: string
  /** Whether the accordion panel is currently open. */
  expanded: boolean
  /** Toggle open/close. */
  onToggle: () => void
  /** Extra actions rendered below the input row (e.g. Bulk-Rename button). */
  extraActions?: ReactNode
  /** Inline save-error message (optional). */
  saveError?: string | null
  /** Warning messages returned by the API after save. */
  warnings?: string[]
}

export function FileSchemaAccordionRow({
  fileType,
  label,
  currentValue,
  onChange,
  onSave,
  isSaving,
  isDirty,
  disabled = false,
  disabledHint,
  adminDefaultHint,
  expanded,
  onToggle,
  extraActions,
  saveError,
  warnings,
}: FileSchemaAccordionRowProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const pendingCursorRef = useRef<number | null>(null)

  const preview = useMemo(() => {
    let result = currentValue
    for (const [key, val] of Object.entries(EXAMPLE_VALUES)) {
      result = result.replaceAll(key, val)
    }
    return result || '(leer)'
  }, [currentValue])

  // Restore cursor position after inserting a variable chip.
  useEffect(() => {
    if (pendingCursorRef.current !== null && inputRef.current) {
      const pos = pendingCursorRef.current
      pendingCursorRef.current = null
      inputRef.current.setSelectionRange(pos, pos)
      inputRef.current.focus()
    }
  }, [currentValue])

  function insertVariable(variable: string) {
    const input = inputRef.current
    if (!input) return
    const start = input.selectionStart ?? currentValue.length
    const end = input.selectionEnd ?? currentValue.length
    const next = currentValue.slice(0, start) + variable + currentValue.slice(end)
    pendingCursorRef.current = start + variable.length
    onChange(next)
  }

  const vars = TYPE_VARIABLES[fileType]

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg overflow-hidden">
      {/* ── Header / toggle ──────────────────────────────────────────────── */}
      <button
        onClick={onToggle}
        className="w-full flex items-center justify-between px-4 py-3 text-left cursor-pointer
                   bg-neutral-50 dark:bg-neutral-800/50
                   hover:bg-neutral-100 dark:hover:bg-neutral-800
                   transition-colors"
        aria-expanded={expanded}
        aria-controls={`schema-body-${fileType}`}
      >
        <span className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
          {label}
        </span>
        <div className="flex items-center gap-3">
          {!expanded && currentValue && (
            <span className="text-xs font-mono text-neutral-400 dark:text-neutral-500 truncate max-w-60">
              {currentValue}
            </span>
          )}
          <ChevronDown
            className={`w-4 h-4 text-neutral-400 transition-transform ${expanded ? 'rotate-180' : ''}`}
            aria-hidden="true"
          />
        </div>
      </button>

      {/* ── Body ─────────────────────────────────────────────────────────── */}
      {expanded && (
        <div
          id={`schema-body-${fileType}`}
          className="px-4 py-4 space-y-3 bg-white dark:bg-neutral-900/30"
        >
          {/* Input + Save */}
          <div>
            <label
              htmlFor={`schema-input-${fileType}`}
              className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1"
            >
              {LIB_LABELS.fileSchemaTemplateLabel}
            </label>
            <div className="flex gap-2">
              <input
                ref={inputRef}
                id={`schema-input-${fileType}`}
                type="text"
                value={currentValue}
                onChange={e => !disabled && onChange(e.target.value)}
                disabled={disabled}
                readOnly={disabled}
                className="flex-1 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2
                           bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100
                           placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                           focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT text-sm font-mono
                           disabled:opacity-60 disabled:cursor-not-allowed disabled:bg-neutral-50
                           dark:disabled:bg-neutral-800/50"
              />
              <button
                onClick={onSave}
                disabled={!isDirty || isSaving || disabled}
                className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium text-white
                           bg-accent-DEFAULT hover:bg-accent-600
                           disabled:opacity-40 disabled:cursor-not-allowed
                           transition-colors"
              >
                {isSaving ? LIB_LABELS.savingButton : LIB_LABELS.saveButton}
              </button>
            </div>

            {disabled && disabledHint && (
              <p className="mt-1.5 text-xs text-neutral-500 dark:text-neutral-400">
                {disabledHint}
              </p>
            )}

            {!disabled && adminDefaultHint && (
              <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                {LIB_LABELS.schemaAdminDefaultHint}
                <code className="rounded bg-neutral-100 px-1 py-0.5 font-mono dark:bg-neutral-800">
                  {adminDefaultHint}
                </code>
              </p>
            )}
          </div>

          {/* Extra actions slot (e.g. Bulk-Rename in admin tab) */}
          {extraActions && (
            <div>
              {extraActions}
            </div>
          )}

          {/* Variable chips */}
          <div className="space-y-1.5">
            <span className="text-xs text-neutral-500 dark:text-neutral-500">
              {LIB_LABELS.fileSchemaAvailableVars}
            </span>
            <div className="flex flex-wrap gap-1.5">
              {vars.map(v => (
                <button
                  key={v.key}
                  onClick={() => insertVariable(v.key)}
                  disabled={disabled}
                  aria-label={`${LIB_LABELS.fileSchemaInsertVariable}: ${v.key}`}
                  className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded-md
                             border border-neutral-200 dark:border-neutral-700
                             bg-neutral-50 dark:bg-neutral-800
                             text-neutral-600 dark:text-neutral-400
                             hover:bg-orange-50 hover:border-orange-300 hover:text-accent-600
                             dark:hover:bg-orange-900/20 dark:hover:border-orange-700 dark:hover:text-orange-400
                             cursor-pointer transition-colors
                             disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-neutral-50
                             dark:disabled:hover:bg-neutral-800 disabled:hover:border-neutral-200
                             dark:disabled:hover:border-neutral-700 disabled:hover:text-neutral-600
                             dark:disabled:hover:text-neutral-400"
                >
                  <span className="font-mono">{v.key}</span>
                  <span className="text-neutral-400 dark:text-neutral-600">{v.label}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Live preview */}
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 p-3">
            <span className="text-xs text-neutral-500 dark:text-neutral-500 block mb-1">
              {LIB_LABELS.fileSchemaPreview}
            </span>
            <span className="text-sm font-mono text-neutral-800 dark:text-neutral-200 break-all">
              {preview}
            </span>
          </div>

          {/* API warnings */}
          {warnings && warnings.length > 0 && (
            <div className="rounded-lg border border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20 p-3 space-y-1">
              {warnings.map(w => (
                <p key={w} className="text-xs text-amber-700 dark:text-amber-400">{w}</p>
              ))}
            </div>
          )}

          {/* Inline save error */}
          {saveError && (
            <p className="text-xs text-rose-500">
              {LIB_LABELS.fileSchemaErrorSave}: {saveError}
            </p>
          )}
        </div>
      )}
    </div>
  )
}

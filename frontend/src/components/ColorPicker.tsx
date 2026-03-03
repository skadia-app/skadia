import { useRef, useState, useEffect } from 'react'
import { createPortal } from 'react-dom'

export interface ColorPickerProps {
  value: string
  onChange: (hex: string) => void
  className?: string
}

/** Normalize a hex string to uppercase #RRGGBB, or return null if invalid. */
function normalizeHex(raw: string): string | null {
  const trimmed = raw.trim()
  const full = /^#([0-9a-fA-F]{6})$/.test(trimmed)
    ? trimmed
    : /^([0-9a-fA-F]{6})$/.test(trimmed)
    ? `#${trimmed}`
    : /^#([0-9a-fA-F]{3})$/.test(trimmed)
    ? `#${trimmed[1]}${trimmed[1]}${trimmed[2]}${trimmed[2]}${trimmed[3]}${trimmed[3]}`
    : null
  return full ? full.toUpperCase() : null
}

/**
 * Compact color picker: a colored swatch button (opens the native color spectrum)
 * plus an editable hex text field (#RRGGBB) side by side.
 *
 * The native <input type="color"> is placed via a portal and positioned
 * viewport-safe next to the swatch button.
 */
export default function ColorPicker({ value, onChange, className }: ColorPickerProps) {
  const btnRef = useRef<HTMLButtonElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  // Display value in the text field — may be temporarily invalid while typing
  const [textValue, setTextValue] = useState<string>(() => value.toUpperCase())

  // Keep text field in sync when the external value changes (e.g. palette click)
  useEffect(() => {
    setTextValue(value.toUpperCase())
  }, [value])

  function openNativePicker() {
    if (btnRef.current && inputRef.current) {
      const rect = btnRef.current.getBoundingClientRect()
      const pickerW = 220
      const pickerH = 250
      let left = rect.left - pickerW - 4
      if (left < 8) left = rect.right + 4
      let top = rect.top
      if (top + pickerH > window.innerHeight - 8) top = window.innerHeight - pickerH - 8
      if (top < 8) top = 8
      inputRef.current.style.top = `${top}px`
      inputRef.current.style.left = `${left}px`
    }
    inputRef.current?.click()
  }

  function handleNativeChange(e: React.ChangeEvent<HTMLInputElement>) {
    const hex = e.target.value.toUpperCase()
    setTextValue(hex)
    onChange(hex)
  }

  function handleTextChange(e: React.ChangeEvent<HTMLInputElement>) {
    const raw = e.target.value
    setTextValue(raw)
    const normalized = normalizeHex(raw)
    if (normalized) {
      onChange(normalized)
    }
  }

  function handleTextBlur() {
    // On blur: if invalid, revert to last valid value
    const normalized = normalizeHex(textValue)
    if (normalized) {
      setTextValue(normalized)
    } else {
      setTextValue(value.toUpperCase())
    }
  }

  function handleTextKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      handleTextBlur()
      ;(e.target as HTMLInputElement).blur()
    }
  }

  return (
    <div className={`flex items-center gap-1.5${className ? ` ${className}` : ''}`}>
      {/* Swatch button — opens native color picker */}
      <button
        ref={btnRef}
        type="button"
        onClick={openNativePicker}
        className="w-7 h-6 rounded border border-neutral-300 dark:border-neutral-600 cursor-pointer flex-shrink-0"
        style={{ backgroundColor: value }}
        title={value}
      />

      {/* Hex text input */}
      <input
        type="text"
        value={textValue}
        onChange={handleTextChange}
        onBlur={handleTextBlur}
        onKeyDown={handleTextKeyDown}
        maxLength={7}
        spellCheck={false}
        className="w-[5.25rem] px-1.5 py-0.5 text-xs font-mono rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 outline-none focus:border-orange-400 dark:focus:border-orange-500 uppercase"
        placeholder="#RRGGBB"
      />

      {/* Hidden native color input — portalled to body for correct stacking */}
      {createPortal(
        <input
          ref={inputRef}
          type="color"
          value={value}
          onChange={handleNativeChange}
          style={{ position: 'fixed', top: 0, left: 0, width: 1, height: 1, opacity: 0, pointerEvents: 'none' }}
          tabIndex={-1}
        />,
        document.body,
      )}
    </div>
  )
}

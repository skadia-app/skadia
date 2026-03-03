import { useRef, useState, useEffect } from 'react'
import { createPortal } from 'react-dom'

export interface ColorPickerProps {
  value: string
  onChange: (hex: string) => void
  className?: string
}

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

function hsvToHex(h: number, s: number, v: number): string {
  const f = (n: number) => {
    const k = (n + h / 60) % 6
    return v - v * s * Math.max(Math.min(k, 4 - k, 1), 0)
  }
  const r = Math.round(f(5) * 255)
  const g = Math.round(f(3) * 255)
  const b = Math.round(f(1) * 255)
  return `#${r.toString(16).padStart(2, '0')}${g.toString(16).padStart(2, '0')}${b.toString(16).padStart(2, '0')}`.toUpperCase()
}

function hexToHsv(hex: string): [number, number, number] {
  const norm = normalizeHex(hex)
  if (!norm) return [0, 0, 0.5]
  const r = parseInt(norm.slice(1, 3), 16) / 255
  const g = parseInt(norm.slice(3, 5), 16) / 255
  const b = parseInt(norm.slice(5, 7), 16) / 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const d = max - min
  let h = 0
  if (d !== 0) {
    if (max === r) h = ((g - b) / d + 6) % 6
    else if (max === g) h = (b - r) / d + 2
    else h = (r - g) / d + 4
    h *= 60
  }
  return [h, max === 0 ? 0 : d / max, max]
}

const GW = 220 // gradient width
const GH = 150 // gradient height

/**
 * Fully custom color picker:
 * - Normal view: only a colored swatch button.
 * - Click: opens a popover with hue/saturation gradient, hue slider and hex input.
 */
export default function ColorPicker({ value, onChange, className }: ColorPickerProps) {
  const swatchRef = useRef<HTMLButtonElement>(null)
  const popoverRef = useRef<HTMLDivElement>(null)
  const gradientRef = useRef<HTMLDivElement>(null)
  const hueRef = useRef<HTMLDivElement>(null)

  const [open, setOpen] = useState(false)
  const [popoverPos, setPopoverPos] = useState({ top: 0, left: 0 })
  const [textValue, setTextValue] = useState<string>(() => (normalizeHex(value) ?? value).toUpperCase())
  const [hsv, setHsv] = useState<[number, number, number]>(() => hexToHsv(value))

  // Sync when external value changes
  useEffect(() => {
    const norm = normalizeHex(value)
    if (norm) {
      setTextValue(norm)
      setHsv(hexToHsv(norm))
    }
  }, [value])

  // Close on outside mousedown
  useEffect(() => {
    if (!open) return
    function handle(e: MouseEvent) {
      if (
        popoverRef.current && !popoverRef.current.contains(e.target as Node) &&
        swatchRef.current && !swatchRef.current.contains(e.target as Node)
      ) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [open])

  function openPicker() {
    if (!swatchRef.current) return
    const rect = swatchRef.current.getBoundingClientRect()
    const popW = GW + 16
    const popH = GH + 12 + 16 + 32 + 24 // gradient + hue + hex + padding
    let left = rect.left
    if (left + popW > window.innerWidth - 8) left = window.innerWidth - popW - 8
    let top = rect.bottom + 4
    if (top + popH > window.innerHeight - 8) top = rect.top - popH - 4
    setPopoverPos({ top, left })
    setOpen(o => !o)
  }

  function emit(h: number, s: number, v: number) {
    const hex = hsvToHex(h, s, v)
    setTextValue(hex)
    onChange(hex)
  }

  function handleGradientPointer(e: React.PointerEvent<HTMLDivElement>) {
    if (e.buttons === 0 && e.type !== 'pointerdown') return
    const rect = gradientRef.current!.getBoundingClientRect()
    const s = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    const v = Math.max(0, Math.min(1, 1 - (e.clientY - rect.top) / rect.height))
    const next: [number, number, number] = [hsv[0], s, v]
    setHsv(next)
    emit(...next)
  }

  function handleHuePointer(e: React.PointerEvent<HTMLDivElement>) {
    if (e.buttons === 0 && e.type !== 'pointerdown') return
    const rect = hueRef.current!.getBoundingClientRect()
    const h = Math.max(0, Math.min(360, ((e.clientX - rect.left) / rect.width) * 360))
    const next: [number, number, number] = [h, hsv[1], hsv[2]]
    setHsv(next)
    emit(...next)
  }

  function handleTextChange(e: React.ChangeEvent<HTMLInputElement>) {
    const raw = e.target.value
    setTextValue(raw)
    const norm = normalizeHex(raw)
    if (norm) {
      setHsv(hexToHsv(norm))
      onChange(norm)
    }
  }

  function handleTextBlur() {
    const norm = normalizeHex(textValue)
    if (norm) setTextValue(norm)
    else setTextValue((normalizeHex(value) ?? value).toUpperCase())
  }

  const [h, s, v] = hsv
  const hueColor = hsvToHex(h, 1, 1)

  return (
    <div className={`inline-flex${className ? ` ${className}` : ''}`}>
      {/* Swatch — only visible element in normal view */}
      <button
        ref={swatchRef}
        type="button"
        onClick={openPicker}
        className="w-7 h-6 rounded border border-neutral-300 dark:border-neutral-600 cursor-pointer flex-shrink-0"
        style={{ backgroundColor: value }}
        title={value}
      />

      {open && createPortal(
        <div
          ref={popoverRef}
          style={{ position: 'fixed', top: popoverPos.top, left: popoverPos.left, zIndex: 9999 }}
          className="rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-xl p-2 space-y-2"
        >
          {/* Saturation / Value gradient */}
          <div
            ref={gradientRef}
            style={{ width: GW, height: GH, background: hueColor, position: 'relative', borderRadius: 4, cursor: 'crosshair', userSelect: 'none' }}
            onPointerDown={e => { e.currentTarget.setPointerCapture(e.pointerId); handleGradientPointer(e) }}
            onPointerMove={handleGradientPointer}
          >
            <div style={{ position: 'absolute', inset: 0, background: 'linear-gradient(to right, #fff, transparent)', borderRadius: 4 }} />
            <div style={{ position: 'absolute', inset: 0, background: 'linear-gradient(to bottom, transparent, #000)', borderRadius: 4 }} />
            {/* Crosshair cursor */}
            <div style={{
              position: 'absolute',
              left: `${s * 100}%`,
              top: `${(1 - v) * 100}%`,
              width: 12, height: 12,
              borderRadius: '50%',
              border: '2px solid #fff',
              boxShadow: '0 0 0 1px rgba(0,0,0,0.4)',
              transform: 'translate(-50%,-50%)',
              pointerEvents: 'none',
            }} />
          </div>

          {/* Hue slider */}
          <div
            ref={hueRef}
            style={{
              width: GW, height: 12, borderRadius: 6,
              background: 'linear-gradient(to right,#f00,#ff0,#0f0,#0ff,#00f,#f0f,#f00)',
              position: 'relative', cursor: 'pointer', userSelect: 'none',
            }}
            onPointerDown={e => { e.currentTarget.setPointerCapture(e.pointerId); handleHuePointer(e) }}
            onPointerMove={handleHuePointer}
          >
            <div style={{
              position: 'absolute',
              left: `${(h / 360) * 100}%`,
              top: '50%',
              width: 14, height: 14,
              borderRadius: '50%',
              border: '2px solid #fff',
              boxShadow: '0 0 0 1px rgba(0,0,0,0.4)',
              background: hueColor,
              transform: 'translate(-50%,-50%)',
              pointerEvents: 'none',
            }} />
          </div>

          {/* Hex input */}
          <input
            type="text"
            value={textValue}
            onChange={handleTextChange}
            onBlur={handleTextBlur}
            onKeyDown={e => {
              if (e.key === 'Enter') { handleTextBlur(); setOpen(false) }
              if (e.key === 'Escape') setOpen(false)
            }}
            maxLength={7}
            spellCheck={false}
            className="w-full px-2 py-1 text-xs font-mono rounded border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 outline-none focus:border-orange-400 dark:focus:border-orange-500 uppercase"
            placeholder="#RRGGBB"
          />
        </div>,
        document.body,
      )}
    </div>
  )
}

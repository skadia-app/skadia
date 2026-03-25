const sizes = {
  sm: { track: 'h-5 w-9', thumb: 'h-3.5 w-3.5', on: 'translate-x-[18px]', off: 'translate-x-[3px]' },
  md: { track: 'h-6 w-11', thumb: 'h-4 w-4', on: 'translate-x-6', off: 'translate-x-1' },
} as const

interface ToggleProps {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  /** Override active color (default: var(--color-accent-DEFAULT)) */
  color?: string
  /** sm = compact (20×36px), md = default (24×44px) */
  size?: 'sm' | 'md'
  className?: string
  'aria-label'?: string
}

export default function Toggle({
  checked,
  onChange,
  disabled,
  color,
  size = 'md',
  className,
  'aria-label': ariaLabel,
}: ToggleProps) {
  const activeColor = color ?? 'var(--color-accent-DEFAULT)'
  const s = sizes[size]
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex ${s.track} items-center rounded-full transition-colors flex-shrink-0 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed${checked ? '' : ' bg-neutral-300 dark:bg-neutral-600'}${className ? ` ${className}` : ''}`}
      style={checked ? { backgroundColor: activeColor } : undefined}
    >
      <span
        className={`inline-block ${s.thumb} transform rounded-full bg-white transition-transform ${
          checked ? s.on : s.off
        }`}
      />
    </button>
  )
}

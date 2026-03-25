interface ToggleProps {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  /** Override active color (default: var(--color-accent-DEFAULT)) */
  color?: string
  className?: string
  'aria-label'?: string
}

export default function Toggle({
  checked,
  onChange,
  disabled,
  color,
  className,
  'aria-label': ariaLabel,
}: ToggleProps) {
  const activeColor = color ?? 'var(--color-accent-DEFAULT)'
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors flex-shrink-0 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed${checked ? '' : ' bg-neutral-300 dark:bg-neutral-600'}${className ? ` ${className}` : ''}`}
      style={checked ? { backgroundColor: activeColor } : undefined}
    >
      <span
        className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
          checked ? 'translate-x-6' : 'translate-x-1'
        }`}
      />
    </button>
  )
}

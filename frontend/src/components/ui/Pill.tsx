// frontend/src/components/ui/Pill.tsx
// Unified selectable pill used for SegType radio, sport category chips,
// and difficulty buttons — canonical styling so all three look cohesive.
import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface PillBaseProps {
  /** Whether the pill is in selected/active state */
  active?: boolean
  children: ReactNode
  /**
   * Optional left slot — rendered before the label text.
   * Used for difficulty colour dots, checkmarks, etc.
   */
  leftSlot?: ReactNode
  className?: string
  /**
   * Size variant.
   * - 'default' (default): standard pill — px-3 py-1.5 text-sm, rounded-lg
   * - 'sm': compact pill for Feinwahl grids — px-2 py-0.5 text-xs, rounded-full
   */
  size?: 'default' | 'sm'
  // A11y — pass role="radio" for mutually-exclusive groups (SegType, Difficulty)
  // or role="checkbox" for multi-select groups (Sport categories).
  role?: 'radio' | 'checkbox'
  // Matching aria attribute for the role
  ariaChecked?: boolean | 'mixed'
  ariaLabel?: string
  /**
   * Tab-order override. Useful for roving-tabindex patterns in radiogroups —
   * active pill gets tabIndex=0, inactive peers -1 so Tab moves between
   * groups, Arrow keys move within.
   */
  tabIndex?: number
}

interface PillButtonProps extends PillBaseProps {
  /**
   * Render as a <button> (default). Supports disabled and onClick.
   * Use this for standalone interactive pills.
   */
  as?: 'button'
  onClick: () => void
  /** Disabled state — rendered but not interactive */
  disabled?: boolean
}

interface PillSpanProps extends PillBaseProps {
  /**
   * Render as a <span> to avoid nested-interactive-content HTML violations
   * (e.g. when Pill is a child of <Link>/<a>). No onClick or disabled.
   */
  as: 'span'
  onClick?: never
  disabled?: never
}

export type PillProps = PillButtonProps | PillSpanProps

/**
 * Canonical pill component for all selector groups in SegmentSidebar.
 *
 * Active state: accent border + tinted bg + accent text (orange).
 * Inactive state: neutral border + neutral text + subtle hover.
 * All three selector families (SegType, Sport categories, Difficulty) use
 * the same visual grammar — Difficulty communicates meaning via the dot in
 * leftSlot, not by changing the accent colour to blue/red/black.
 *
 * Use `as="span"` when embedding inside an <a>/<Link> to avoid the HTML5
 * spec violation of <button> as descendant of <a>.
 */
export function Pill(props: PillProps) {
  const {
    as: Component = 'button',
    active = false,
    children,
    leftSlot,
    className,
    size = 'default',
    role,
    ariaChecked,
    ariaLabel,
    tabIndex,
  } = props

  const sharedClassName = cn(
    // Shape — size-dependent padding, font and radius
    'inline-flex items-center border font-medium',
    'transition-colors duration-150',
    // Focus ring
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1 focus-visible:ring-offset-white dark:focus-visible:ring-offset-neutral-900',
    // Size variants
    size === 'sm'
      ? 'gap-1 rounded-full px-2 py-0.5 text-xs'
      : 'gap-1.5 rounded-lg px-3 py-1.5 text-sm',
    // Active state — orange accent (same for ALL pill types)
    active
      ? 'border-accent-DEFAULT bg-accent-DEFAULT/10 text-accent-700 dark:text-accent-300'
      : 'border-neutral-300 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:border-neutral-400 dark:hover:border-neutral-600 hover:bg-neutral-50 dark:hover:bg-neutral-800/60',
    className,
  )

  const inner = (
    <>
      {leftSlot && <span className="shrink-0">{leftSlot}</span>}
      {children}
    </>
  )

  if (Component === 'span') {
    return (
      <span
        role={role}
        aria-checked={ariaChecked}
        aria-label={ariaLabel}
        tabIndex={tabIndex}
        className={sharedClassName}
      >
        {inner}
      </span>
    )
  }

  const { onClick, disabled } = props as PillButtonProps
  return (
    <button
      type="button"
      role={role}
      aria-checked={ariaChecked}
      aria-label={ariaLabel}
      tabIndex={tabIndex}
      disabled={disabled}
      onClick={onClick}
      className={cn(sharedClassName, disabled && 'cursor-not-allowed opacity-50')}
    >
      {inner}
    </button>
  )
}

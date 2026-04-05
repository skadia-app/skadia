import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface SettingsSectionProps {
  title: string
  description?: string
  children: ReactNode
  danger?: boolean
  className?: string
}

/**
 * Standard wrapper for settings sections.
 * Renders a bordered, rounded card with a title/description header and
 * space for content below.  Pass `danger` for the red (destructive)
 * border variant and `className` to override / extend `space-y-*`.
 */
export function SettingsSection({
  title,
  description,
  children,
  danger = false,
  className,
}: SettingsSectionProps) {
  return (
    <section
      className={cn(
        'rounded-xl p-6 space-y-4 border',
        danger
          ? 'border-red-200 dark:border-red-800'
          : 'border-neutral-200 dark:border-neutral-800',
        className,
      )}
    >
      <div>
        <h2
          className={cn(
            'text-lg font-medium',
            danger
              ? 'text-red-700 dark:text-red-400'
              : 'text-neutral-900 dark:text-neutral-100',
          )}
        >
          {title}
        </h2>
        {description && (
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            {description}
          </p>
        )}
      </div>
      {children}
    </section>
  )
}

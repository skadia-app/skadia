import { useCallback, useId, useState } from 'react'

export interface UseHoverPopoverOptions {
  onOpen?: () => void
  onClose?: () => void
  /** Whether pressing Escape while open closes the popover. Default: true. */
  closeOnEscape?: boolean
}

export interface UseHoverPopoverResult {
  open: boolean
  /** Props to spread onto the trigger element (button/div that activates the popover). */
  triggerProps: {
    'aria-expanded': boolean
    onMouseEnter: () => void
    onMouseLeave: (e: React.MouseEvent) => void
    onFocus: () => void
    onBlur: (e: React.FocusEvent) => void
    onKeyDown: (e: React.KeyboardEvent) => void
  }
  /** Props to spread onto the popover container element (provides the sentinel data attribute). */
  popoverProps: {
    [key: string]: string
  }
  /** The sentinel data-attribute name used for the relatedTarget guard. */
  sentinelAttr: string
}

/**
 * Shared hover-popover guard pattern.
 *
 * Uses a per-instance sentinel data attribute (generated via useId) so that
 * moving the mouse from the trigger to the popover content does not close the
 * popover. Both the trigger wrapper and the popover container must receive the
 * sentinel attribute (spread triggerProps on the wrapper and popoverProps on
 * the popover itself, or add {[sentinelAttr]: ''} manually).
 *
 * No memory leak on unmount: the hook carries no global listeners — all events
 * are bound as React synthetic event handlers that are cleaned up by React.
 */
export function useHoverPopover({
  onOpen,
  onClose,
  closeOnEscape = true,
}: UseHoverPopoverOptions = {}): UseHoverPopoverResult {
  const [open, setOpen] = useState(false)
  // Stable per-instance ID so multiple popover instances don't share sentinels.
  const id = useId()
  // Replace characters that are invalid in attribute names.
  const sentinelAttr = `data-hover-popover-${id.replace(/:/g, '')}`

  const openPopover = useCallback(() => {
    setOpen(true)
    onOpen?.()
  }, [onOpen])

  const closeWithGuard = useCallback(
    (relatedTarget: EventTarget | null) => {
      const rt = relatedTarget as HTMLElement | null
      if (rt?.closest?.(`[${sentinelAttr}]`)) return
      setOpen(false)
      onClose?.()
    },
    [sentinelAttr, onClose],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (closeOnEscape && e.key === 'Escape' && open) {
        setOpen(false)
        onClose?.()
      }
    },
    [closeOnEscape, open, onClose],
  )

  const triggerProps = {
    'aria-expanded': open,
    onMouseEnter: openPopover,
    onMouseLeave: (e: React.MouseEvent) => closeWithGuard(e.relatedTarget),
    onFocus: openPopover,
    onBlur: (e: React.FocusEvent) => closeWithGuard(e.relatedTarget),
    onKeyDown: handleKeyDown,
  }

  const popoverProps: { [key: string]: string } = {
    [sentinelAttr]: '',
  }

  return { open, triggerProps, popoverProps, sentinelAttr }
}

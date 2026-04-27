import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import { describe, expect, it, vi } from 'vitest'
import { ToastContainer } from '../Toast'
import { ToastProvider, useToast } from '@/lib/toast'

function AddActionToast({ onAction }: { onAction: () => void }) {
  const { addToast } = useToast()
  return (
    <button
      type="button"
      onClick={() => {
        addToast({
          type: 'info',
          title: 'Migration verfügbar',
          message: 'Diese Route nutzt ein altes Profil-Format.',
          duration: 0,
          action: { label: 'Re-Migrieren', onClick: onAction },
        })
      }}
    >
      Toast anzeigen
    </button>
  )
}

describe('Toast action', () => {
  it('renders an action toast and closes it after action click', () => {
    const onAction = vi.fn()
    render(
      <ToastProvider>
        <AddActionToast onAction={onAction} />
        <ToastContainer />
      </ToastProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Toast anzeigen' }))
    expect(screen.getByText('Migration verfügbar')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Re-Migrieren' }))

    expect(onAction).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('Migration verfügbar')).not.toBeInTheDocument()
  })
})

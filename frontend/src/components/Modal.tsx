import { useState } from 'react'
import type { ModalConfig } from '@/lib/modal'
import { useModal } from '@/lib/modal'
import { useAccentColor } from '@/lib/accentColor'

interface ModalItemProps {
  modal: ModalConfig
}

function ModalItem({ modal }: ModalItemProps) {
  const { closeModal } = useModal()
  const [isLoading, setIsLoading] = useState(false)
  const { accent } = useAccentColor()

  const handleButtonClick = async (button: ModalConfig['buttons'][0]) => {
    try {
      setIsLoading(true)
      await Promise.resolve(button.onClick())
      closeModal(modal.id)
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      onClick={() => closeModal(modal.id)}
      role="presentation"
    >
      {/* Blur Overlay */}
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" />

      {/* Modal Content */}
      <div
        className="relative bg-white dark:bg-neutral-950 rounded-lg shadow-lg p-6 max-w-md w-[90vw] max-h-[90vh] overflow-auto"
        onClick={e => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        {/* Title */}
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-50">
          {modal.title}
        </h2>

        {/* Message */}
        {modal.message && (
          <p className="mt-3 text-sm text-neutral-600 dark:text-neutral-400 leading-relaxed">
            {modal.message}
          </p>
        )}

        {/* Error Code */}
        {modal.errorCode && (
          <p className="mt-2 text-xs text-neutral-500 dark:text-neutral-400 font-mono">
            ({modal.errorCode})
          </p>
        )}

        {/* Buttons Container */}
        <div className="mt-6 flex flex-col gap-3 sm:flex-row sm:gap-2 sm:justify-end">
          {modal.buttons.map((button, idx) => (
            <button
              key={idx}
              onClick={() => handleButtonClick(button)}
              disabled={isLoading}
              className={`px-4 py-2 rounded-md font-medium text-sm transition-colors whitespace-nowrap ${
                button.variant === 'destructive'
                  ? 'bg-red-500 hover:bg-red-600 text-white disabled:opacity-50'
                  : 'text-white disabled:opacity-50 hover:opacity-90'
              }`}
              style={button.variant !== 'destructive' ? { backgroundColor: accent.shade500 } : {}}
            >
              {button.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}

export function ModalContainer() {
  const { modals } = useModal()

  return (
    <>
      {modals.map(modal => (
        <ModalItem key={modal.id} modal={modal} />
      ))}
    </>
  )
}

import { useEffect, useRef, useState } from 'react'
import type { ModalConfig, PromptModalConfig } from '@/lib/modal'
import { useModal } from '@/lib/modal'
import { useAccentColor } from '@/lib/accentColor'

interface ModalItemProps {
  modal: ModalConfig
}

function ModalItem({ modal }: ModalItemProps) {
  const dialogRef = useRef<HTMLDivElement>(null)
  const { closeModal } = useModal()
  const [isLoading, setIsLoading] = useState(false)
  const { accent } = useAccentColor()

  useEffect(() => {
    // Focus auf ersten Button (oder Modal selbst, falls kein Button)
    const firstButton = dialogRef.current?.querySelector('button')
    if (firstButton && firstButton instanceof HTMLButtonElement) {
      firstButton.focus()
    } else {
      dialogRef.current?.focus()
    }

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        closeModal(modal.id)
      }
    }

    document.addEventListener('keydown', handleEscape)
    return () => document.removeEventListener('keydown', handleEscape)
  }, [modal.id, closeModal])

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
        ref={dialogRef}
        tabIndex={0}
        className="relative bg-white dark:bg-neutral-950 rounded-lg shadow-lg p-6 max-w-md w-[90vw] max-h-[90vh] overflow-auto"
        onClick={e => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby={`modal-title-${modal.id}`}
        aria-describedby={modal.message ? `modal-message-${modal.id}` : undefined}
      >
        {/* Title */}
        <h2 id={`modal-title-${modal.id}`} className="text-lg font-semibold text-neutral-900 dark:text-neutral-50">
          {modal.title}
        </h2>

        {/* Message */}
        {modal.message && (
          <p id={`modal-message-${modal.id}`} className="mt-3 text-sm text-neutral-600 dark:text-neutral-400 leading-relaxed">
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
              key={`${modal.id}-btn-${idx}`}
              onClick={() => handleButtonClick(button)}
              disabled={isLoading}
              className={`px-4 py-2 rounded-md font-medium text-sm transition-colors whitespace-nowrap disabled:cursor-not-allowed ${
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

interface PromptModalItemProps {
  config: PromptModalConfig
}

function PromptModalItem({ config }: PromptModalItemProps) {
  const [inputValue, setInputValue] = useState(config.defaultValue || '')
  const [isLoading, setIsLoading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const dialogRef = useRef<HTMLDivElement>(null)
  const { closePromptModal } = useModal()
  const { accent } = useAccentColor()

  useEffect(() => {
    inputRef.current?.focus()

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        closePromptModal()
        config.onCancel?.()
      }
    }

    document.addEventListener('keydown', handleEscape)
    return () => document.removeEventListener('keydown', handleEscape)
  }, [closePromptModal, config])

  const handleConfirm = async () => {
    try {
      setIsLoading(true)
      await Promise.resolve(config.onConfirm(inputValue))
      closePromptModal()
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      onClick={() => {
        closePromptModal()
        config.onCancel?.()
      }}
      role="presentation"
    >
      {/* Blur Overlay */}
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" />

      {/* Modal Content */}
      <div
        ref={dialogRef}
        tabIndex={0}
        className="relative bg-white dark:bg-neutral-950 rounded-lg shadow-lg p-6 max-w-md w-[90vw] max-h-[90vh] overflow-auto"
        onClick={e => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        {/* Title */}
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-50">
          {config.title}
        </h2>

        {/* Message */}
        {config.message && (
          <p className="mt-3 text-sm text-neutral-600 dark:text-neutral-400 leading-relaxed">
            {config.message}
          </p>
        )}

        {/* Input Field */}
        <input
          ref={inputRef}
          type="text"
          placeholder={config.placeholder || ''}
          value={inputValue}
          onChange={e => setInputValue(e.target.value)}
          onKeyDown={e => {
            if (e.key === 'Enter') {
              handleConfirm()
            }
          }}
          className="w-full px-3 py-2 mt-4 border border-neutral-300 rounded-md dark:bg-neutral-900 dark:border-neutral-700 dark:text-neutral-50 focus:outline-none focus:ring-2 focus:ring-offset-0"
          disabled={isLoading}
        />

        {/* Buttons Container */}
        <div className="mt-6 flex flex-col gap-3 sm:flex-row sm:gap-2 sm:justify-end">
          <button
            onClick={() => {
              closePromptModal()
              config.onCancel?.()
            }}
            disabled={isLoading}
            className="cursor-pointer px-4 py-2 rounded-md font-medium text-sm transition-colors whitespace-nowrap disabled:cursor-not-allowed bg-neutral-200 hover:bg-neutral-300 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-900 dark:text-neutral-50 disabled:opacity-50"
          >
            Abbrechen
          </button>
          <button
            onClick={handleConfirm}
            disabled={isLoading}
            className="cursor-pointer px-4 py-2 rounded-md font-medium text-sm transition-colors whitespace-nowrap disabled:cursor-not-allowed text-white disabled:opacity-50 hover:opacity-90"
            style={{ backgroundColor: accent.shade500 }}
          >
            OK
          </button>
        </div>
      </div>
    </div>
  )
}

export function ModalContainer() {
  const { modals, prompts } = useModal()

  return (
    <>
      {modals.map(modal => (
        <ModalItem key={modal.id} modal={modal} />
      ))}
      {prompts.length > 0 && <PromptModalItem config={prompts[0]} />}
    </>
  )
}

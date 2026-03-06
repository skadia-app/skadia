import { createContext, useContext, useState, useCallback, ReactNode } from 'react'

export interface ModalButton {
  label: string
  variant: 'default' | 'destructive' // default = accent color, destructive = red
  onClick: () => void | Promise<void>
}

export interface ModalConfig {
  id: string
  title: string
  message?: string
  errorCode?: string
  buttons: ModalButton[]
  onClose?: () => void
}

interface ModalContextType {
  modals: ModalConfig[]
  openModal: (config: Omit<ModalConfig, 'id'>) => void
  closeModal: (id: string) => void
}

const ModalContext = createContext<ModalContextType | null>(null)

export function ModalProvider({ children }: { children: ReactNode }) {
  const [modals, setModals] = useState<ModalConfig[]>([])

  const openModal = useCallback((config: Omit<ModalConfig, 'id'>) => {
    const id = `modal-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`
    setModals(prev => [...prev, { ...config, id }])
  }, [])

  const closeModal = useCallback((id: string) => {
    setModals(prev => prev.filter(m => m.id !== id))
  }, [])

  return (
    <ModalContext.Provider value={{ modals, openModal, closeModal }}>
      {children}
    </ModalContext.Provider>
  )
}

export function useModal() {
  const context = useContext(ModalContext)
  if (!context) {
    throw new Error('useModal must be used within ModalProvider')
  }
  return context
}

import { useState, useRef, KeyboardEvent } from 'react'
import { SendHorizonal } from 'lucide-react'

interface ChatInputProps {
  onSend: (message: string) => void
  disabled?: boolean
  placeholder?: string
}

export default function ChatInput({ onSend, disabled, placeholder = 'Frag mich etwas…' }: ChatInputProps) {
  const [value, setValue] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const handleSend = () => {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const handleInput = () => {
    const el = textareaRef.current
    if (el) {
      el.style.height = 'auto'
      el.style.height = Math.min(el.scrollHeight, 120) + 'px'
    }
  }

  return (
    <div className="flex gap-2 items-end p-3 border-t border-neutral-200 dark:border-neutral-800">
      <div className="flex-1 relative">
        <textarea
          ref={textareaRef}
          value={value}
          onChange={e => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          onInput={handleInput}
          disabled={disabled}
          maxLength={2000}
          rows={1}
          placeholder={placeholder}
          className="w-full resize-none rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT disabled:opacity-50 transition-colors"
          style={{ minHeight: '38px', maxHeight: '120px' }}
        />
        <span className="absolute bottom-1 right-2 text-xs text-neutral-400">{value.length}/2000</span>
      </div>
      <button
        onClick={handleSend}
        disabled={disabled || !value.trim()}
        className="cursor-pointer p-2 rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors flex-shrink-0"
      >
        <SendHorizonal className="w-4 h-4" />
      </button>
    </div>
  )
}

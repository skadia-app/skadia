import { useState } from 'react'
import { ChevronDown, ChevronRight, Wrench } from 'lucide-react'

function formatJson(value: string): string {
  try {
    const parsed = JSON.parse(value)
    return JSON.stringify(parsed, null, 2)
  } catch {
    return value
  }
}

interface ToolCallCardProps {
  toolName: string
  args?: string
  result?: string
}

export default function ToolCallCard({ toolName, args, result }: ToolCallCardProps) {
  const [open, setOpen] = useState(false)
  return (
    <div className="my-1 rounded border border-neutral-200 dark:border-neutral-700 text-sm">
      <button
        onClick={() => setOpen(o => !o)}
        className="flex items-center gap-2 w-full px-3 py-1.5 text-left text-neutral-500 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
      >
        <Wrench className="w-3 h-3" />
        <span className="font-mono text-xs">{toolName}</span>
        {open ? <ChevronDown className="w-3 h-3 ml-auto" /> : <ChevronRight className="w-3 h-3 ml-auto" />}
      </button>
      {open && (
        <div className="px-3 py-2 space-y-2 bg-neutral-50 dark:bg-neutral-900/50 border-t border-neutral-200 dark:border-neutral-700">
          {args && (
            <div>
              <div className="text-xs font-semibold text-neutral-400 dark:text-neutral-500 mb-0.5">Args</div>
              <pre className="text-xs text-neutral-500 whitespace-pre-wrap break-all">{formatJson(args)}</pre>
            </div>
          )}
          {result && (
            <div>
              <div className="text-xs font-semibold text-neutral-400 dark:text-neutral-500 mb-0.5">Result</div>
              <pre className="text-xs text-neutral-600 dark:text-neutral-400 whitespace-pre-wrap break-all">{formatJson(result)}</pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

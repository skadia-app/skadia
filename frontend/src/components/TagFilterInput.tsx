import { useState, useRef, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchTags } from '@/api/tags'
import type { Tag } from '@/api/tags'

export type TagFilterState = 'include' | 'exclude'

export interface TagFilter {
  id: string
  name: string
  color: string
  state: TagFilterState
}

interface Props {
  value: TagFilter[]
  onChange: (filters: TagFilter[]) => void
  placeholder?: string
}

export default function TagFilterInput({ value, onChange, placeholder = 'Tag suchen...' }: Props) {
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const { data: allTags = [] } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })

  // Close dropdown on outside click
  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  const selectedIds = new Set(value.map((f) => f.id))

  const filtered = allTags.filter(
    (t) =>
      !selectedIds.has(t.id) &&
      t.name.toLowerCase().includes(search.toLowerCase().trim()),
  )

  function addTag(tag: Tag) {
    onChange([...value, { id: tag.id, name: tag.name, color: tag.color, state: 'include' }])
    setSearch('')
    setOpen(false)
  }

  function cycleChip(id: string) {
    const chip = value.find((f) => f.id === id)
    if (!chip) return
    if (chip.state === 'include') {
      // include → exclude
      onChange(value.map((f) => f.id === id ? { ...f, state: 'exclude' as TagFilterState } : f))
    } else {
      // exclude → remove
      onChange(value.filter((f) => f.id !== id))
    }
  }

  return (
    <div ref={containerRef} className="relative">
      {/* Selected chips */}
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mb-2">
          {value.map((chip) => (
            <button
              key={chip.id}
              type="button"
              onClick={() => cycleChip(chip.id)}
              title={chip.state === 'include' ? 'Klicken zum Ausschließen' : 'Klicken zum Entfernen'}
              className={[
                'inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium border transition-all',
                chip.state === 'include'
                  ? 'bg-green-100 dark:bg-green-900/40 border-green-400 dark:border-green-600 text-green-800 dark:text-green-200'
                  : 'bg-red-100 dark:bg-red-900/40 border-red-400 dark:border-red-600 text-red-700 dark:text-red-300 line-through',
              ].join(' ')}
            >
              <span
                className="w-2 h-2 rounded-full flex-shrink-0"
                style={{ backgroundColor: chip.color }}
              />
              {chip.name}
              <span className="ml-0.5 opacity-60 text-[10px]">
                {chip.state === 'include' ? '✓' : '✕'}
              </span>
            </button>
          ))}
        </div>
      )}

      {/* Search input */}
      <div className="relative">
        <input
          type="text"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setOpen(true)
          }}
          onFocus={() => setOpen(true)}
          placeholder={placeholder}
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent placeholder-neutral-400 dark:placeholder-neutral-500"
        />
        {search && (
          <button
            type="button"
            onClick={() => { setSearch(''); setOpen(false) }}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200"
          >
            ✕
          </button>
        )}
      </div>

      {/* Dropdown */}
      {open && filtered.length > 0 && (
        <div className="absolute z-50 mt-1 w-full bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg max-h-48 overflow-y-auto">
          {filtered.map((tag) => (
            <button
              key={tag.id}
              type="button"
              onMouseDown={(e) => {
                e.preventDefault() // prevent blur before click
                addTag(tag)
              }}
              className="w-full flex items-center gap-2 px-3 py-2 text-sm text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors text-left"
            >
              <span
                className="w-3 h-3 rounded-full flex-shrink-0"
                style={{ backgroundColor: tag.color }}
              />
              <span>{tag.name}</span>
              {tag.activity_count > 0 && (
                <span className="ml-auto text-xs text-neutral-400 dark:text-neutral-500">
                  {tag.activity_count}
                </span>
              )}
            </button>
          ))}
        </div>
      )}

      {open && search.trim() !== '' && filtered.length === 0 && (
        <div className="absolute z-50 mt-1 w-full bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg px-3 py-2 text-sm text-neutral-400 dark:text-neutral-500">
          Keine Tags gefunden
        </div>
      )}
    </div>
  )
}

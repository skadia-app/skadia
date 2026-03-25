import { useState, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchTags } from '@/api/tags'

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
  /** Hide chips and hint — for composite layouts that render chips externally */
  compact?: boolean
}

export default function TagFilterInput({ value, onChange, placeholder = 'Tag suchen...', compact }: Props) {
  const [search, setSearch] = useState('')
  const [listOpen, setListOpen] = useState(false)
  const listRef = useRef<HTMLDivElement>(null)

  const { data: allTags = [] } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })

  const filterMap = new Map(value.map((f) => [f.id, f.state]))

  const filtered = allTags.filter(
    (t) => t.name.toLowerCase().includes(search.toLowerCase().trim()),
  )

  function cycleTag(tag: { id: string; name: string; color: string }) {
    const current = filterMap.get(tag.id)
    if (!current) {
      onChange([...value, { id: tag.id, name: tag.name, color: tag.color, state: 'include' }])
    } else if (current === 'include') {
      onChange(value.map((f) => f.id === tag.id ? { ...f, state: 'exclude' as TagFilterState } : f))
    } else {
      onChange(value.filter((f) => f.id !== tag.id))
    }
  }

  function stateStyle(state: TagFilterState | undefined) {
    if (state === 'include') return 'bg-green-50 dark:bg-green-900/30 border-green-400 dark:border-green-700'
    if (state === 'exclude') return 'bg-red-50 dark:bg-red-900/30 border-red-400 dark:border-red-700'
    return 'border-transparent hover:bg-neutral-50 dark:hover:bg-neutral-800'
  }

  function stateIndicator(state: TagFilterState | undefined) {
    if (state === 'include') return <span className="ml-auto text-green-600 dark:text-green-400 text-xs">&#10003;</span>
    if (state === 'exclude') return <span className="ml-auto text-red-500 dark:text-red-400 text-xs">&#10005;</span>
    return null
  }

  const hasActive = value.length > 0

  function handleOpen() {
    setListOpen(true)
    requestAnimationFrame(() => {
      listRef.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    })
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Tags</span>
        {hasActive && !compact && (
          <button
            type="button"
            onClick={() => onChange([])}
            className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
          >
            Zurücksetzen
          </button>
        )}
      </div>

      {/* Search input */}
      <div className="relative">
        <input
          type="text"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            handleOpen()
          }}
          onClick={handleOpen}
          onFocus={handleOpen}
          placeholder={placeholder}
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent placeholder-neutral-400 dark:placeholder-neutral-500"
        />
        {search && (
          <button
            type="button"
            onClick={() => { setSearch(''); setListOpen(false) }}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
          >
            &#10005;
          </button>
        )}
      </div>

      {/* Tag list (inline, not absolute — avoids overflow clipping) */}
      {listOpen && filtered.length > 0 && (
        <div ref={listRef} className="mt-1.5 space-y-0.5 max-h-40 overflow-y-auto">
          {filtered.map((tag) => {
            const state = filterMap.get(tag.id)
            return (
              <button
                key={tag.id}
                type="button"
                onClick={() => cycleTag(tag)}
                className={`w-full flex items-center gap-2 px-2.5 py-1.5 text-sm rounded-md border transition-colors cursor-pointer text-left ${stateStyle(state)}`}
              >
                <span
                  className="w-2.5 h-2.5 rounded-full flex-shrink-0"
                  style={{ backgroundColor: tag.color }}
                />
                <span className={`text-neutral-800 dark:text-neutral-200 ${state === 'exclude' ? 'line-through' : ''}`}>
                  {tag.name}
                </span>
                {tag.activity_count > 0 && !state && (
                  <span className="ml-auto text-xs text-neutral-400 dark:text-neutral-500">
                    {tag.activity_count}
                  </span>
                )}
                {stateIndicator(state)}
              </button>
            )
          })}
        </div>
      )}

      {listOpen && search.trim() !== '' && filtered.length === 0 && (
        <div className="mt-1.5 px-3 py-2 text-sm text-neutral-400 dark:text-neutral-500">
          Keine Tags gefunden
        </div>
      )}

      {hasActive && !compact && (
        <p className="text-[10px] text-neutral-400 dark:text-neutral-500 mt-1.5">
          Klick: einschließen &rarr; ausschließen &rarr; entfernen
        </p>
      )}
    </div>
  )
}

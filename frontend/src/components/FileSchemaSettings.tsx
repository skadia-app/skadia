import { useRef, useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getFileSchemas,
  updateFileSchema,
  getLibraryDir,
  updateLibraryDir,
  type FileType,
} from '@/api/filestore'
import { useToast } from '@/lib/toast'

// ── i18n strings (centralised) ─────────────────────────────────────────────

const LABELS = {
  title: 'Dateibenennung',
  description: 'Benennungsschema für jeden Dateityp. Variablen werden durch Werte des jeweiligen Eintrags ersetzt.',
  availableVars: 'Verfügbare Variablen',
  preview: 'Vorschau',
  save: 'Speichern',
  saving: 'Speichert...',
  libraryDir: 'Bibliotheks-Verzeichnis',
  libraryDirDesc: 'Name des Wurzel-Verzeichnisses in der Bibliothek.',
  libraryDirPlaceholder: 'skadia-library',
  types: {
    activity: 'Aktivitäten',
    media: 'Medien',
    document: 'Dokumente',
    health: 'Gesundheit',
    route: 'Routen',
  } as Record<FileType, string>,
  errorLoad: 'Schemata konnten nicht geladen werden',
  errorSave: 'Fehler beim Speichern',
  saved: 'Schema gespeichert',
  savedLibDir: 'Bibliotheks-Verzeichnis gespeichert',
} as const

// ── Variable definitions per type ─────────────────────────────────────────

interface VarDef {
  key: string
  label: string
}

const TYPE_VARIABLES: Record<FileType, VarDef[]> = {
  activity: [
    { key: '{year}',        label: 'Jahr'         },
    { key: '{month}',       label: 'Monat'        },
    { key: '{day}',         label: 'Tag'          },
    { key: '{time}',        label: 'Uhrzeit'      },
    { key: '{sport}',       label: 'Sportart'     },
    { key: '{subCategory}', label: 'Unterkategorie'},
    { key: '{city}',        label: 'Stadt'        },
    { key: '{country}',     label: 'Land'         },
    { key: '{title}',       label: 'Titel'        },
    { key: '{distance_km}', label: 'Distanz (km)' },
    { key: '{duration_h}',  label: 'Dauer (h)'    },
    { key: '{elevation_m}', label: 'Höhenmeter'   },
  ],
  media: [
    { key: '{year}',        label: 'Jahr'          },
    { key: '{month}',       label: 'Monat'         },
    { key: '{day}',         label: 'Tag'           },
    { key: '{time}',        label: 'Uhrzeit'       },
    { key: '{title}',       label: 'Titel'         },
    { key: '{parent_type}', label: 'Eltern-Typ'    },
    { key: '{parent_name}', label: 'Eltern-Name'   },
    { key: '{camera}',      label: 'Kamera'        },
    { key: '{width}',       label: 'Breite (px)'   },
    { key: '{height}',      label: 'Höhe (px)'     },
  ],
  document: [
    { key: '{year}',        label: 'Jahr'        },
    { key: '{month}',       label: 'Monat'       },
    { key: '{category}',    label: 'Kategorie'   },
    { key: '{title}',       label: 'Titel'       },
    { key: '{parent_type}', label: 'Eltern-Typ'  },
    { key: '{parent_name}', label: 'Eltern-Name' },
  ],
  health: [
    { key: '{year}',  label: 'Jahr'  },
    { key: '{month}', label: 'Monat' },
    { key: '{day}',   label: 'Tag'   },
    { key: '{type}',  label: 'Typ'   },
  ],
  route: [
    { key: '{sport}',       label: 'Sportart'     },
    { key: '{title}',       label: 'Titel'        },
    { key: '{distance_km}', label: 'Distanz (km)' },
    { key: '{elevation_m}', label: 'Höhenmeter'   },
    { key: '{difficulty}',  label: 'Schwierigkeit'},
  ],
}

const EXAMPLE_VALUES: Record<string, string> = {
  '{year}':        '2024',
  '{month}':       '03',
  '{day}':         '15',
  '{time}':        '14-30',
  '{sport}':       'cycling',
  '{subCategory}': 'mountain',
  '{city}':        'Berlin',
  '{country}':     'Deutschland',
  '{title}':       'Feierabendrunde',
  '{distance_km}': '42',
  '{duration_h}':  '2',
  '{elevation_m}': '850',
  '{parent_type}': 'activity',
  '{parent_name}': 'Feierabendrunde',
  '{camera}':      'Nikon-Z6',
  '{width}':       '5504',
  '{height}':      '3096',
  '{category}':    'rechnung',
  '{type}':        'hrv',
  '{difficulty}':  'T3',
}

const FILE_TYPE_ORDER: FileType[] = ['activity', 'media', 'document', 'health', 'route']

// ── SchemaSection — one collapsible per file type ─────────────────────────

interface SchemaSectionProps {
  fileType: FileType
  initialSchema: string
}

function SchemaSection({ fileType, initialSchema }: SchemaSectionProps) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const inputRef = useRef<HTMLInputElement>(null)
  const [open, setOpen] = useState(false)
  const [localValue, setLocalValue] = useState<string | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])

  const currentValue = localValue ?? initialSchema
  const isDirty = localValue !== null && localValue !== initialSchema

  const preview = useMemo(() => {
    let result = currentValue
    for (const [key, val] of Object.entries(EXAMPLE_VALUES)) {
      result = result.replaceAll(key, val)
    }
    return result || '(leer)'
  }, [currentValue])

  const mut = useMutation({
    mutationFn: () => updateFileSchema(fileType, currentValue),
    onSuccess: (data) => {
      setWarnings(data.warnings ?? [])
      setLocalValue(null)
      queryClient.invalidateQueries({ queryKey: ['file-schemas'] })
      addToast({ type: 'success', title: LABELS.saved })
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : 'Unbekannter Fehler'
      addToast({ type: 'error', title: LABELS.errorSave, message: msg, duration: 0 })
    },
  })

  function insertVariable(variable: string) {
    const input = inputRef.current
    if (!input) return
    const start = input.selectionStart ?? currentValue.length
    const end = input.selectionEnd ?? currentValue.length
    const next = currentValue.slice(0, start) + variable + currentValue.slice(end)
    setLocalValue(next)
    requestAnimationFrame(() => {
      const pos = start + variable.length
      input.setSelectionRange(pos, pos)
      input.focus()
    })
  }

  const vars = TYPE_VARIABLES[fileType]

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg overflow-hidden">
      {/* Header / toggle */}
      <button
        onClick={() => setOpen(prev => !prev)}
        className="w-full flex items-center justify-between px-4 py-3 text-left cursor-pointer
                   bg-neutral-50 dark:bg-neutral-800/50
                   hover:bg-neutral-100 dark:hover:bg-neutral-800
                   transition-colors"
        aria-expanded={open}
      >
        <span className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
          {LABELS.types[fileType]}
        </span>
        <div className="flex items-center gap-3">
          {!open && initialSchema && (
            <span className="text-xs font-mono text-neutral-400 dark:text-neutral-500 truncate max-w-60">
              {initialSchema}
            </span>
          )}
          <svg
            className={`w-4 h-4 text-neutral-400 transition-transform ${open ? 'rotate-180' : ''}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
            aria-hidden="true"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </div>
      </button>

      {/* Body */}
      {open && (
        <div className="px-4 py-4 space-y-3 bg-white dark:bg-neutral-900/30">
          {/* Input + Save */}
          <div>
            <label className="block text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1">
              Schema-Template
            </label>
            <div className="flex gap-2">
              <input
                ref={inputRef}
                type="text"
                value={currentValue}
                onChange={e => setLocalValue(e.target.value)}
                className="flex-1 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2
                           bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100
                           placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                           focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT text-sm font-mono"
              />
              <button
                onClick={() => mut.mutate()}
                disabled={!isDirty || mut.isPending}
                className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium text-white
                           bg-accent-DEFAULT hover:bg-accent-600
                           disabled:opacity-40 disabled:cursor-not-allowed
                           transition-colors"
              >
                {mut.isPending ? LABELS.saving : LABELS.save}
              </button>
            </div>
          </div>

          {/* Variable chips */}
          <div className="space-y-1.5">
            <span className="text-xs text-neutral-500 dark:text-neutral-500">
              {LABELS.availableVars}
            </span>
            <div className="flex flex-wrap gap-1.5">
              {vars.map(v => (
                <button
                  key={v.key}
                  onClick={() => insertVariable(v.key)}
                  className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded-md
                             border border-neutral-200 dark:border-neutral-700
                             bg-neutral-50 dark:bg-neutral-800
                             text-neutral-600 dark:text-neutral-400
                             hover:bg-orange-50 hover:border-orange-300 hover:text-accent-600
                             dark:hover:bg-orange-900/20 dark:hover:border-orange-700 dark:hover:text-orange-400
                             cursor-pointer transition-colors"
                >
                  <span className="font-mono">{v.key}</span>
                  <span className="text-neutral-400 dark:text-neutral-600">{v.label}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Live preview */}
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 p-3">
            <span className="text-xs text-neutral-500 dark:text-neutral-500 block mb-1">
              {LABELS.preview}
            </span>
            <span className="text-sm font-mono text-neutral-800 dark:text-neutral-200 break-all">
              {preview}
            </span>
          </div>

          {/* Warnings from API */}
          {warnings.length > 0 && (
            <div className="rounded-lg border border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20 p-3 space-y-1">
              {warnings.map((w, i) => (
                <p key={i} className="text-xs text-amber-700 dark:text-amber-400">{w}</p>
              ))}
            </div>
          )}

          {/* Save error inline */}
          {mut.isError && (
            <p className="text-xs text-rose-500">
              {LABELS.errorSave}:{' '}
              {mut.error instanceof Error ? mut.error.message : 'Unbekannt'}
            </p>
          )}
        </div>
      )}
    </div>
  )
}

// ── Library directory section ─────────────────────────────────────────────

function LibraryDirSection() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const [localValue, setLocalValue] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['library-dir'],
    queryFn: getLibraryDir,
    staleTime: 60_000,
  })

  const currentValue = localValue ?? data?.library_dir_name ?? ''
  const isDirty = localValue !== null && localValue !== data?.library_dir_name

  const mut = useMutation({
    mutationFn: () => updateLibraryDir(currentValue),
    onSuccess: () => {
      setLocalValue(null)
      queryClient.invalidateQueries({ queryKey: ['library-dir'] })
      addToast({ type: 'success', title: LABELS.savedLibDir })
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : 'Unbekannter Fehler'
      addToast({ type: 'error', title: LABELS.errorSave, message: msg, duration: 0 })
    },
  })

  return (
    <div className="border border-neutral-200 dark:border-neutral-700 rounded-lg p-4 space-y-3">
      <div>
        <h3 className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
          {LABELS.libraryDir}
        </h3>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
          {LABELS.libraryDirDesc}
        </p>
      </div>
      <div className="flex gap-2">
        {isLoading ? (
          <div className="h-9 flex-1 rounded-lg bg-neutral-100 dark:bg-neutral-800 animate-pulse" />
        ) : (
          <input
            type="text"
            value={currentValue}
            onChange={e => setLocalValue(e.target.value)}
            placeholder={LABELS.libraryDirPlaceholder}
            className="flex-1 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2
                       bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100
                       placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                       focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT text-sm font-mono"
          />
        )}
        <button
          onClick={() => mut.mutate()}
          disabled={!isDirty || mut.isPending || isLoading}
          className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium text-white
                     bg-accent-DEFAULT hover:bg-accent-600
                     disabled:opacity-40 disabled:cursor-not-allowed
                     transition-colors"
        >
          {mut.isPending ? LABELS.saving : LABELS.save}
        </button>
      </div>
      {mut.isError && (
        <p className="text-xs text-rose-500">
          {LABELS.errorSave}:{' '}
          {mut.error instanceof Error ? mut.error.message : 'Unbekannt'}
        </p>
      )}
    </div>
  )
}

// ── Main component ─────────────────────────────────────────────────────────

export default function FileSchemaSettings() {
  const { data: schemas, isLoading, isError } = useQuery({
    queryKey: ['file-schemas'],
    queryFn: getFileSchemas,
    staleTime: 60_000,
  })

  // Build lookup by file_type for O(1) access
  const schemaMap = useMemo(() => {
    const map: Record<string, string> = {}
    for (const s of schemas ?? []) {
      map[s.file_type] = s.schema
    }
    return map
  }, [schemas])

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">
          {LABELS.title}
        </h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          {LABELS.description}
        </p>
      </div>

      {isError && (
        <p className="text-sm text-rose-500">{LABELS.errorLoad}</p>
      )}

      {isLoading ? (
        <div className="space-y-2">
          {FILE_TYPE_ORDER.map(ft => (
            <div key={ft} className="h-12 rounded-lg bg-neutral-100 dark:bg-neutral-800 animate-pulse" />
          ))}
        </div>
      ) : (
        <div className="space-y-2">
          {FILE_TYPE_ORDER.map(ft => (
            <SchemaSection
              key={ft}
              fileType={ft}
              initialSchema={schemaMap[ft] ?? ''}
            />
          ))}
        </div>
      )}

      <LibraryDirSection />
    </section>
  )
}

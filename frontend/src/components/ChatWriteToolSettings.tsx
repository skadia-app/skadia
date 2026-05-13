import { useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getChatPreferences, updateChatPreferences, type ChatPreferences } from '@/api/chat'
import Toggle from '@/components/Toggle'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { useToast } from '@/lib/toast'

type ToolGroup = {
  title: string
  tools: Array<{ id: string; label: string }>
}

const WRITE_TOOL_GROUPS: ToolGroup[] = [
  {
    title: 'Gear',
    tools: [
      { id: 'create_gear', label: 'Gear anlegen' },
      { id: 'update_gear_metadata', label: 'Gear-Metadaten ändern' },
      { id: 'retire_gear', label: 'Gear in den Ruhestand versetzen' },
      { id: 'set_activity_gear', label: 'Gear einer Aktivität zuweisen' },
    ],
  },
  {
    title: 'Aktivitäten',
    tools: [
      { id: 'update_activity_metadata', label: 'Aktivitäts-Metadaten ändern' },
      { id: 'rename_activity', label: 'Aktivität umbenennen' },
      { id: 'tag_activity', label: 'Aktivität taggen' },
    ],
  },
  {
    title: 'Gesundheit',
    tools: [
      { id: 'record_health_metric', label: 'Gesundheitswert eintragen' },
    ],
  },
  {
    title: 'Tags',
    tools: [
      { id: 'create_tag', label: 'Tag anlegen' },
      { id: 'update_tag', label: 'Tag ändern' },
      { id: 'delete_tag', label: 'Tag löschen' },
    ],
  },
  {
    title: 'Automation',
    tools: [
      { id: 'create_automation_rule', label: 'Regel anlegen' },
      { id: 'update_automation_rule', label: 'Regel ändern' },
      { id: 'delete_automation_rule', label: 'Regel löschen' },
      { id: 'enable_automation_rule', label: 'Regel aktivieren' },
      { id: 'disable_automation_rule', label: 'Regel deaktivieren' },
    ],
  },
]

const DEFAULT_CHAT_PREFS: ChatPreferences = {
  chat_expose_document_extracted_data: false,
  chat_write_tools_enabled: false,
  chat_allowed_write_tools: [],
}

export default function ChatWriteToolSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { data, isLoading, isError, isSuccess } = useQuery({
    queryKey: ['chat-preferences'],
    queryFn: getChatPreferences,
  })

  const preferencesLoaded = isSuccess && data !== undefined
  const prefs = data ?? DEFAULT_CHAT_PREFS
  const allowedSet = useMemo(
    () => new Set(prefs.chat_allowed_write_tools),
    [prefs.chat_allowed_write_tools],
  )

  const mutation = useMutation({
    mutationFn: updateChatPreferences,
    onMutate: async next => {
      await queryClient.cancelQueries({ queryKey: ['chat-preferences'] })
      const previous = queryClient.getQueryData<ChatPreferences>(['chat-preferences'])
      queryClient.setQueryData(['chat-preferences'], next)
      return { previous }
    },
    onSuccess: saved => {
      queryClient.setQueryData(['chat-preferences'], saved)
      addToast({ type: 'success', title: 'Chat-Einstellungen gespeichert' })
    },
    onError: (_err, _next, context) => {
      if (context?.previous) {
        queryClient.setQueryData(['chat-preferences'], context.previous)
      }
      addToast({ type: 'error', title: 'Chat-Einstellungen konnten nicht gespeichert werden', duration: 0 })
    },
  })

  const save = (patch: Partial<ChatPreferences>) => {
    if (!preferencesLoaded) return
    const current = queryClient.getQueryData<ChatPreferences>(['chat-preferences']) ?? prefs
    mutation.mutate({ ...current, ...patch })
  }

  const setToolAllowed = (toolID: string, checked: boolean) => {
    const next = new Set(prefs.chat_allowed_write_tools)
    if (checked) next.add(toolID)
    else next.delete(toolID)
    save({ chat_allowed_write_tools: Array.from(next) })
  }

  const controlsDisabled = isLoading || mutation.isPending || !preferencesLoaded
  const writeToolsDisabled = controlsDisabled || !prefs.chat_write_tools_enabled

  return (
    <SettingsSection
      title="Chat-Assistent"
      description="Steuert, welche sensiblen Dokumentdaten und Änderungsvorschläge der Assistent verwenden darf."
      className="space-y-6"
    >
      {isError && (
        <p className="text-sm text-red-500 dark:text-red-400">Chat-Einstellungen konnten nicht geladen werden.</p>
      )}

      <div className="space-y-4">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              Dokument-OCR-Text dem Chat-Assistenten zugänglich machen
            </div>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
              OCR-Text kann Adressen, Namen oder Beträge enthalten und wird bei passenden Fragen mit dem konfigurierten LLM-Anbieter geteilt.
            </p>
          </div>
          <Toggle
            checked={prefs.chat_expose_document_extracted_data}
            onChange={checked => save({ chat_expose_document_extracted_data: checked })}
            disabled={controlsDisabled}
            aria-label="Dokument-OCR-Text dem Chat-Assistenten zugänglich machen"
          />
        </div>

        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              Chat-Assistent darf Änderungen vorschlagen
            </div>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
              Aktivierte Werkzeuge zeigen vor jeder Änderung eine Vorschau und erfordern deine ausdrückliche Bestätigung.
            </p>
          </div>
          <Toggle
            checked={prefs.chat_write_tools_enabled}
            onChange={checked => save({ chat_write_tools_enabled: checked })}
            disabled={controlsDisabled}
            aria-label="Chat-Assistent darf Änderungen vorschlagen"
          />
        </div>
      </div>

      <div className={writeToolsDisabled ? 'opacity-60' : undefined}>
        <div className="grid gap-4 md:grid-cols-2">
          {WRITE_TOOL_GROUPS.map(group => (
            <fieldset key={group.title} className="space-y-2">
              <legend className="text-sm font-medium text-neutral-800 dark:text-neutral-200">{group.title}</legend>
              <div className="space-y-1.5">
                {group.tools.map(tool => (
                  <label key={tool.id} className="flex items-center gap-2 text-sm text-neutral-700 dark:text-neutral-300">
                    <input
                      type="checkbox"
                      className="h-4 w-4 rounded border-neutral-300 text-accent-DEFAULT focus:ring-accent-DEFAULT disabled:cursor-not-allowed"
                      checked={allowedSet.has(tool.id)}
                      disabled={writeToolsDisabled}
                      onChange={e => setToolAllowed(tool.id, e.target.checked)}
                    />
                    <span>{tool.label}</span>
                  </label>
                ))}
              </div>
            </fieldset>
          ))}
        </div>
      </div>
    </SettingsSection>
  )
}

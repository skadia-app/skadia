import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getChatPreferences, updateChatPreferences, type ChatPreferences } from '@/api/chat'
import Toggle from '@/components/Toggle'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { useCapabilities } from '@/hooks/useCapabilities'
import { featureAvailable, type CapabilityKey } from '@/lib/deploymentCapabilities'
import { useToast } from '@/lib/toast'

type ToolGroup = {
  title: string
  tools: Array<{ id: string; label: string; requiredCapability?: CapabilityKey }>
}

const WRITE_TOOL_GROUPS: ToolGroup[] = [
  {
    title: 'Gear',
    tools: [
      { id: 'create_gear', label: 'Gear anlegen' },
      { id: 'update_gear_metadata', label: 'Gear-Metadaten ändern' },
      { id: 'retire_gear', label: 'Gear in den Ruhestand versetzen' },
      { id: 'set_activity_gear', label: 'Gear einer Aktivität zuweisen' },
      { id: 'batch_set_activity_gear', label: 'Gear vielen Aktivitäten zuweisen' },
      { id: 'add_gear_component', label: 'Gear-Komponente anlegen' },
      { id: 'update_gear_component_status', label: 'Komponentenstatus ändern' },
      { id: 'record_gear_service_event', label: 'Service-Ereignis erfassen' },
      { id: 'update_gear_default_predicates', label: 'Gear-Standards ändern' },
    ],
  },
  {
    title: 'Aktivitäten',
    tools: [
      { id: 'update_activity_metadata', label: 'Aktivitäts-Metadaten ändern' },
      { id: 'rename_activity', label: 'Aktivität umbenennen' },
      { id: 'tag_activity', label: 'Aktivität taggen' },
      { id: 'bulk_tag_activities', label: 'Viele Aktivitäten taggen' },
      { id: 'start_recompute_activities', label: 'Aktivitäten neu berechnen' },
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
      { id: 'merge_tags', label: 'Tags zusammenführen' },
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
      { id: 'apply_automation_rule_retroactively', label: 'Regel rückwirkend anwenden' },
    ],
  },
  {
    title: 'Routen',
    tools: [
      { id: 'save_planned_route', label: 'Geplante Route speichern' },
    ],
  },
]

const DEFAULT_CHAT_PREFS: ChatPreferences = {
  chat_expose_document_extracted_data: false,
  chat_write_tools_enabled: false,
  chat_allowed_write_tools: [],
}

function samePreferences(a: ChatPreferences, b: ChatPreferences): boolean {
  if (a.chat_expose_document_extracted_data !== b.chat_expose_document_extracted_data) return false
  if (a.chat_write_tools_enabled !== b.chat_write_tools_enabled) return false
  if (a.chat_allowed_write_tools.length !== b.chat_allowed_write_tools.length) return false
  return a.chat_allowed_write_tools.every((tool, index) => tool === b.chat_allowed_write_tools[index])
}

export default function ChatWriteToolSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { data: capabilities } = useCapabilities()
  const [draft, setDraft] = useState<ChatPreferences | null>(null)
  const { data, isLoading, isError, isSuccess } = useQuery({
    queryKey: ['chat-preferences'],
    queryFn: getChatPreferences,
  })

  const preferencesLoaded = isSuccess && data !== undefined
  const savedPrefs = data ?? DEFAULT_CHAT_PREFS
  const prefs = draft ?? savedPrefs
  const isDirty = preferencesLoaded && draft !== null && !samePreferences(draft, savedPrefs)
  const allowedSet = useMemo(
    () => new Set(prefs.chat_allowed_write_tools),
    [prefs.chat_allowed_write_tools],
  )

  useEffect(() => {
    if (preferencesLoaded && draft === null) {
      setDraft(savedPrefs)
    }
  }, [draft, preferencesLoaded, savedPrefs])

  const mutation = useMutation({
    mutationFn: updateChatPreferences,
    onSuccess: saved => {
      queryClient.setQueryData(['chat-preferences'], saved)
      setDraft(saved)
      addToast({ type: 'success', title: 'Agent-Berechtigungen gespeichert' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Agent-Berechtigungen konnten nicht gespeichert werden', duration: 0 })
    },
  })

  const patchDraft = (patch: Partial<ChatPreferences>) => {
    if (!preferencesLoaded) return
    setDraft(current => ({ ...(current ?? savedPrefs), ...patch }))
  }

  const setToolAllowed = (toolID: string, checked: boolean) => {
    const next = new Set(prefs.chat_allowed_write_tools)
    if (checked) next.add(toolID)
    else next.delete(toolID)
    patchDraft({ chat_allowed_write_tools: Array.from(next) })
  }

  const controlsDisabled = isLoading || mutation.isPending || !preferencesLoaded
  const writeToolsDisabled = controlsDisabled || !prefs.chat_write_tools_enabled
  const documentTextAvailable = capabilities === undefined || featureAvailable(capabilities, 'document_ml')
  const visibleToolGroups = WRITE_TOOL_GROUPS.map(group => ({
    ...group,
    tools: group.tools.filter(tool => !tool.requiredCapability || capabilities === undefined || featureAvailable(capabilities, tool.requiredCapability)),
  })).filter(group => group.tools.length > 0)

  return (
    <SettingsSection
      title="AI-Agent-Berechtigungen"
      description="Steuert, welche sensiblen Dokumentdaten und Änderungsvorschläge der Assistent verwenden darf."
      className="space-y-6"
    >
      {isError && (
        <p className="text-sm text-red-500 dark:text-red-400">Agent-Berechtigungen konnten nicht geladen werden.</p>
      )}

      <div className="space-y-4">
        {documentTextAvailable && (
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                Dokument-OCR-Text dem AI-Agenten zugänglich machen
              </div>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
                OCR-Text kann Adressen, Namen oder Beträge enthalten und wird bei passenden Fragen mit dem konfigurierten LLM-Anbieter geteilt.
              </p>
            </div>
            <Toggle
              checked={prefs.chat_expose_document_extracted_data}
              onChange={checked => patchDraft({ chat_expose_document_extracted_data: checked })}
              disabled={controlsDisabled}
              aria-label="Dokument-OCR-Text dem AI-Agenten zugänglich machen"
            />
          </div>
        )}

        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              AI-Agent darf Änderungen vorschlagen
            </div>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
              Aktivierte Werkzeuge zeigen vor jeder Änderung eine Vorschau und erfordern deine ausdrückliche Bestätigung.
            </p>
          </div>
          <Toggle
            checked={prefs.chat_write_tools_enabled}
            onChange={checked => patchDraft({ chat_write_tools_enabled: checked })}
            disabled={controlsDisabled}
            aria-label="AI-Agent darf Änderungen vorschlagen"
          />
        </div>
      </div>

      <div className={writeToolsDisabled ? 'opacity-60' : undefined}>
        <div className="grid gap-4 md:grid-cols-2">
          {visibleToolGroups.map(group => (
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

      <div className="flex items-center justify-end gap-3 border-t border-neutral-100 dark:border-neutral-800 pt-4">
        {isDirty && (
          <button
            type="button"
            onClick={() => setDraft(savedPrefs)}
            disabled={mutation.isPending}
            className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 underline disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            Abbrechen
          </button>
        )}
        <button
          type="button"
          onClick={() => mutation.mutate(prefs)}
          disabled={!isDirty || mutation.isPending}
          className="px-4 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
        >
          {mutation.isPending ? 'Speichern…' : 'Speichern'}
        </button>
      </div>
    </SettingsSection>
  )
}

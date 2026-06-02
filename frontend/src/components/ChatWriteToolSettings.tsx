import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getChatPreferences, updateChatPreferences, type ChatPreferences } from '@/api/chat'
import Toggle from '@/components/Toggle'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { useCapabilities } from '@/hooks/useCapabilities'
import { featureAvailable, type CapabilityKey } from '@/lib/deploymentCapabilities'
import { useToast } from '@/lib/toast'

type ToolGroup = {
  titleKey: string
  tools: Array<{ id: string; labelKey: string; requiredCapability?: CapabilityKey }>
}

const WRITE_TOOL_GROUPS: ToolGroup[] = [
  {
    titleKey: 'components.chatWriteToolSettings.groups.gear',
    tools: [
      { id: 'create_gear', labelKey: 'components.chatWriteToolSettings.tools.create_gear' },
      { id: 'update_gear_metadata', labelKey: 'components.chatWriteToolSettings.tools.update_gear_metadata' },
      { id: 'retire_gear', labelKey: 'components.chatWriteToolSettings.tools.retire_gear' },
      { id: 'set_activity_gear', labelKey: 'components.chatWriteToolSettings.tools.set_activity_gear' },
      { id: 'batch_set_activity_gear', labelKey: 'components.chatWriteToolSettings.tools.batch_set_activity_gear' },
      { id: 'add_gear_component', labelKey: 'components.chatWriteToolSettings.tools.add_gear_component' },
      { id: 'update_gear_component_status', labelKey: 'components.chatWriteToolSettings.tools.update_gear_component_status' },
      { id: 'record_gear_service_event', labelKey: 'components.chatWriteToolSettings.tools.record_gear_service_event' },
      { id: 'create_chain_rotation', labelKey: 'components.chatWriteToolSettings.tools.create_chain_rotation' },
      { id: 'update_gear_default_predicates', labelKey: 'components.chatWriteToolSettings.tools.update_gear_default_predicates' },
    ],
  },
  {
    titleKey: 'components.chatWriteToolSettings.groups.activities',
    tools: [
      { id: 'update_activity_metadata', labelKey: 'components.chatWriteToolSettings.tools.update_activity_metadata' },
      { id: 'rename_activity', labelKey: 'components.chatWriteToolSettings.tools.rename_activity' },
      { id: 'tag_activity', labelKey: 'components.chatWriteToolSettings.tools.tag_activity' },
      { id: 'bulk_tag_activities', labelKey: 'components.chatWriteToolSettings.tools.bulk_tag_activities' },
      { id: 'start_recompute_activities', labelKey: 'components.chatWriteToolSettings.tools.start_recompute_activities' },
    ],
  },
  {
    titleKey: 'components.chatWriteToolSettings.groups.health',
    tools: [
      { id: 'record_health_metric', labelKey: 'components.chatWriteToolSettings.tools.record_health_metric' },
    ],
  },
  {
    titleKey: 'components.chatWriteToolSettings.groups.tags',
    tools: [
      { id: 'create_tag', labelKey: 'components.chatWriteToolSettings.tools.create_tag' },
      { id: 'update_tag', labelKey: 'components.chatWriteToolSettings.tools.update_tag' },
      { id: 'delete_tag', labelKey: 'components.chatWriteToolSettings.tools.delete_tag' },
      { id: 'merge_tags', labelKey: 'components.chatWriteToolSettings.tools.merge_tags' },
    ],
  },
  {
    titleKey: 'components.chatWriteToolSettings.groups.automation',
    tools: [
      { id: 'create_automation_rule', labelKey: 'components.chatWriteToolSettings.tools.create_automation_rule' },
      { id: 'update_automation_rule', labelKey: 'components.chatWriteToolSettings.tools.update_automation_rule' },
      { id: 'delete_automation_rule', labelKey: 'components.chatWriteToolSettings.tools.delete_automation_rule' },
      { id: 'enable_automation_rule', labelKey: 'components.chatWriteToolSettings.tools.enable_automation_rule' },
      { id: 'disable_automation_rule', labelKey: 'components.chatWriteToolSettings.tools.disable_automation_rule' },
      { id: 'apply_automation_rule_retroactively', labelKey: 'components.chatWriteToolSettings.tools.apply_automation_rule_retroactively' },
    ],
  },
  {
    titleKey: 'components.chatWriteToolSettings.groups.routes',
    tools: [
      { id: 'save_planned_route', labelKey: 'components.chatWriteToolSettings.tools.save_planned_route' },
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
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { data: capabilities, isLoading: capabilitiesLoading } = useCapabilities()
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
      addToast({ type: 'success', title: t('components.chatWriteToolSettings.toasts.saved') })
    },
    onError: () => {
      addToast({ type: 'error', title: t('components.chatWriteToolSettings.toasts.saveFailed'), duration: 0 })
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

  const capabilitiesKnown = capabilities !== undefined
  const controlsDisabled = isLoading || mutation.isPending || !preferencesLoaded || capabilitiesLoading || !capabilitiesKnown
  const writeToolsDisabled = controlsDisabled || !prefs.chat_write_tools_enabled
  const documentTextAvailable = capabilitiesKnown && featureAvailable(capabilities, 'document_ml')
  const visibleToolGroups = WRITE_TOOL_GROUPS.map(group => ({
    ...group,
    tools: capabilitiesKnown
      ? group.tools.filter(tool => !tool.requiredCapability || featureAvailable(capabilities, tool.requiredCapability))
      : [],
  })).filter(group => group.tools.length > 0)

  return (
    <SettingsSection
      title={t('components.chatWriteToolSettings.title')}
      description={t('components.chatWriteToolSettings.description')}
      className="space-y-6"
    >
      {isError && (
        <p className="text-sm text-red-500 dark:text-red-400">{t('components.chatWriteToolSettings.loadError')}</p>
      )}

      <div className="space-y-4">
        {documentTextAvailable && (
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
                {t('components.chatWriteToolSettings.documentText.label')}
              </div>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
                {t('components.chatWriteToolSettings.documentText.description')}
              </p>
            </div>
            <Toggle
              checked={prefs.chat_expose_document_extracted_data}
              onChange={checked => patchDraft({ chat_expose_document_extracted_data: checked })}
              disabled={controlsDisabled}
              aria-label={t('components.chatWriteToolSettings.documentText.label')}
            />
          </div>
        )}

        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              {t('components.chatWriteToolSettings.writeTools.label')}
            </div>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
              {t('components.chatWriteToolSettings.writeTools.description')}
            </p>
          </div>
          <Toggle
            checked={prefs.chat_write_tools_enabled}
            onChange={checked => patchDraft({ chat_write_tools_enabled: checked })}
            disabled={controlsDisabled}
            aria-label={t('components.chatWriteToolSettings.writeTools.label')}
          />
        </div>
      </div>

      <div className={writeToolsDisabled ? 'opacity-60' : undefined}>
        <div className="grid gap-4 md:grid-cols-2">
          {visibleToolGroups.map(group => (
            <fieldset key={group.titleKey} className="space-y-2">
              <legend className="text-sm font-medium text-neutral-800 dark:text-neutral-200">{t(group.titleKey)}</legend>
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
                    <span>{t(tool.labelKey)}</span>
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
            {t('common.cancel')}
          </button>
        )}
        <button
          type="button"
          onClick={() => mutation.mutate(prefs)}
          disabled={!isDirty || mutation.isPending}
          className="px-4 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
        >
          {mutation.isPending ? t('common.saving') : t('common.save')}
        </button>
      </div>
    </SettingsSection>
  )
}

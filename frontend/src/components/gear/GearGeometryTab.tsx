import { useTranslation } from 'react-i18next'
import type { Attribute, GearSchema } from '@/lib/generatedGearSchema'
import { gearSchemaLabel, gearTypeAttributes, optionSetOptions } from '@/lib/gearSchemaHelpers'
import type { Gear } from '@/api/types'
import RecursiveSubfieldRenderer from '@/components/gear/RecursiveSubfieldRenderer'
import { supportedLanguageFrom } from '@/i18n/localizedString'

interface GearGeometryTabProps {
  item: Gear
  schema: GearSchema
  onChange: (patch: Record<string, unknown>) => void
}

export default function GearGeometryTab({ item, schema, onChange }: GearGeometryTabProps) {
  const { i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const attrs = gearTypeAttributes(schema, item.type)
  const geometryAttrs = attrs.filter(
    (a) => (a.valueType === 'geometry' || a.valueType === 'object') && a.subfields && a.subfields.length > 0,
  )

  if (geometryAttrs.length === 0) return null

  const storedAttrs =
    typeof item.attributes === 'object' && item.attributes !== null
      ? (item.attributes as Record<string, unknown>)
      : {}

  // Strip the gear-type prefix from the full attribute key to get the storage key.
  // Convention: bike.geometry is stored as attributes.geometry.
  function attrStorageKey(fullKey: string): string {
    const dotIdx = fullKey.indexOf('.')
    return dotIdx >= 0 ? fullKey.slice(dotIdx + 1) : fullKey
  }

  function resolveOptions(optionSetKey: string | undefined) {
    return optionSetOptions(schema, optionSetKey)
  }

  return (
    <div className="space-y-6">
      {geometryAttrs.map((attr: Attribute) => {
        const storageKey = attrStorageKey(attr.key)
        const attrValues =
          typeof storedAttrs[storageKey] === 'object' && storedAttrs[storageKey] !== null
            ? (storedAttrs[storageKey] as Record<string, unknown>)
            : {}

        return (
          <section key={attr.key} className="space-y-3">
            <h3 className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">
              {gearSchemaLabel(attr, language)}
            </h3>
            <RecursiveSubfieldRenderer
              subfields={attr.subfields!}
              values={attrValues}
              prefix={attr.key}
              depth={1}
              onChange={(subfieldKey, value) => {
                onChange({ [storageKey]: { ...attrValues, [subfieldKey]: value } })
              }}
              resolveOptions={resolveOptions}
            />
          </section>
        )
      })}
    </div>
  )
}

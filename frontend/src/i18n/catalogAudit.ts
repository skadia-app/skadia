import { PROTECTED_TERMS, type ProtectedTerm } from './protectedTerms'
import { SUPPORTED_LANGUAGES, type SupportedLanguage } from './languages'
import type { CatalogResources, TranslationTree } from './resources'

export type CatalogAuditIssue =
  | { code: 'missing-key'; locale: SupportedLanguage; key: string }
  | { code: 'extra-key'; locale: SupportedLanguage; key: string }
  | { code: 'empty-value'; locale: SupportedLanguage; key: string }
  | { code: 'protected-term-changed'; locale: SupportedLanguage; key: string; term: ProtectedTerm }
  | { code: 'german-ascii-umlaut'; locale: 'de'; key: string; text: string }

const GERMAN_ASCII_UMLAUT_PATTERNS = [
  /\bSchliessen\b/,
  /\bschliessen\b/,
  /\bVerfuegbare\b/,
  /\bverfuegbare\b/,
  /\beinfuegen\b/,
  /\bEinfuegen\b/,
  /\bfuer\b/,
  /\bFuer\b/,
  /\bzurueck\b/,
  /\bZurueck\b/,
  /\bwaehlen\b/,
  /\bWaehlen\b/,
  /\bpruefen\b/,
  /\bPruefen\b/,
  /\bloeschen\b/,
  /\bLoeschen\b/,
]

function flattenCatalog(tree: TranslationTree, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(tree)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (typeof value === 'string') {
      out[path] = value
    } else {
      Object.assign(out, flattenCatalog(value, path))
    }
  }
  return out
}

export function auditCatalogResources(resources: CatalogResources): CatalogAuditIssue[] {
  const flattened = Object.fromEntries(
    SUPPORTED_LANGUAGES.map(locale => [locale, flattenCatalog(resources[locale])]),
  ) as Record<SupportedLanguage, Record<string, string>>
  const source = flattened.de
  const sourceKeys = new Set(Object.keys(source))
  const issues: CatalogAuditIssue[] = []

  for (const locale of SUPPORTED_LANGUAGES) {
    const catalog = flattened[locale]
    for (const key of sourceKeys) {
      if (!(key in catalog)) {
        issues.push({ code: 'missing-key', locale, key })
        continue
      }
      if (catalog[key].trim() === '') {
        issues.push({ code: 'empty-value', locale, key })
      }
      if (locale === 'de' && GERMAN_ASCII_UMLAUT_PATTERNS.some(pattern => pattern.test(catalog[key]))) {
        issues.push({ code: 'german-ascii-umlaut', locale, key, text: catalog[key] })
      }
      for (const term of PROTECTED_TERMS) {
        if (source[key].includes(term) && !catalog[key].includes(term)) {
          issues.push({ code: 'protected-term-changed', locale, key, term })
        }
      }
    }
    for (const key of Object.keys(catalog)) {
      if (!sourceKeys.has(key)) {
        issues.push({ code: 'extra-key', locale, key })
      }
    }
  }

  return issues
}

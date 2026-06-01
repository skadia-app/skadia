import * as ts from 'typescript'

export interface HardcodedTextIssue {
  code: 'hardcoded-ui-text'
  file: string
  text: string
  location: 'jsx-text' | 'jsx-attribute' | 'call-argument' | 'object-property'
}

const UI_ATTRIBUTE_NAMES = new Set(['aria-label', 'title', 'placeholder', 'alt'])
const UI_CALL_NAMES = new Set(['alert', 'confirm', 'prompt', 'toast', 'notify', 'showToast', 'Notification'])
const UI_PROPERTY_NAMES = new Set([
  'actionLabel',
  'alt',
  'description',
  'emptyText',
  'errorText',
  'label',
  'loadingText',
  'message',
  'placeholder',
  'subtitle',
  'title',
  'tooltip',
])

function normalizedText(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

function isHumanText(text: string): boolean {
  const value = normalizedText(text)
  if (value.length < 2) return false
  if (value.startsWith('/api/') || value.startsWith('/')) return false
  if (/^(text|bg|border|rounded|font|flex|grid|items|justify|gap|p|m|w|h)-/.test(value)) return false
  if (/^[a-z][a-z0-9]*(?:[.-][A-Za-z0-9_{}-]+)+$/.test(value)) return false
  return /[A-Za-zÄÖÜäöüßÀ-ÿ]/.test(value)
}

function literalText(node: ts.Node): string | undefined {
  return ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node) ? node.text : undefined
}

function calleeNames(node: ts.Expression): string[] {
  if (ts.isIdentifier(node)) return [node.text]
  if (ts.isPropertyAccessExpression(node)) return [node.name.text, ...calleeNames(node.expression)]
  return []
}

function propertyName(node: ts.PropertyName): string | undefined {
  if (ts.isIdentifier(node) || ts.isStringLiteral(node) || ts.isNumericLiteral(node)) return node.text
  return undefined
}

export function auditSourceText(file: string, source: string): HardcodedTextIssue[] {
  const sourceFile = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
  const issues: HardcodedTextIssue[] = []

  function visit(node: ts.Node) {
    if (ts.isJsxText(node)) {
      const text = normalizedText(node.getText(sourceFile))
      if (isHumanText(text)) {
        issues.push({ code: 'hardcoded-ui-text', file, text, location: 'jsx-text' })
      }
    }

    if (ts.isJsxAttribute(node) && ts.isIdentifier(node.name) && UI_ATTRIBUTE_NAMES.has(node.name.text)) {
      const initializer = node.initializer
      if (initializer && ts.isStringLiteral(initializer)) {
        const text = normalizedText(initializer.text)
        if (isHumanText(text)) {
          issues.push({ code: 'hardcoded-ui-text', file, text, location: 'jsx-attribute' })
        }
      }
    }

    if (ts.isCallExpression(node) && calleeNames(node.expression).some(name => UI_CALL_NAMES.has(name))) {
      const text = node.arguments[0] ? literalText(node.arguments[0]) : undefined
      if (text && isHumanText(text)) {
        issues.push({ code: 'hardcoded-ui-text', file, text: normalizedText(text), location: 'call-argument' })
      }
    }

    if (ts.isNewExpression(node) && node.expression.getText(sourceFile) === 'Notification') {
      const text = node.arguments?.[0] ? literalText(node.arguments[0]) : undefined
      if (text && isHumanText(text)) {
        issues.push({ code: 'hardcoded-ui-text', file, text: normalizedText(text), location: 'call-argument' })
      }
    }

    if (ts.isPropertyAssignment(node) && UI_PROPERTY_NAMES.has(propertyName(node.name) ?? '')) {
      const text = literalText(node.initializer)
      if (text && isHumanText(text)) {
        issues.push({ code: 'hardcoded-ui-text', file, text: normalizedText(text), location: 'object-property' })
      }
    }

    ts.forEachChild(node, visit)
  }

  visit(sourceFile)
  return issues
}

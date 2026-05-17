import { describe, expect, it } from 'vitest'
import { activeImportStepFromSteps } from './importSteps'

describe('activeImportStepFromSteps', () => {
  it('returns the first non-terminal step', () => {
    expect(activeImportStepFromSteps([
      { key: 'upload', label: 'Upload', status: 'completed' },
      { key: 'text_extraction', label: 'Texterkennung', status: 'processing' },
      { key: 'markdown', label: 'Markdown', status: 'pending' },
    ])).toEqual({ current: 2, total: 3, label: 'Texterkennung' })
  })

  it('returns null when every step is terminal', () => {
    expect(activeImportStepFromSteps([
      { key: 'upload', label: 'Upload', status: 'completed' },
      { key: 'text_extraction', label: 'Texterkennung', status: 'failed' },
      { key: 'markdown', label: 'Markdown', status: 'skipped' },
    ])).toBeNull()
  })

  it('ignores malformed step entries', () => {
    expect(activeImportStepFromSteps([
      null,
      { key: 'upload', status: 'pending' },
      { key: 'markdown', label: 'Markdown', status: 'pending' },
    ])).toEqual({ current: 1, total: 1, label: 'Markdown' })
  })
})

import { describe, expect, it } from 'vitest'

import { komootAvailabilityText, komootCompletionLog, komootProgressLabel } from './ImportPage'

describe('Komoot import labels', () => {
  it('uses route wording for route-only imports', () => {
    const selection = { importRecorded: false, importPlanned: true, importPhotos: false }

    expect(komootProgressLabel(129, 129, selection)).toBe('Route 129 / 129')
    expect(komootCompletionLog(129, selection)).toBe('Abgeschlossen — 129 Routen importiert')
    expect(komootAvailabilityText(selection)).toBe('Die importierten Routen sind nun verfügbar.')
  })
})

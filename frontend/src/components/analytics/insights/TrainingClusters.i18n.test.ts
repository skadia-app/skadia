import { describe, expect, it } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { formatClusterDistance, localizedClusterLabel } from './TrainingClusters'

describe('localizedClusterLabel', () => {
  it('uses stable cluster label keys instead of cached raw labels', async () => {
    await i18n.changeLanguage('en')

    expect(localizedClusterLabel({ label: 'Berg', label_key: 'mountain' }, i18n.t.bind(i18n), 'Cluster 1')).toBe('Climb')
  })

  it('maps legacy cached labels to the active language', async () => {
    await i18n.changeLanguage('es')

    expect(localizedClusterLabel({ label: 'Erholung' }, i18n.t.bind(i18n), 'Cluster 1')).toBe('Recuperación')
  })

  it('formats cluster distances through the active unit formatter', () => {
    expect(formatClusterDistance(1609.344, meters => `${meters / 1609.344} mi`)).toBe('1 mi')
  })
})

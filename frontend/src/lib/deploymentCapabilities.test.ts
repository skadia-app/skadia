import { describe, expect, it } from 'vitest'

import {
  deploymentProfile,
  featureAvailable,
  featureConfigured,
  featureReason,
  legacyCapability,
  storageBackend,
  storageMode,
  validateCapabilityKey,
} from './deploymentCapabilities'
import type { Capabilities } from '@/api/routes'

describe('deploymentCapabilities', () => {
  it('reads the new feature-state response shape', () => {
    const caps: Capabilities = {
      deployment_profile: 'core',
      deployment_profile_source: 'env',
      storage_mode: 's3-local-garage',
      storage_backend: 's3',
      features: {
        badrouter: { configured: false, available: false, reason: 'core_profile' },
        brouter: { configured: true, available: true },
      },
      badrouter: false,
      brouter: true,
    }

    expect(deploymentProfile(caps)).toBe('core')
    expect(storageMode(caps)).toBe('s3-local-garage')
    expect(storageBackend(caps)).toBe('s3')
    expect(featureConfigured(caps, 'badrouter')).toBe(false)
    expect(featureAvailable(caps, 'badrouter')).toBe(false)
    expect(featureReason(caps, 'badrouter')).toBe('core_profile')
    expect(featureAvailable(caps, 'brouter')).toBe(true)
  })

  it('supports old boolean-only capability responses', () => {
    const caps: Capabilities = { offline_map: true, badrouter: false }

    expect(featureConfigured(caps, 'offline_map')).toBe(true)
    expect(featureAvailable(caps, 'offline_map')).toBe(true)
    expect(legacyCapability(caps, 'offline_map')).toBe(true)
    expect(featureConfigured(caps, 'badrouter')).toBe(false)
    expect(featureAvailable(caps, 'badrouter')).toBe(false)
  })

  it('does not treat configured-but-unavailable as legacy available', () => {
    const caps: Capabilities = {
      features: {
        coverage: { configured: true, available: false, reason: 'missing_artifacts' },
      },
      coverage: false,
    }

    expect(featureConfigured(caps, 'coverage')).toBe(true)
    expect(featureAvailable(caps, 'coverage')).toBe(false)
    expect(legacyCapability(caps, 'coverage')).toBe(false)
  })

  it('treats missing keys as unavailable', () => {
    expect(featureConfigured({}, 'route_suggestion')).toBe(false)
    expect(featureAvailable({}, 'route_suggestion')).toBe(false)
    expect(featureReason({}, 'route_suggestion')).toBeUndefined()
  })

  it('validates canonical snake_case capability keys', () => {
    expect(validateCapabilityKey('route_suggestion')).toBe(true)
    expect(validateCapabilityKey('route-suggestion')).toBe(false)
    expect(validateCapabilityKey('RouteSuggestion')).toBe(false)
  })
})

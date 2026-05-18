import { describe, expect, it } from 'vitest'
import { frontendBuildInfo } from './buildInfo'

describe('frontendBuildInfo', () => {
  it('uses unknown defaults when build env is absent', () => {
    expect(frontendBuildInfo({} as ImportMetaEnv)).toMatchObject({
      git_commit: 'unknown',
      commit_time: 'unknown',
      build_time: 'unknown',
      config_version: '1',
    })
  })

  it('returns injected build values', () => {
    expect(frontendBuildInfo({
      VITE_SKADIA_GIT_COMMIT: 'abc123',
      VITE_SKADIA_GIT_COMMIT_TIME: '2026-05-17T09:58:00Z',
      VITE_SKADIA_BUILD_TIME: '2026-05-17T10:00:00Z',
      VITE_SKADIA_CONFIG_VERSION: '2',
    } as unknown as ImportMetaEnv)).toMatchObject({
      git_commit: 'abc123',
      commit_time: '2026-05-17T09:58:00Z',
      build_time: '2026-05-17T10:00:00Z',
      config_version: '2',
    })
  })
})

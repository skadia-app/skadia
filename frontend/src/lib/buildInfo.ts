export interface FrontendBuildInfo {
  service: 'frontend'
  git_commit: string
  commit_time: string
  build_time: string
  config_version: string
}

export function frontendBuildInfo(env: ImportMetaEnv = import.meta.env): FrontendBuildInfo {
  return {
    service: 'frontend',
    git_commit: env.VITE_SKADIA_GIT_COMMIT || 'unknown',
    commit_time: env.VITE_SKADIA_GIT_COMMIT_TIME || 'unknown',
    build_time: env.VITE_SKADIA_BUILD_TIME || 'unknown',
    config_version: env.VITE_SKADIA_CONFIG_VERSION || '1',
  }
}

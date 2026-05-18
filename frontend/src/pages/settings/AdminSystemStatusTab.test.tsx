import '@testing-library/jest-dom/vitest'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import AdminSystemStatusTab from './AdminSystemStatusTab'

vi.mock('@/api/systemStatus', () => ({
  fetchSystemStatus: vi.fn(async () => ({
    schema_version: 1,
    generated_at: '2026-05-17T10:00:00Z',
    overall: 'degraded',
    build: { api: { service: 'api', git_commit: 'abc123', commit_time: '2026-05-17T09:58:00Z', build_time: '2026-05-17T10:00:00Z' } },
    components: {
      db: { key: 'db', label: 'Database', state: 'degraded', reason_code: 'migration_dirty', checked_at: '2026-05-17T10:00:00Z' },
      timescale_compression: { key: 'timescale_compression', label: 'TimescaleDB compression', state: 'ok', checked_at: '2026-05-17T10:00:00Z' },
      storage: { key: 'storage', label: 'Storage', state: 'ok', checked_at: '2026-05-17T10:00:00Z' },
      jobs: { key: 'jobs', label: 'Jobs', state: 'ok', checked_at: '2026-05-17T10:00:00Z' },
      gdpr_artifacts: { key: 'gdpr_artifacts', label: 'GDPR artifacts', state: 'ok', checked_at: '2026-05-17T10:00:00Z' },
    },
  })),
  downloadSystemStatusExport: vi.fn(),
}))

vi.mock('@/lib/buildInfo', () => ({
  frontendBuildInfo: () => ({
    service: 'frontend',
    git_commit: 'front123',
    commit_time: '2026-05-17T09:59:00Z',
    build_time: '2026-05-17T10:01:00Z',
    config_version: '1',
  }),
}))

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('AdminSystemStatusTab', () => {
  it('renders overall system status', async () => {
    render(<AdminSystemStatusTab />, { wrapper })
    expect(await screen.findByText(/Gesamtstatus: degraded/i)).toBeInTheDocument()
    expect(screen.getByText(/Frontend-Build: front123.*Commit-Zeit: 2026-05-17T09:59:00Z/i)).toBeInTheDocument()
    expect(screen.getByText('Datenbank')).toBeInTheDocument()
    expect(screen.getByText('DSGVO-Artefakte')).toBeInTheDocument()
  })
})

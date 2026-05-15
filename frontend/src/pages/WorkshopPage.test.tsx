import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import WorkshopPage from './WorkshopPage'

const mocks = vi.hoisted(() => ({
  capabilities: {
    features: {
      document_ml: { configured: false, available: false, reason: 'core_profile' },
      geo_poi_enrichment: { configured: false, available: false, reason: 'core_profile' },
    },
  } as Record<string, unknown>,
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

function renderWorkshop() {
  return render(
    <MemoryRouter>
      <WorkshopPage />
    </MemoryRouter>,
  )
}

describe('WorkshopPage deployment capabilities', () => {
  beforeEach(() => {
    mocks.capabilities = {
      features: {
        document_ml: { configured: false, available: false, reason: 'core_profile' },
        geo_poi_enrichment: { configured: false, available: false, reason: 'core_profile' },
      },
    }
  })

  it('keeps core-safe tiles, disables document review, and hides unconfigured summit collection in Core', () => {
    renderWorkshop()

    expect(screen.getByText('Physiologisches Profil')).toBeInTheDocument()
    expect(screen.getByText('Ausrüstung')).toBeInTheDocument()
    expect(screen.getByText('Neu verarbeiten')).toBeInTheDocument()
    expect(screen.getByText('Fusionskandidaten')).toBeInTheDocument()
    expect(screen.getByText('Zu prüfen')).toBeInTheDocument()
    expect(screen.getByText('Dokument-Überprüfung').closest('[aria-disabled="true"]')).not.toBeNull()
    expect(screen.queryByText('Gipfelsammlung')).not.toBeInTheDocument()
  })

  it('shows summit collection disabled when geodata is configured but unavailable', () => {
    mocks.capabilities = {
      features: {
        document_ml: { configured: false, available: false, reason: 'core_profile' },
        geo_poi_enrichment: { configured: true, available: false, reason: 'missing_artifacts' },
      },
    }

    renderWorkshop()

    expect(screen.getByText('Gipfelsammlung').closest('[aria-disabled="true"]')).not.toBeNull()
  })

  it('shows full navigation when geodata and document capabilities are available', () => {
    mocks.capabilities = {
      features: {
        document_ml: { configured: true, available: true },
        geo_poi_enrichment: { configured: true, available: true },
      },
    }

    renderWorkshop()

    expect(screen.getByRole('link', { name: /Dokument-Überprüfung/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Gipfelsammlung/i })).toBeInTheDocument()
  })
})

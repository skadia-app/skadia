import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import '@testing-library/jest-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import VisibilityAccessField from './VisibilityAccessField'

const mocks = vi.hoisted(() => ({
  createObjectAccessScope: vi.fn(),
  listObjectAccessScopes: vi.fn(),
}))

vi.mock('@/api/accessScopes', async () => {
  const actual = await vi.importActual<typeof import('@/api/accessScopes')>('@/api/accessScopes')
  return {
    ...actual,
    createObjectAccessScope: mocks.createObjectAccessScope,
    listObjectAccessScopes: mocks.listObjectAccessScopes,
  }
})

function renderField(onChange = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <VisibilityAccessField visibility="shared" accessScopeId={null} onChange={onChange} />
    </QueryClientProvider>,
  )
  return { onChange }
}

describe('VisibilityAccessField', () => {
  beforeEach(() => {
    mocks.createObjectAccessScope.mockReset()
    mocks.listObjectAccessScopes.mockReset()
    mocks.listObjectAccessScopes.mockResolvedValue({ scopes: [] })
  })

  it('offers only backend-supported principal kinds for new scopes', async () => {
    renderField()

    const principalKind = screen.getByLabelText('Art')
    expect(within(principalKind).getByRole('option', { name: 'Nutzer' })).toBeInTheDocument()
    expect(within(principalKind).getByRole('option', { name: 'Link' })).toBeInTheDocument()
    expect(within(principalKind).queryByRole('option', { name: 'Gruppe' })).not.toBeInTheDocument()
    expect(within(principalKind).queryByRole('option', { name: 'Team' })).not.toBeInTheDocument()
    expect(within(principalKind).queryByRole('option', { name: 'Organisation' })).not.toBeInTheDocument()
    expect(within(principalKind).queryByRole('option', { name: 'Rolle' })).not.toBeInTheDocument()
  })

  it('creates user grants with principal_user_id', async () => {
    const { onChange } = renderField()
    mocks.createObjectAccessScope.mockResolvedValue({
      scope: { id: 'scope-1' },
    })

    fireEvent.change(screen.getByLabelText('Neue Freigabe'), { target: { value: 'Freunde' } })
    fireEvent.change(screen.getByLabelText('Adressat'), { target: { value: 'user-2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Anlegen' }))

    await waitFor(() => {
      expect(mocks.createObjectAccessScope).toHaveBeenCalledWith({
        scope_kind: 'manual',
        label: 'Freunde',
        grants: [{ principal_kind: 'user', principal_user_id: 'user-2' }],
      })
    })
    await waitFor(() => expect(onChange).toHaveBeenCalledWith({ visibility: 'shared', access_scope_id: 'scope-1' }))
  })

  it('creates link grants without an address and shows the generated token once', async () => {
    renderField()
    mocks.createObjectAccessScope.mockResolvedValue({
      scope: { id: 'scope-link' },
      link_tokens: [{ grant_id: 'grant-1', token: 'link-token-raw' }],
    })

    fireEvent.change(screen.getByLabelText('Art'), { target: { value: 'link_token' } })
    expect(screen.queryByLabelText('Adressat')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Anlegen' })).toBeEnabled()

    fireEvent.click(screen.getByRole('button', { name: 'Anlegen' }))

    await waitFor(() => {
      expect(mocks.createObjectAccessScope).toHaveBeenCalledWith({
        scope_kind: 'manual',
        label: undefined,
        grants: [{ principal_kind: 'link_token' }],
      })
    })
    expect(await screen.findByText('link-token-raw')).toBeInTheDocument()
  })

  it('does not expose global scope revocation from an inline visibility field', () => {
    renderField()

    expect(screen.queryByRole('button', { name: /deaktivieren/i })).not.toBeInTheDocument()
  })
})

import '@testing-library/jest-dom/vitest'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import AdminSystemTab from './AdminSystemTab'

vi.mock('@/api/adminSettings', () => ({
  getAdminSettings: vi.fn(async () => ({
    smtp: {
      host: { value: '', source: 'default' },
      port: { value: '587', source: 'default' },
      username: { value: '', source: 'default' },
      password: { value: '', source: 'default', has_value: false },
      from: { value: '', source: 'default' },
      tls: { value: 'starttls', source: 'default' },
    },
  })),
  getEmailTemplates: vi.fn(async () => [
    {
      key: 'future_template',
      subject: 'Subject',
      html: '<p>Body</p>',
      text: 'Body',
      enabled: true,
      source: 'default',
    },
  ]),
  previewEmailTemplate: vi.fn(),
  putAdminSettings: vi.fn(),
  resetEmailTemplate: vi.fn(),
  saveEmailTemplate: vi.fn(),
  sendEmailTemplateTest: vi.fn(),
  sendTestEmail: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('AdminSystemTab', () => {
  it('uses an unknown label for unrecognized email template keys', async () => {
    render(<AdminSystemTab />, { wrapper })

    expect(await screen.findByText('Unbekannte Vorlage')).toBeInTheDocument()
    expect(screen.queryByText('future_template')).not.toBeInTheDocument()
  })
})

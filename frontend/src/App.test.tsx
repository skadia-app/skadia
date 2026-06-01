import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { afterEach, describe, expect, it } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { LoadingSpinner } from './App'

describe('App shell loading state', () => {
  afterEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('uses the app language for the route loading indicator', async () => {
    await i18n.changeLanguage('en')

    render(<LoadingSpinner />)

    expect(screen.getByRole('status', { name: 'Loading...' })).toBeInTheDocument()
  })
})

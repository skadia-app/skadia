import { fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import SportTuplePicker from '../SportTuplePicker'
import { i18n } from '@/i18n/i18n'
import { sportDetailLabel } from '@/lib/sportTaxonomy'

describe('SportTuplePicker', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('de')
  })

  it('filters tuple options and returns sport plus sub-sport', () => {
    const onChange = vi.fn()
    render(
      <I18nextProvider i18n={i18n}>
        <SportTuplePicker sportType="cycling" subType="mountain" onChange={onChange} />
      </I18nextProvider>,
    )

    expect(screen.getByRole('button', { name: sportDetailLabel('cycling', 'mountain') })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: sportDetailLabel('cycling', 'mountain') }))
    fireEvent.change(screen.getByPlaceholderText('Sportart suchen...'), { target: { value: 'trail' } })
    fireEvent.click(screen.getByRole('button', { name: sportDetailLabel('running', 'trail') }))

    expect(onChange).toHaveBeenCalledWith('running', 'trail')
    expect(screen.queryByPlaceholderText('Sportart suchen...')).not.toBeInTheDocument()
  })
})

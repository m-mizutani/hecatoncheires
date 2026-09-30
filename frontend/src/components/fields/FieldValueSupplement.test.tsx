import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import { en } from '../../i18n/en'
import FieldValueSupplement from './FieldValueSupplement'

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

const URL = 'https://slack.com/archives/C0123ABCD'

describe('FieldValueSupplement', () => {
  it('links the label to the url in a new tab', () => {
    renderWithI18n(<FieldValueSupplement display={{ label: '#general', url: URL }} />)
    const link = screen.getByRole('link')
    expect(link).toHaveTextContent('#general')
    expect(link).toHaveAttribute('href', URL)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noreferrer noopener')
  })

  it('shows the label as text when there is no url', () => {
    renderWithI18n(<FieldValueSupplement display={{ label: '#general', url: null }} />)
    expect(screen.getByTestId('field-value-supplement')).toHaveTextContent('#general')
    expect(screen.queryByRole('link')).toBeNull()
  })

  it('shows the generic open link when only the url is known', () => {
    renderWithI18n(<FieldValueSupplement display={{ label: null, url: URL }} />)
    const link = screen.getByRole('link')
    expect(link).toHaveTextContent(en.fieldValueOpenLink)
    expect(link).toHaveAttribute('href', URL)
  })

  it('renders nothing without a display', () => {
    const { container } = renderWithI18n(<FieldValueSupplement display={null} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when both parts are empty', () => {
    const { container } = renderWithI18n(<FieldValueSupplement display={{ label: '', url: '' }} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('does not let a click on the link reach the parent', () => {
    const parentClick = vi.fn()
    renderWithI18n(
      <div onClick={parentClick}>
        <FieldValueSupplement display={{ label: '#general', url: URL }} />
      </div>,
    )
    fireEvent.click(screen.getByRole('link'))
    expect(parentClick).not.toHaveBeenCalled()
  })
})

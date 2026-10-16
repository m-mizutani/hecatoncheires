import { afterEach, describe, it, expect, vi } from 'vitest'
import { cleanup, render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import CaseMarkdownFieldSection from './CaseMarkdownFieldSection'
import type { CustomFieldDefinition } from './CustomFieldHelpRow'

const FIELD: CustomFieldDefinition = {
  id: 'runbook',
  name: 'Runbook',
  type: 'MARKDOWN',
  description: 'Step-by-step notes.',
}

function renderSection(props: Partial<React.ComponentProps<typeof CaseMarkdownFieldSection>> = {}) {
  const onSave = props.onSave ?? vi.fn()
  render(
    <I18nProvider defaultLang="en">
      <CaseMarkdownFieldSection
        field={FIELD}
        value={'## Steps\n\n1. first'}
        onSave={onSave}
        testId="case-field-runbook"
        {...props}
      />
    </I18nProvider>,
  )
  return { onSave }
}

afterEach(() => {
  cleanup()
})

describe('CaseMarkdownFieldSection', () => {
  it('renders the field name as the section title and the value as Markdown', () => {
    renderSection()
    const section = screen.getByTestId('case-field-runbook')
    expect(section).toHaveTextContent('Runbook')
    expect(screen.getByRole('heading', { level: 2, name: 'Steps' })).toBeInTheDocument()
  })

  it('shows a placeholder when the value is empty or not a string', () => {
    renderSection({ value: undefined })
    expect(screen.getByTestId('field-runbook')).toHaveTextContent('Add value...')
  })

  it('saves the edited Markdown through onSave', async () => {
    const { onSave } = renderSection({ value: 'old' })
    fireEvent.click(screen.getByTestId('field-runbook'))
    fireEvent.change(screen.getByTestId('field-runbook-input'), { target: { value: '# new' } })
    fireEvent.click(screen.getByTestId('field-runbook-save'))
    await waitFor(() => {
      expect(onSave).toHaveBeenCalledWith('# new')
    })
  })

  it('does not enter edit mode when disabled', () => {
    renderSection({ disabled: true })
    fireEvent.click(screen.getByTestId('field-runbook'))
    expect(screen.queryByTestId('field-runbook-input')).toBeNull()
  })

  it('offers the help toggle only when the field has a description', () => {
    renderSection()
    expect(screen.getByTestId('case-field-runbook-help')).toBeInTheDocument()
    cleanup()
    renderSection({ field: { ...FIELD, description: null } })
    expect(screen.queryByTestId('case-field-runbook-help')).toBeNull()
  })
})

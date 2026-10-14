import { useRef, useState } from 'react'
import InlineLongText from '../inline/InlineLongText'
import FieldHelpButton from './FieldHelpButton'
import FieldCatalogPopover from './FieldCatalogPopover'
import type { CustomFieldDefinition } from './CustomFieldHelpRow'
import { useTranslation } from '../../i18n'

interface Props {
  field: CustomFieldDefinition
  value: unknown
  disabled?: boolean
  onSave: (next: string) => Promise<void> | void
  testId?: string
}

// Main-column section for a MARKDOWN custom field on the case detail page.
// Markdown fields tend to hold long text that is hard to read in the narrow
// sidebar, so they are laid out like the Description: a section headed by the
// field name, with the value rendered as Markdown and edited in place.
export default function CaseMarkdownFieldSection({
  field, value, disabled, onSave, testId,
}: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const titleRef = useRef<HTMLSpanElement>(null)

  return (
    <section className="h-section h-detail-body" data-testid={testId}>
      <div className="h-section-h">
        <span
          className="h-section-title"
          ref={titleRef}
          style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}
        >
          <span>{field.name}</span>
          {field.description && (
            <FieldHelpButton
              ariaLabel={t('fieldHelpAriaToggle', { name: field.name })}
              expanded={open}
              onToggle={() => setOpen((v) => !v)}
              testId={testId ? `${testId}-help` : undefined}
            />
          )}
        </span>
      </div>
      <InlineLongText
        value={typeof value === 'string' ? value : ''}
        onSave={onSave}
        ariaLabel={field.name}
        placeholder={t('placeholderAddValue')}
        disabled={disabled}
        testId={`field-${field.id}`}
        renderMarkdown
      />
      {field.description && (
        <FieldCatalogPopover
          field={field}
          value={value}
          anchor={titleRef.current}
          open={open}
          onClose={() => setOpen(false)}
          testId={testId ? `${testId}-catalog` : undefined}
        />
      )}
    </section>
  )
}

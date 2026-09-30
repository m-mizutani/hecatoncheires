import type { MouseEvent } from 'react'
import { IconExt } from '../Icons'
import { useTranslation } from '../../i18n'
import styles from './FieldValueSupplement.module.css'

// What a field value refers to, as resolved by the server from the field's
// semantic (FieldValue.display). Either part may be null.
export interface FieldValueDisplay {
  label?: string | null
  url?: string | null
}

interface Props {
  display?: FieldValueDisplay | null
  testId?: string
}

// Clicks on the link must not reach the parent: the field value above it
// enters edit mode on click, and a Case list row navigates on click.
const stop = (e: MouseEvent) => e.stopPropagation()

// Supplementary line rendered directly under a field value. It never replaces
// the value; it only adds what the value refers to. It knows nothing about
// individual semantics: the server decides the label and the link.
export default function FieldValueSupplement({ display, testId = 'field-value-supplement' }: Props) {
  const { t } = useTranslation()
  const label = display?.label || null
  const url = display?.url || null
  if (!label && !url) return null

  if (!url) {
    return <span className={styles.supplement} data-testid={testId}>{label}</span>
  }

  return (
    <span className={styles.supplement} data-testid={testId}>
      <a
        className={styles.link}
        href={url}
        target="_blank"
        rel="noreferrer noopener"
        onClick={stop}
        onMouseDown={stop}
      >
        {label ?? t('fieldValueOpenLink')}
        <IconExt size={10} className={styles.icon} aria-hidden="true" />
      </a>
    </span>
  )
}

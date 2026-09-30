import type { MouseEvent } from 'react'
import { IconExt } from '../Icons'
import { useTranslation } from '../../i18n'
import styles from './FieldValueSupplement.module.css'

// What a field value refers to, as resolved by the server from the field's
// semantic (FieldValue.display). A null label means the name could not be
// resolved.
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
// the value; it only adds the resolved name (e.g. "#general"), or says the
// name could not be resolved. It knows nothing about individual semantics:
// the server decides the label and the link.
export default function FieldValueSupplement({ display, testId = 'field-value-supplement' }: Props) {
  const { t } = useTranslation()
  if (!display) return null

  const label = display.label || null
  const url = display.url || null

  if (!label) {
    return (
      <span className={`${styles.supplement} ${styles.error}`} role="status" data-testid={testId}>
        {t('fieldValueUnresolved')}
      </span>
    )
  }

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
        {label}
        <IconExt size={10} className={styles.icon} aria-hidden="true" />
      </a>
    </span>
  )
}

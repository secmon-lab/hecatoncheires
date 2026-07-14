import { useState } from 'react'
import MarkdownContent from './MarkdownContent'
import { useTranslation } from '../../i18n'
import styles from './MarkdownEditor.module.css'

interface Props {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  testId?: string
}

// Controlled Write / Preview editor for Markdown text. The Write tab is a
// multiline textarea over the raw Markdown source; the Preview tab renders it
// via the shared MarkdownContent. It holds only the active-tab state — the
// value lives with the caller. It wires no Enter side effect: the textarea's
// Enter inserts a newline, and any save shortcut is the caller's concern
// (attached at the container level so it survives the textarea unmounting
// while the Preview tab is shown).
export default function MarkdownEditor({ value, onChange, disabled, testId }: Props) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<'write' | 'preview'>('write')
  const previewIsEmpty = value.trim() === ''

  return (
    <div className={styles.editor} data-testid={testId}>
      <div className={styles.tabs} role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'write'}
          className={activeTab === 'write' ? `${styles.tab} ${styles.tabActive}` : styles.tab}
          onClick={() => setActiveTab('write')}
          data-testid={testId ? `${testId}-tab-write` : undefined}
        >
          {t('tabWrite')}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'preview'}
          className={activeTab === 'preview' ? `${styles.tab} ${styles.tabActive}` : styles.tab}
          onClick={() => setActiveTab('preview')}
          data-testid={testId ? `${testId}-tab-preview` : undefined}
        >
          {t('labelPreview')}
        </button>
      </div>

      {activeTab === 'write' ? (
        <textarea
          className={styles.textarea}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          data-testid={testId ? `${testId}-textarea` : undefined}
        />
      ) : (
        <div
          className={styles.preview}
          role="tabpanel"
          aria-label={t('labelPreview')}
          data-testid={testId ? `${testId}-preview` : undefined}
        >
          {previewIsEmpty ? (
            <span className={styles.previewEmpty}>{t('labelPreviewEmpty')}</span>
          ) : (
            <MarkdownContent source={value} />
          )}
        </div>
      )}
    </div>
  )
}

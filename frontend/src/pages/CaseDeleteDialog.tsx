import { useState } from 'react'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { useTranslation } from '../i18n'
import { IconWarn } from '../components/Icons'

interface CaseDeleteDialogProps {
  caseTitle: string
  caseLabel: string
  deleting: boolean
  onCancel: () => void
  onConfirm: () => void
}

export default function CaseDeleteDialog({
  caseTitle,
  deleting,
  onCancel,
  onConfirm,
}: CaseDeleteDialogProps) {
  const { t } = useTranslation()
  const [confirmText, setConfirmText] = useState('')
  const matches = confirmText.trim() === caseTitle.trim()

  return (
    <Modal
      open
      onClose={onCancel}
      title={
        <div className="row" style={{ gap: 10 }}>
          <span style={{
            width: 28, height: 28, borderRadius: 8,
            background: 'color-mix(in oklch, var(--danger) 14%, transparent)',
            color: 'var(--danger)',
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
          }}>
            <IconWarn size={14} sw={2} />
          </span>
          <h3>{t('titleDeleteCase')}</h3>
        </div>
      }
      width={460}
      footer={
        <>
          <Button variant="ghost" onClick={onCancel}>{t('btnCancel')}</Button>
          <Button
            variant="danger"
            onClick={onConfirm}
            disabled={!matches || deleting}
            data-testid="confirm-delete-case-button"
          >
            {deleting ? t('btnDeleting') : t('btnDelete')}
          </Button>
        </>
      }
    >
      <p
        style={{ fontSize: 13, lineHeight: 1.6, margin: 0 }}
        dangerouslySetInnerHTML={{ __html: t('msgDeleteCaseConfirm', { title: caseTitle }) }}
      />
      <p className="muted" style={{ marginTop: 8, fontSize: 12.5 }}>
        {t('warningDeleteCasePermanent')}
      </p>
      <div style={{ marginTop: 12 }}>
        <div className="field-label">{t('labelTitle')}</div>
        <input
          className="input"
          placeholder={caseTitle}
          value={confirmText}
          onChange={(e) => setConfirmText(e.target.value)}
        />
      </div>
    </Modal>
  )
}

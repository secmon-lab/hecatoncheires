import Modal from '../Modal'
import Button from '../Button'
import { IconWarn, IconCheck } from '../Icons'
import { useTranslation } from '../../i18n'

interface Props {
  memoTitle: string
  onConfirm: () => void
  onCancel: () => void
  archiving: boolean
}

export default function MemoArchiveDialog({ memoTitle: _memoTitle, onConfirm, onCancel, archiving }: Props) {
  const { t } = useTranslation()

  return (
    <Modal
      open
      onClose={onCancel}
      title={t('memoTitleArchiveDialog')}
      width={460}
      footer={
        <>
          <Button variant="ghost" onClick={onCancel} disabled={archiving}>
            {t('btnCancel')}
          </Button>
          <Button variant="danger" onClick={onConfirm} disabled={archiving} icon={<IconCheck size={13} />}>
            {t('memoArchiveDialogConfirm')}
          </Button>
        </>
      }
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-4)' }}>
        <div style={{ display: 'flex', gap: 'var(--sp-3)', alignItems: 'flex-start' }}>
          <IconWarn size={18} style={{ color: 'var(--warn)', flexShrink: 0, marginTop: 1 }} />
          <p style={{ margin: 0, fontSize: 13, lineHeight: 1.6 }}>
            {t('memoArchiveDialogBody')}
          </p>
        </div>
        <div
          style={{
            background: 'var(--bg-sunken)',
            border: '1px solid var(--border-light)',
            borderRadius: '0.375rem',
            padding: 'var(--sp-3) var(--sp-4)',
            fontSize: 12,
            lineHeight: 1.6,
            color: 'var(--fg-muted)',
          }}
        >
          {t('memoArchiveDialogReassurance')}
        </div>
      </div>
    </Modal>
  )
}

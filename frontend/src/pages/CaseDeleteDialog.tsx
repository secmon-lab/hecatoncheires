import { useMutation } from '@apollo/client'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { AlertTriangle } from 'lucide-react'
import { DELETE_CASE, GET_CASES } from '../graphql/case'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import styles from './CaseDeleteDialog.module.css'

interface CaseDeleteDialogProps {
  isOpen: boolean
  onClose: () => void
  onConfirm: () => void
  caseTitle: string
  caseId?: number
}

export default function CaseDeleteDialog({
  isOpen,
  onClose,
  onConfirm,
  caseTitle,
  caseId,
}: CaseDeleteDialogProps) {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const [deleteCase, { loading }] = useMutation(DELETE_CASE, {
    update(cache) {
      if (!caseId) return
      for (const status of ['OPEN', 'CLOSED']) {
        const existingCases = cache.readQuery<{ cases: any[] }>({
          query: GET_CASES,
          variables: { workspaceId: currentWorkspace!.id, status },
        })
        if (existingCases) {
          cache.writeQuery({
            query: GET_CASES,
            variables: { workspaceId: currentWorkspace!.id, status },
            data: {
              cases: existingCases.cases.filter((c) => c.id !== caseId),
            },
          })
        }
      }
    },
    onCompleted: () => {
      onConfirm()
    },
    onError: (error) => {
      console.error('Delete error:', error)
    },
  })

  const handleDelete = async () => {
    if (caseId) {
      await deleteCase({
        variables: { workspaceId: currentWorkspace!.id, id: caseId },
      })
    } else {
      onConfirm()
    }
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('titleDeleteCase')}
      footer={
        <>
          <Button variant="outline" onClick={onClose} disabled={loading}>
            {t('btnCancel')}
          </Button>
          <Button variant="danger" onClick={handleDelete} disabled={loading}>
            {loading ? t('btnDeleting') : t('btnDelete')}
          </Button>
        </>
      }
    >
      <div className={styles.content}>
        <div className={styles.iconContainer}>
          <AlertTriangle size={48} className={styles.icon} />
        </div>
        <p
          className={styles.message}
          dangerouslySetInnerHTML={{ __html: t('msgDeleteCaseConfirm', { title: caseTitle }) }}
        />
        <p className={styles.warning}>
          {t('warningDeleteCasePermanent')}
        </p>
      </div>
    </Modal>
  )
}

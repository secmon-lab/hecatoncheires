import { useMutation } from '@apollo/client'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { AlertTriangle } from 'lucide-react'
import { DELETE_CASE, GET_CASES } from '../graphql/case'
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
  const [deleteCase, { loading }] = useMutation(DELETE_CASE, {
    update(cache) {
      if (!caseId) return
      const existingCases = cache.readQuery<{ cases: any[] }>({ query: GET_CASES })
      if (existingCases) {
        cache.writeQuery({
          query: GET_CASES,
          data: {
            cases: existingCases.cases.filter((c) => c.id !== caseId),
          },
        })
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
        variables: { id: caseId },
      })
    } else {
      onConfirm()
    }
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Delete Case"
      footer={
        <>
          <Button variant="outline" onClick={onClose} disabled={loading}>
            Cancel
          </Button>
          <Button variant="danger" onClick={handleDelete} disabled={loading}>
            {loading ? 'Deleting...' : 'Delete'}
          </Button>
        </>
      }
    >
      <div className={styles.content}>
        <div className={styles.iconContainer}>
          <AlertTriangle size={48} className={styles.icon} />
        </div>
        <p className={styles.message}>
          Are you sure you want to delete <strong>{caseTitle}</strong>?
        </p>
        <p className={styles.warning}>
          This action cannot be undone. All related actions and data will also be deleted.
        </p>
      </div>
    </Modal>
  )
}

import { useMutation } from '@apollo/client'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { AlertTriangle } from 'lucide-react'
import { DELETE_ACTION, GET_ACTIONS } from '../graphql/action'
import styles from './ActionDeleteDialog.module.css'

interface ActionDeleteDialogProps {
  isOpen: boolean
  onClose: () => void
  onConfirm: () => void
  actionTitle: string
  actionId?: number
}

export default function ActionDeleteDialog({
  isOpen,
  onClose,
  onConfirm,
  actionTitle,
  actionId,
}: ActionDeleteDialogProps) {
  const [deleteAction, { loading }] = useMutation(DELETE_ACTION, {
    update(cache) {
      if (!actionId) return
      const existingActions = cache.readQuery<{ actions: any[] }>({ query: GET_ACTIONS })
      if (existingActions) {
        cache.writeQuery({
          query: GET_ACTIONS,
          data: {
            actions: existingActions.actions.filter((a) => a.id !== actionId),
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
    if (actionId) {
      await deleteAction({
        variables: { id: actionId },
      })
    } else {
      onConfirm()
    }
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Delete Action"
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
          Are you sure you want to delete <strong>{actionTitle}</strong>?
        </p>
        <p className={styles.warning}>
          This action cannot be undone.
        </p>
      </div>
    </Modal>
  )
}

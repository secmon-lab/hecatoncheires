import { useMutation } from '@apollo/client'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { AlertTriangle } from 'lucide-react'
import { DELETE_RESPONSE, GET_RESPONSES } from '../graphql/response'
import styles from './ResponseDeleteDialog.module.css'

interface Response {
  id: number
  title: string
}

interface ResponseDeleteDialogProps {
  response: Response
  onClose: () => void
  onConfirm: () => void
}

export default function ResponseDeleteDialog({
  response,
  onClose,
  onConfirm,
}: ResponseDeleteDialogProps) {
  const [deleteResponse, { loading }] = useMutation(DELETE_RESPONSE, {
    update(cache) {
      const existingResponses = cache.readQuery<{ responses: Response[] }>({
        query: GET_RESPONSES,
      })
      if (existingResponses) {
        cache.writeQuery({
          query: GET_RESPONSES,
          data: {
            responses: existingResponses.responses.filter((r) => r.id !== response.id),
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
    await deleteResponse({
      variables: { id: response.id },
    })
  }

  return (
    <Modal
      isOpen={true}
      onClose={onClose}
      title="Delete Response"
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
          Are you sure you want to delete <strong>{response.title}</strong>?
        </p>
        <p className={styles.warning}>This action cannot be undone.</p>
      </div>
    </Modal>
  )
}

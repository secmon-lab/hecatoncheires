import { useMutation } from '@apollo/client'
import { AlertTriangle } from 'lucide-react'
import Modal from '../Modal'
import Button from '../Button'
import { DELETE_SOURCE, GET_SOURCES } from '../../graphql/source'
import styles from './source.module.css'

interface SourceDeleteDialogProps {
  isOpen: boolean
  onClose: () => void
  onConfirm: () => void
  sourceId: string
  sourceName: string
}

export default function SourceDeleteDialog({
  isOpen,
  onClose,
  onConfirm,
  sourceId,
  sourceName,
}: SourceDeleteDialogProps) {
  const [deleteSource, { loading }] = useMutation(DELETE_SOURCE, {
    update(cache) {
      const existingData = cache.readQuery<{ sources: { id: string }[] }>({ query: GET_SOURCES })
      if (existingData) {
        cache.writeQuery({
          query: GET_SOURCES,
          data: {
            sources: existingData.sources.filter((s) => s.id !== sourceId),
          },
        })
      }
    },
    onCompleted: () => {
      onConfirm()
    },
    onError: (error) => {
      console.error('Delete source error:', error)
    },
  })

  const handleDelete = async () => {
    await deleteSource({
      variables: { id: sourceId },
    })
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Delete Source"
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
      <div className={styles.deleteDialogContent}>
        <div className={styles.deleteDialogWarning}>
          <AlertTriangle size={24} />
          <div>
            <p className={styles.deleteDialogMessage}>
              Are you sure you want to delete <strong>{sourceName}</strong>?
            </p>
            <p className={styles.deleteDialogMessage}>
              This action cannot be undone. All data associated with this source will be permanently removed.
            </p>
          </div>
        </div>
      </div>
    </Modal>
  )
}

import { useMutation } from '@apollo/client'
import { AlertTriangle } from 'lucide-react'
import { useWorkspace } from '../../contexts/workspace-context'
import { useTranslation } from '../../i18n'
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
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const [deleteSource, { loading }] = useMutation(DELETE_SOURCE, {
    update(cache) {
      const existingData = cache.readQuery<{ sources: { id: string }[] }>({ query: GET_SOURCES, variables: { workspaceId: currentWorkspace!.id } })
      if (existingData) {
        cache.writeQuery({
          query: GET_SOURCES,
          variables: { workspaceId: currentWorkspace!.id },
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
      variables: { workspaceId: currentWorkspace!.id, id: sourceId },
    })
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('titleDeleteSource')}
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
      <div className={styles.deleteDialogContent}>
        <div className={styles.deleteDialogWarning}>
          <AlertTriangle size={24} />
          <div>
            <p className={styles.deleteDialogMessage} dangerouslySetInnerHTML={{ __html: t('msgDeleteSourceConfirm', { name: sourceName }) }} />
            <p className={styles.deleteDialogMessage}>
              {t('warningDeleteSourcePermanent')}
            </p>
          </div>
        </div>
      </div>
    </Modal>
  )
}

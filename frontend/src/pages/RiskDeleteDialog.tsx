import Modal from '../components/Modal'
import Button from '../components/Button'
import { AlertTriangle } from 'lucide-react'
import styles from './RiskDeleteDialog.module.css'

interface RiskDeleteDialogProps {
  isOpen: boolean
  onClose: () => void
  onConfirm: () => void
  riskName: string
}

export default function RiskDeleteDialog({
  isOpen,
  onClose,
  onConfirm,
  riskName,
}: RiskDeleteDialogProps) {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Delete Risk"
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="danger" onClick={onConfirm}>
            Delete
          </Button>
        </>
      }
    >
      <div className={styles.content}>
        <div className={styles.iconContainer}>
          <AlertTriangle size={48} className={styles.icon} />
        </div>
        <p className={styles.message}>
          Are you sure you want to delete <strong>{riskName}</strong>?
        </p>
        <p className={styles.warning}>
          This action cannot be undone.
        </p>
      </div>
    </Modal>
  )
}

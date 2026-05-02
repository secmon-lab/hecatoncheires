import { useEffect, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { IconX } from './Icons'
import { useTranslation } from '../i18n'

interface ModalProps {
  /** Either `open` or `isOpen` controls visibility. */
  open?: boolean
  isOpen?: boolean
  onClose: () => void
  title?: ReactNode
  width?: number
  footer?: ReactNode
  children: ReactNode
}

export default function Modal({ open, isOpen, onClose, title, width = 540, footer, children }: ModalProps) {
  const visible = open ?? isOpen ?? false
  const { t } = useTranslation()

  useEffect(() => {
    if (!visible) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = ''
    }
  }, [visible, onClose])

  if (!visible) return null

  return createPortal(
    <div
      className="modal-stage modal-backdrop"
      role="dialog"
      aria-modal="true"
      style={{ position: 'fixed', inset: 0, zIndex: 1000 }}
      onClick={onClose}
    >
      <div
        className="modal-card"
        style={{ width }}
        role="document"
        aria-labelledby="modal-title"
        onClick={(e) => e.stopPropagation()}
      >
        {title && (
          <div className="modal-h">
            {typeof title === 'string' ? (
              <h2 id="modal-title" style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>{title}</h2>
            ) : (
              title
            )}
            <span className="spacer" />
            <button
              type="button"
              className="h-icon-btn"
              onClick={onClose}
              aria-label={t('btnClose') || 'Close'}
              title={t('btnClose') || 'Close'}
            >
              <IconX size={15} />
            </button>
          </div>
        )}
        <div className="modal-b">{children}</div>
        {footer && <div className="modal-f">{footer}</div>}
      </div>
    </div>,
    document.body,
  )
}

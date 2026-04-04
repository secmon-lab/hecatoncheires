import { Database, FileText, GitBranch, MessageSquare } from 'lucide-react'
import Modal from '../Modal'
import Button from '../Button'
import { useTranslation } from '../../i18n'
import { SOURCE_TYPE } from '../../constants/source'
import styles from './source.module.css'

interface SourceTypeSelectorProps {
  isOpen: boolean
  onClose: () => void
  onSelect: (type: string) => void
}

interface SourceTypeOption {
  type: string
  name: string
  description: string
  icon: React.ReactNode
}

export default function SourceTypeSelector({ isOpen, onClose, onSelect }: SourceTypeSelectorProps) {
  const { t } = useTranslation()

  const sourceTypes: SourceTypeOption[] = [
    {
      type: SOURCE_TYPE.NOTION_DB,
      name: t('sourceTypeNotionDB'),
      description: t('descSourceNotionDB'),
      icon: <Database size={24} />,
    },
    {
      type: SOURCE_TYPE.NOTION_PAGE,
      name: t('sourceTypeNotionPage'),
      description: t('descSourceNotionPage'),
      icon: <FileText size={24} />,
    },
    {
      type: SOURCE_TYPE.SLACK,
      name: t('sourceTypeSlack'),
      description: t('descSourceSlack'),
      icon: <MessageSquare size={24} />,
    },
    {
      type: SOURCE_TYPE.GITHUB,
      name: t('sourceTypeGitHub'),
      description: t('descSourceGitHub'),
      icon: <GitBranch size={24} />,
    },
  ]

  const handleSelect = (type: string) => {
    onSelect(type)
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('titleSelectSourceType')}
      footer={
        <Button variant="outline" onClick={onClose}>
          {t('btnCancel')}
        </Button>
      }
    >
      <div className={styles.typeList}>
        {sourceTypes.map((sourceType) => (
          <button
            key={sourceType.type}
            className={styles.typeCard}
            onClick={() => handleSelect(sourceType.type)}
          >
            <div className={styles.typeIcon}>{sourceType.icon}</div>
            <div className={styles.typeInfo}>
              <h3 className={styles.typeName}>{sourceType.name}</h3>
              <p className={styles.typeDescription}>{sourceType.description}</p>
            </div>
          </button>
        ))}
      </div>
    </Modal>
  )
}

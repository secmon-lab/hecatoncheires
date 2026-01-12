import { Database, MessageSquare } from 'lucide-react'
import Modal from '../Modal'
import Button from '../Button'
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

const sourceTypes: SourceTypeOption[] = [
  {
    type: SOURCE_TYPE.NOTION_DB,
    name: 'Notion Database',
    description: 'Connect to a Notion database to monitor and track changes',
    icon: <Database size={24} />,
  },
  {
    type: SOURCE_TYPE.SLACK,
    name: 'Slack Channels',
    description: 'Monitor Slack channels for risk-related discussions and incidents',
    icon: <MessageSquare size={24} />,
  },
]

export default function SourceTypeSelector({ isOpen, onClose, onSelect }: SourceTypeSelectorProps) {
  const handleSelect = (type: string) => {
    onSelect(type)
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Select Source Type"
      footer={
        <Button variant="outline" onClick={onClose}>
          Cancel
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

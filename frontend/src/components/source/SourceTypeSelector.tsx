import Modal from '../Modal'
import Button from '../Button'
import { useTranslation } from '../../i18n'
import { SOURCE_TYPE } from '../../constants/source'
import { IconNotion, IconSlack, IconGitHub } from '../Icons'

interface SourceTypeSelectorProps {
  isOpen: boolean
  onClose: () => void
  onSelect: (type: string) => void
}

export default function SourceTypeSelector({ isOpen, onClose, onSelect }: SourceTypeSelectorProps) {
  const { t } = useTranslation()

  const sourceTypes = [
    {
      type: SOURCE_TYPE.NOTION_DB,
      name: t('sourceTypeNotionDB'),
      description: t('descSourceNotionDB'),
      icon: <IconNotion size={22} />,
    },
    {
      type: SOURCE_TYPE.NOTION_PAGE,
      name: t('sourceTypeNotionPage'),
      description: t('descSourceNotionPage'),
      icon: <IconNotion size={22} />,
    },
    {
      type: SOURCE_TYPE.SLACK,
      name: t('sourceTypeSlack'),
      description: t('descSourceSlack'),
      icon: <IconSlack size={22} />,
    },
    {
      type: SOURCE_TYPE.GITHUB,
      name: t('sourceTypeGitHub'),
      description: t('descSourceGitHub'),
      icon: <IconGitHub size={22} />,
    },
  ]

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('titleSelectSourceType')}
      width={640}
      footer={
        <Button variant="ghost" onClick={onClose}>
          {t('btnCancel')}
        </Button>
      }
    >
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(2, 1fr)',
          gap: 10,
        }}
      >
        {sourceTypes.map((s) => (
          <button
            key={s.type}
            type="button"
            className="source-card"
            onClick={() => onSelect(s.type)}
            style={{ textAlign: 'left', fontFamily: 'inherit', color: 'inherit' }}
            data-testid={`source-type-${s.type}`}
          >
            <div className="ic">{s.icon}</div>
            <h4>{s.name}</h4>
            <p>{s.description}</p>
          </button>
        ))}
      </div>
    </Modal>
  )
}

import { useMemo, useRef, useState } from 'react'
import InlineFieldFrame from './InlineFieldFrame'
import InlinePopover from './InlinePopover'
import styles from './Inline.module.css'
import { Avatar, AvatarStack } from '../Primitives'
import { IconCheck } from '../Icons'
import { useTranslation } from '../../i18n'

export interface UserItem {
  id: string
  name: string
  realName: string
  imageUrl?: string | null
}

interface BaseProps {
  users: UserItem[]
  ariaLabel: string
  placeholder?: string
  disabled?: boolean
  testId?: string
}

interface SingleProps extends BaseProps {
  isMulti?: false
  value: string | null
  onSave: (next: string | null) => Promise<void> | void
}

interface MultiProps extends BaseProps {
  isMulti: true
  value: string[]
  onSave: (next: string[]) => Promise<void> | void
}

type Props = SingleProps | MultiProps

export default function InlineUserSelect(props: Props) {
  const { users, ariaLabel, placeholder, disabled, testId } = props
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const anchorRef = useRef<HTMLDivElement>(null)

  const selectedIds = useMemo<string[]>(() => {
    if (props.isMulti) return props.value
    return props.value ? [props.value] : []
  }, [props.isMulti, props.value])

  const selectedUsers = useMemo(
    () => users.filter((u) => selectedIds.includes(u.id)),
    [users, selectedIds],
  )

  const filtered = useMemo(() => {
    if (!query) return users
    const q = query.toLowerCase()
    return users.filter((u) =>
      (u.realName || '').toLowerCase().includes(q) ||
      (u.name || '').toLowerCase().includes(q))
  }, [users, query])

  const handlePick = async (id: string) => {
    if (props.isMulti) {
      const next = props.value.includes(id)
        ? props.value.filter((v) => v !== id)
        : [...props.value, id]
      await props.onSave(next)
    } else {
      setOpen(false)
      setQuery('')
      if (props.value === id) return
      await props.onSave(id)
    }
  }

  return (
    <>
      <div ref={anchorRef} style={{ display: 'inline-flex', maxWidth: '100%' }}>
        <InlineFieldFrame
          ariaLabel={ariaLabel}
          disabled={disabled}
          onActivate={() => setOpen((v) => !v)}
          testId={testId}
        >
          {selectedUsers.length === 0 ? (
            <span className={styles.placeholder}>{placeholder || '—'}</span>
          ) : selectedUsers.length === 1 ? (
            <>
              <Avatar
                size="sm"
                name={selectedUsers[0].name}
                realName={selectedUsers[0].realName}
                imageUrl={selectedUsers[0].imageUrl}
              />
              <span className={styles.optionLabel}>
                {selectedUsers[0].realName || selectedUsers[0].name}
              </span>
            </>
          ) : (
            <AvatarStack users={selectedUsers} max={4} />
          )}
        </InlineFieldFrame>
      </div>
      <InlinePopover
        anchor={anchorRef.current}
        open={open}
        onClose={() => { setOpen(false); setQuery('') }}
        testId={testId ? `${testId}-popover` : undefined}
      >
        <div className={styles.popoverSearch}>
          <input
            autoFocus
            className={styles.popoverSearchInput}
            placeholder={t('placeholderSearch')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            data-testid={testId ? `${testId}-search` : undefined}
          />
        </div>
        {filtered.length === 0 ? (
          <div className={styles.optionEmpty}>{t('noDataAvailable')}</div>
        ) : (
          filtered.map((u) => {
            const active = selectedIds.includes(u.id)
            return (
              <button
                key={u.id}
                type="button"
                className={`${styles.option} ${active ? styles.optionActive : ''}`}
                onClick={() => void handlePick(u.id)}
                data-testid={testId ? `${testId}-option-${u.id}` : undefined}
              >
                <Avatar size="sm" name={u.name} realName={u.realName} imageUrl={u.imageUrl} />
                <span className={styles.optionLabel}>{u.realName || u.name}</span>
                {active && <IconCheck size={12} className={styles.optionCheck} />}
              </button>
            )
          })
        )}
      </InlinePopover>
    </>
  )
}

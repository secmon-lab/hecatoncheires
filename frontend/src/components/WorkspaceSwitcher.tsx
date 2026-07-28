import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router'
import { IconChevDown } from './Icons'
import { useTranslation } from '../i18n'
import { sortByFavorite, workspaceVisual, type WorkspaceVisualInput } from '../utils/workspace'
import styles from './WorkspaceSwitcher.module.css'

interface Workspace {
  id: string
  name: string
  emoji?: string | null
  color?: string | null
}

interface Props {
  current: Workspace | null
  workspaces: Workspace[]
  favoriteWorkspaceIds: string[]
}

// Rows shown before the menu collapses the rest behind the "show more" toggle.
// Favorites are exempt from the cap (see splitWorkspaces).
const collapsedRowLimit = 10

// Ties the trigger button to the popup via aria-controls. Only one switcher is
// rendered per page (it lives in the top bar), so a fixed id is unambiguous.
const menuElementId = 'workspace-switcher-menu'

// splitWorkspaces returns the rows rendered while collapsed and the rows kept
// behind the toggle. Every favorite is always visible, even when favorites
// alone exceed the limit; non-favorites only fill the slots left over.
function splitWorkspaces(
  workspaces: Workspace[],
  favoriteIds: string[],
  limit: number,
): { visible: Workspace[]; hidden: Workspace[] } {
  const favSet = new Set(favoriteIds)
  const favorites = workspaces.filter((ws) => favSet.has(ws.id))
  const others = workspaces.filter((ws) => !favSet.has(ws.id))
  const slots = Math.max(0, limit - favorites.length)
  return {
    visible: [...favorites, ...others.slice(0, slots)],
    hidden: others.slice(slots),
  }
}

function WorkspaceBadge({ ws }: { ws: WorkspaceVisualInput }) {
  const v = workspaceVisual(ws)
  return (
    <span
      className={v.kind === 'emoji' ? 'h-ws-mark is-emoji' : 'h-ws-mark'}
      style={{ background: v.background }}
    >
      {v.kind === 'emoji' ? v.emoji : v.mark}
    </span>
  )
}

export default function WorkspaceSwitcher({ current, workspaces, favoriteWorkspaceIds }: Props) {
  const [open, setOpen] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const { t } = useTranslation()

  useEffect(() => {
    if (!open) return
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [open])

  // Reopening the menu starts collapsed again, so a long list never reopens
  // fully expanded from a previous visit.
  useEffect(() => {
    if (!open) setExpanded(false)
  }, [open])

  const favSet = useMemo(() => new Set(favoriteWorkspaceIds), [favoriteWorkspaceIds])
  const { visible, hidden } = useMemo(
    () => splitWorkspaces(sortByFavorite(workspaces, favoriteWorkspaceIds), favoriteWorkspaceIds, collapsedRowLimit),
    [workspaces, favoriteWorkspaceIds],
  )

  if (!current) return null

  const mark = <WorkspaceBadge ws={current} />

  if (workspaces.length <= 1) {
    return (
      <button type="button" className={`h-ws ${styles.staticTrigger}`} tabIndex={-1}>
        {mark}
        <span>{current.name}</span>
      </button>
    )
  }

  const rows = expanded ? [...visible, ...hidden] : visible

  return (
    <div
      ref={ref}
      className={styles.root}
      onKeyDown={(e) => {
        // Escape closes the popup and returns focus to the trigger, so a
        // keyboard user is not left inside a dismissed scroll region.
        if (e.key !== 'Escape' || !open) return
        e.preventDefault()
        setOpen(false)
        triggerRef.current?.focus()
      }}
    >
      <button
        type="button"
        ref={triggerRef}
        className="h-ws"
        aria-expanded={open}
        aria-controls={menuElementId}
        onClick={() => setOpen((v) => !v)}
      >
        {mark}
        <span>{current.name}</span>
        <IconChevDown size={14} />
      </button>
      {open && (
        <div id={menuElementId} className={styles.menu} data-testid="workspace-switcher-menu">
          {rows.map((ws) => {
            const isFav = favSet.has(ws.id)
            const isCurrent = ws.id === current.id
            return (
              <Link
                key={ws.id}
                to={`/ws/${ws.id}/cases`}
                className={isCurrent ? `${styles.item} ${styles.itemCurrent}` : styles.item}
                data-testid={`workspace-switcher-item-${ws.id}`}
                onClick={() => setOpen(false)}
              >
                <WorkspaceBadge ws={ws} />
                <span>{ws.name}</span>
                {isFav && (
                  <span
                    className={styles.star}
                    data-testid={`workspace-switcher-star-${ws.id}`}
                    title={t('fav')}
                    aria-label={t('fav')}
                    role="img"
                  >
                    ★
                  </span>
                )}
              </Link>
            )
          })}
          {hidden.length > 0 && (
            <button
              type="button"
              className={styles.moreButton}
              data-testid="workspace-switcher-more"
              aria-expanded={expanded}
              onClick={() => setExpanded((v) => !v)}
            >
              {expanded
                ? t('workspaceSwitcherShowLess')
                : t('workspaceSwitcherShowMore', { count: hidden.length })}
            </button>
          )}
        </div>
      )}
    </div>
  )
}

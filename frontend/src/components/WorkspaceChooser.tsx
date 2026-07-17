import { Link } from 'react-router-dom'
import { useTranslation } from '../i18n'
import { workspaceVisual, type WorkspaceVisualInput } from '../utils/workspace'
import styles from './WorkspaceChooser.module.css'

interface WorkspaceChooserProps {
  workspaces: WorkspaceVisualInput[]
  isLoading: boolean
  favoriteWorkspaceIds: string[]
  onToggleFavorite: (workspaceId: string) => void
}

// Favorites first, otherwise the order the workspace list was given in.
// Array.prototype.sort is stable per spec, so ties keep their relative order.
function sortByFavorite<T extends { id: string }>(workspaces: T[], favoriteIds: string[]): T[] {
  const favSet = new Set(favoriteIds)
  return [...workspaces].sort((a, b) => {
    const rankA = favSet.has(a.id) ? 0 : 1
    const rankB = favSet.has(b.id) ? 0 : 1
    return rankA - rankB
  })
}

export default function WorkspaceChooser({
  workspaces,
  isLoading,
  favoriteWorkspaceIds,
  onToggleFavorite,
}: WorkspaceChooserProps) {
  const { t } = useTranslation()

  if (isLoading) {
    return (
      <div className={styles.grid} aria-hidden="true" data-testid="workspace-chooser-loading">
        <div className={styles.skeletonCard} />
        <div className={styles.skeletonCard} />
        <div className={styles.skeletonCard} />
      </div>
    )
  }

  if (workspaces.length === 0) {
    return (
      <div className={styles.emptyState} data-testid="workspace-chooser-empty">
        <div className={styles.emptyTitle}>{t('workspaceSelectorEmpty')}</div>
        <div className={styles.emptyBody}>{t('workspaceSelectorEmptyHint')}</div>
      </div>
    )
  }

  const sorted = sortByFavorite(workspaces, favoriteWorkspaceIds)
  const favSet = new Set(favoriteWorkspaceIds)

  return (
    <div className={styles.grid} data-testid="workspace-chooser">
      {sorted.map((ws) => {
        const visual = workspaceVisual(ws)
        const isFav = favSet.has(ws.id)
        return (
          <Link
            key={ws.id}
            to={`/ws/${ws.id}/cases`}
            className={styles.card}
            data-testid={`workspace-card-${ws.id}`}
          >
            {visual.kind === 'emoji' ? (
              <span className={styles.badgeEmoji} aria-hidden="true">{visual.emoji}</span>
            ) : (
              <span className={styles.badgeColor} style={{ background: visual.background }} aria-hidden="true" />
            )}
            <span className={styles.name}>{ws.name}</span>
            <button
              type="button"
              className={`${styles.starButton} ${isFav ? styles.starActive : styles.starInactive}`}
              aria-label={t('fav')}
              aria-pressed={isFav}
              data-testid={`workspace-favorite-${ws.id}`}
              onClick={(e) => {
                // The card itself is a Link; a bare click would both toggle
                // the star AND navigate to the workspace. Stop it here.
                e.preventDefault()
                e.stopPropagation()
                onToggleFavorite(ws.id)
              }}
            >
              {isFav ? '★' : '☆'}
            </button>
          </Link>
        )
      })}
    </div>
  )
}

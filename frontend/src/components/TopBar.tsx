import { Bell, Menu, Search } from 'lucide-react'
import { useIsMobileOrTablet } from '../hooks/useMediaQuery'
import { useWorkspace } from '../contexts/workspace-context'
import WorkspaceSwitcher from './WorkspaceSwitcher'
import styles from './TopBar.module.css'
import { UserMenu } from './UserMenu'

interface TopBarProps {
  onToggleSidebar?: () => void
}

export default function TopBar({ onToggleSidebar }: TopBarProps) {
  const isMobileOrTablet = useIsMobileOrTablet()
  const { currentWorkspace, workspaces } = useWorkspace()

  return (
    <header className={styles.topBar}>
      <div className={styles.leftSection}>
        {isMobileOrTablet && onToggleSidebar && (
          <button className={styles.menuButton} onClick={onToggleSidebar} aria-label="Toggle navigation">
            <Menu size={18} />
          </button>
        )}
        <WorkspaceSwitcher current={currentWorkspace} workspaces={workspaces} />
      </div>

      <div className="h-search" style={{ marginLeft: 'auto' }}>
        <Search size={14} />
        <span>Search cases, actions, channels…</span>
        <span style={{ marginLeft: 'auto', fontSize: 10, padding: '1px 5px', borderRadius: 3, background: 'var(--bg-sunken)', border: '1px solid var(--line)' }}>⌘K</span>
      </div>

      <div className={styles.actions}>
        <button className={styles.iconButton} aria-label="Notifications">
          <Bell size={16} />
        </button>
        <UserMenu />
      </div>
    </header>
  )
}

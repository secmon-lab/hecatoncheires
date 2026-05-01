import { Bell, Menu } from 'lucide-react'
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
          <button className={styles.menuButton} onClick={onToggleSidebar}>
            <Menu size={20} />
          </button>
        )}
        <WorkspaceSwitcher
          current={currentWorkspace}
          workspaces={workspaces}
        />
      </div>

      <div className={styles.actions}>
        <button className={styles.iconButton}>
          <Bell size={18} />
        </button>
        <UserMenu />
      </div>
    </header>
  )
}

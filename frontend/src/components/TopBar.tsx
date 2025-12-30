import { Bell, Menu } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useIsMobileOrTablet } from '../hooks/useMediaQuery'
import styles from './TopBar.module.css'
import { UserMenu } from './UserMenu'

interface TopBarProps {
  onToggleSidebar?: () => void
}

export default function TopBar({ onToggleSidebar }: TopBarProps) {
  const isMobileOrTablet = useIsMobileOrTablet()

  return (
    <header className={styles.topBar}>
      <div className={styles.leftSection}>
        {isMobileOrTablet && onToggleSidebar && (
          <button className={styles.menuButton} onClick={onToggleSidebar}>
            <Menu size={24} />
          </button>
        )}
        <Link to="/" className={styles.logo}>
          <img src="/logo.png" alt="Hecatoncheires" className={styles.logoIcon} />
          <span className={styles.logoText}>Hecatoncheires</span>
        </Link>
      </div>

      <div className={styles.actions}>
        <button className={styles.iconButton}>
          <Bell size={20} />
        </button>
        <UserMenu />
      </div>
    </header>
  )
}

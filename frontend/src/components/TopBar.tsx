import { Bell } from 'lucide-react'
import { Link } from 'react-router-dom'
import styles from './TopBar.module.css'
import { UserMenu } from './UserMenu'

export default function TopBar() {
  return (
    <header className={styles.topBar}>
      <div className={styles.leftSection}>
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

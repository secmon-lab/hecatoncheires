import { Shield } from 'lucide-react'
import { NavLink, Link } from 'react-router-dom'
import styles from './Sidebar.module.css'

export default function Sidebar() {
  return (
    <aside className={styles.sidebar}>
      <Link to="/" className={styles.logoContainer}>
        <div className={styles.logo}>
          <img src="/logo.png" alt="Hecatoncheires" className={styles.logoIcon} />
          <span className={styles.logoText}>Hecatoncheires</span>
        </div>
      </Link>

      <nav className={styles.nav}>
        <NavLink
          to="/risks"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
        >
          <Shield size={20} />
          <span>Risks</span>
        </NavLink>
      </nav>
    </aside>
  )
}

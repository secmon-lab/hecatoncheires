import { Shield, ListTodo, Database } from 'lucide-react'
import { NavLink } from 'react-router-dom'
import styles from './Sidebar.module.css'

interface SidebarProps {
  isOpen?: boolean
  onClose?: () => void
}

export default function Sidebar({ isOpen = true, onClose }: SidebarProps) {
  const handleNavClick = () => {
    if (onClose) {
      onClose()
    }
  }

  return (
    <aside className={`${styles.sidebar} ${isOpen ? styles.open : ''}`}>
      <nav className={styles.nav}>
        <NavLink
          to="/risks"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <Shield size={20} />
          <span>Risks</span>
        </NavLink>
        <NavLink
          to="/responses"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <ListTodo size={20} />
          <span>Responses</span>
        </NavLink>
        <NavLink
          to="/sources"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <Database size={20} />
          <span>Sources</span>
        </NavLink>
      </nav>
    </aside>
  )
}

import { Shield, ListTodo, Database, BookOpen } from 'lucide-react'
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
          to="/cases"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <Shield size={20} />
          <span>Cases</span>
        </NavLink>
        <NavLink
          to="/actions"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <ListTodo size={20} />
          <span>Actions</span>
        </NavLink>
        <NavLink
          to="/knowledges"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <BookOpen size={20} />
          <span>Knowledges</span>
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

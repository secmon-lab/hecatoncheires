import { Shield, ListTodo, Database, BookOpen } from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { useWorkspace } from '../contexts/workspace-context'
import styles from './Sidebar.module.css'

interface SidebarProps {
  isOpen?: boolean
  onClose?: () => void
}

export default function Sidebar({ isOpen = true, onClose }: SidebarProps) {
  const { currentWorkspace } = useWorkspace()
  const wsPrefix = currentWorkspace ? `/ws/${currentWorkspace.id}` : ''

  const handleNavClick = () => {
    if (onClose) {
      onClose()
    }
  }

  return (
    <aside className={`${styles.sidebar} ${isOpen ? styles.open : ''}`}>
      <nav className={styles.nav}>
        <NavLink
          to={`${wsPrefix}/cases`}
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <Shield size={20} />
          <span>Cases</span>
        </NavLink>
        <NavLink
          to={`${wsPrefix}/actions`}
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <ListTodo size={20} />
          <span>Actions</span>
        </NavLink>
        <NavLink
          to={`${wsPrefix}/knowledges`}
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <BookOpen size={20} />
          <span>Knowledges</span>
        </NavLink>
        <NavLink
          to={`${wsPrefix}/sources`}
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

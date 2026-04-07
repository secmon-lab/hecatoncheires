import { Shield, ListTodo, Database, BookOpen } from 'lucide-react'
import { NavLink, Link } from 'react-router-dom'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import styles from './Sidebar.module.css'

interface SidebarProps {
  isOpen?: boolean
  onClose?: () => void
}

export default function Sidebar({ isOpen = true, onClose }: SidebarProps) {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const wsPrefix = currentWorkspace ? `/ws/${currentWorkspace.id}` : ''

  const handleNavClick = () => {
    if (onClose) {
      onClose()
    }
  }

  return (
    <aside className={`${styles.sidebar} ${isOpen ? styles.open : ''}`}>
      <div className={styles.brand}>
        <Link to="/" className={styles.brandLink}>
          <img src="/logo.png" alt={t('appName')} className={styles.brandLogo} />
          <span className={styles.brandName}>{t('appName')}</span>
        </Link>
      </div>
      <nav className={styles.nav}>
        <NavLink
          to={`${wsPrefix}/cases`}
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <Shield size={20} />
          <span>{t('navCases')}</span>
        </NavLink>
        <NavLink
          to={`${wsPrefix}/actions`}
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <ListTodo size={20} />
          <span>{t('navActions')}</span>
        </NavLink>
        <NavLink
          to={`${wsPrefix}/knowledges`}
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <BookOpen size={20} />
          <span>{t('navKnowledges')}</span>
        </NavLink>
        <NavLink
          to={`${wsPrefix}/sources`}
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
          onClick={handleNavClick}
        >
          <Database size={20} />
          <span>{t('navSources')}</span>
        </NavLink>
      </nav>
    </aside>
  )
}

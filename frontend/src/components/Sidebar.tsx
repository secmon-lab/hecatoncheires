import { Briefcase, ListTodo, Database, BookOpen } from 'lucide-react'
import { NavLink, Link } from 'react-router-dom'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'

interface SidebarProps {
  isOpen?: boolean
  onClose?: () => void
}

export default function Sidebar({ onClose }: SidebarProps) {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const wsPrefix = currentWorkspace ? `/ws/${currentWorkspace.id}` : ''

  const handleNavClick = () => {
    if (onClose) onClose()
  }

  const items = [
    { to: `${wsPrefix}/cases`,      Icon: Briefcase, label: t('navCases') },
    { to: `${wsPrefix}/actions`,    Icon: ListTodo,  label: t('navActions') },
    { to: `${wsPrefix}/knowledges`, Icon: BookOpen,  label: t('navKnowledges') },
    { to: `${wsPrefix}/sources`,    Icon: Database,  label: t('navSources') },
  ]

  return (
    <aside className="h-side">
      <Link to="/" className="h-side-logo" onClick={handleNavClick}>
        <img src="/logo-three-heads.png" alt={t('appName')} />
        <span>{t('appName')}</span>
      </Link>
      <nav className="h-side-nav">
        <div className="h-side-section">Workspace</div>
        {items.map((it) => (
          <NavLink
            key={it.to}
            to={it.to}
            className={({ isActive }) => (isActive ? 'active' : '')}
            onClick={handleNavClick}
          >
            <span className="nav-icon"><it.Icon size={15} /></span>
            <span>{it.label}</span>
          </NavLink>
        ))}
      </nav>
    </aside>
  )
}

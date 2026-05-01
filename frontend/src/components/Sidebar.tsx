import { NavLink, Link } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { useWorkspace } from '../contexts/workspace-context'
import { useAuth } from '../contexts/auth-context'
import { useTranslation } from '../i18n'
import { GET_CASES } from '../graphql/case'
import { GET_ACTIONS } from '../graphql/action'
import { GET_KNOWLEDGES } from '../graphql/knowledge'
import { GET_SOURCES } from '../graphql/source'
import { Avatar } from './Primitives'
import {
  IconCases,
  IconActions,
  IconKnowledge,
  IconSources,
  IconSettings,
  IconUser,
} from './Icons'

interface NavCount {
  cases: number | null
  actions: number | null
  knowledges: number | null
  sources: number | null
}

function useNavCounts(workspaceId: string | undefined): NavCount {
  const { data: cases } = useQuery(GET_CASES, {
    variables: { workspaceId, status: 'OPEN' },
    skip: !workspaceId,
  })
  const { data: actions } = useQuery(GET_ACTIONS, {
    variables: { workspaceId },
    skip: !workspaceId,
  })
  const { data: knowledges } = useQuery(GET_KNOWLEDGES, {
    variables: { workspaceId },
    skip: !workspaceId,
  })
  const { data: sources } = useQuery(GET_SOURCES, {
    variables: { workspaceId },
    skip: !workspaceId,
  })
  return {
    cases: cases?.cases?.length ?? null,
    actions: actions?.actions?.length ?? null,
    knowledges: knowledges?.knowledges?.length ?? null,
    sources: sources?.sources?.length ?? null,
  }
}

export default function Sidebar() {
  const { currentWorkspace } = useWorkspace()
  const { user } = useAuth()
  const { t } = useTranslation()
  const wsPrefix = currentWorkspace ? `/ws/${currentWorkspace.id}` : ''
  const counts = useNavCounts(currentWorkspace?.id)

  const items = [
    { id: 'cases',      label: t('navCases'),      Icon: IconCases,     to: `${wsPrefix}/cases`,      count: counts.cases },
    { id: 'actions',    label: t('navActions'),    Icon: IconActions,   to: `${wsPrefix}/actions`,    count: counts.actions },
    { id: 'knowledges', label: t('navKnowledges'), Icon: IconKnowledge, to: `${wsPrefix}/knowledges`, count: counts.knowledges },
    { id: 'sources',    label: t('navSources'),    Icon: IconSources,   to: `${wsPrefix}/sources`,    count: counts.sources },
  ]

  return (
    <aside className="h-side">
      <Link to="/" className="h-side-logo" style={{ textDecoration: 'none' }}>
        <img src="/logo-three-heads.png" alt={t('appName')} />
        <span>{t('appName')}</span>
      </Link>
      <nav className="h-side-nav">
        <div className="h-side-section">{t('sidebarSectionWorkspace')}</div>
        {items.map((it) => (
          <NavLink
            key={it.id}
            to={it.to}
            className={({ isActive }) => (isActive ? 'active' : '')}
            end={it.id === 'cases'}
          >
            <span className="nav-icon"><it.Icon size={15} /></span>
            <span>{it.label}</span>
            {it.count != null && <span className="nav-count">{it.count}</span>}
          </NavLink>
        ))}
        <div className="h-side-section">{t('sidebarSectionWorkspaceSettings')}</div>
        <a
          href="#"
          aria-disabled="true"
          title="Coming soon"
          onClick={(e) => e.preventDefault()}
          style={{ opacity: 0.45, cursor: 'not-allowed' }}
        >
          <span className="nav-icon"><IconSettings size={15} /></span>
          <span>{t('navCustomFields')}</span>
        </a>
        <a
          href="#"
          aria-disabled="true"
          title="Coming soon"
          onClick={(e) => e.preventDefault()}
          style={{ opacity: 0.45, cursor: 'not-allowed' }}
        >
          <span className="nav-icon"><IconUser size={15} /></span>
          <span>{t('navMembers')}</span>
        </a>
      </nav>
      {user && (
        <div className="h-side-foot">
          <Avatar size="sm" name={user.name} />
          <div className="col" style={{ gap: 0, lineHeight: 1.2, minWidth: 0 }}>
            <div style={{ fontSize: 12, fontWeight: 500 }} className="truncate">{user.name}</div>
            <div style={{ fontSize: 11, color: 'var(--side-fg-muted)' }} className="truncate">{user.email}</div>
          </div>
        </div>
      )}
    </aside>
  )
}

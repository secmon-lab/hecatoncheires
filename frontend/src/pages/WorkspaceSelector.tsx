import { useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import { IconChevRight } from '../components/Icons'

function workspaceMark(name: string): string {
  const trimmed = (name || '').trim()
  if (!trimmed) return '?'
  const parts = trimmed.split(/\s+/)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return trimmed.slice(0, 2).toUpperCase()
}

export default function WorkspaceSelector() {
  const { workspaces, isLoading } = useWorkspace()
  const { t } = useTranslation()
  const navigate = useNavigate()

  useEffect(() => {
    if (isLoading) return
    if (workspaces.length === 1) {
      navigate(`/ws/${workspaces[0].id}/cases`, { replace: true })
      return
    }
    const last = localStorage.getItem('lastWorkspaceId')
    if (last && workspaces.find((w) => w.id === last)) {
      navigate(`/ws/${last}/cases`, { replace: true })
    }
  }, [workspaces, isLoading, navigate])

  if (isLoading) {
    return (
      <div className="login-stage">
        <div className="muted">Loading workspaces…</div>
      </div>
    )
  }

  if (workspaces.length === 0) {
    return (
      <div className="login-stage">
        <div className="login-card">
          <h1>No workspaces configured</h1>
          <p className="tag">Configure at least one workspace in your config files.</p>
        </div>
      </div>
    )
  }

  return (
    <div className="login-stage" style={{ minHeight: '100vh' }}>
      <div style={{ width: 560, maxWidth: '100%' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 28, justifyContent: 'center' }}>
          <img
            src="/logo-three-heads.png"
            alt={t('appName')}
            style={{ width: 42, height: 42, background: 'var(--logo-bg)', borderRadius: 8, padding: 3, objectFit: 'contain' }}
          />
          <div>
            <div style={{ fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em' }}>{t('appName')}</div>
            <div style={{ fontSize: 13, color: 'var(--fg-muted)' }}>Select a workspace to continue</div>
          </div>
        </div>
        <div className="col" style={{ gap: 8 }}>
          {workspaces.map((ws) => (
            <Link
              key={ws.id}
              to={`/ws/${ws.id}/cases`}
              onClick={() => localStorage.setItem('lastWorkspaceId', ws.id)}
              className="card"
              style={{
                display: 'flex', alignItems: 'center', gap: 14,
                padding: 16, textDecoration: 'none', color: 'inherit',
                background: 'var(--bg-elev)', cursor: 'pointer',
              }}
            >
              <div
                style={{
                  width: 40, height: 40, borderRadius: 8,
                  background: 'linear-gradient(135deg, oklch(0.55 0.15 264), oklch(0.45 0.16 290))',
                  color: 'white', display: 'flex', alignItems: 'center', justifyContent: 'center',
                  fontSize: 13, fontWeight: 700,
                }}
              >
                {workspaceMark(ws.name)}
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 600, fontSize: 14 }} className="truncate">{ws.name}</div>
                <div style={{ fontSize: 12, color: 'var(--fg-muted)' }} className="truncate">{ws.id}</div>
              </div>
              <IconChevRight size={16} style={{ color: 'var(--fg-soft)' }} />
            </Link>
          ))}
        </div>
      </div>
    </div>
  )
}

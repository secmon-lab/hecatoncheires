import { Link } from 'react-router-dom'
import { useAuth } from '../contexts/auth-context'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import { IconChevRight } from '../components/Icons'
import { workspaceVisual } from '../utils/workspace'

export default function WorkspaceSelector() {
  const { workspaces, isLoading } = useWorkspace()
  const { logout } = useAuth()
  const { t } = useTranslation()

  if (isLoading) {
    return (
      <div className="login-stage">
        <div className="muted">Loading workspaces…</div>
      </div>
    )
  }

  if (workspaces.length === 0) {
    return (
      <div className="login-stage" data-testid="workspace-selector-empty">
        <div className="login-card">
          <h1>{t('workspaceSelectorEmpty')}</h1>
          <p className="tag">{t('workspaceSelectorEmptyHint')}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="login-stage" style={{ minHeight: '100vh' }} data-testid="workspace-selector">
      <div style={{ width: 560, maxWidth: '100%' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 28, justifyContent: 'center' }}>
          <img
            src="/logo-three-heads.png"
            alt={t('appName')}
            style={{ width: 42, height: 42, background: 'var(--logo-bg)', borderRadius: 8, padding: 3, objectFit: 'contain' }}
          />
          <div>
            <h1 style={{ margin: 0, fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em' }}>{t('appName')}</h1>
            <div style={{ fontSize: 13, color: 'var(--fg-muted)' }}>{t('workspaceSelectorSubtitle')}</div>
          </div>
        </div>
        <div className="col" style={{ gap: 8 }}>
          {workspaces.map((ws) => {
            const visual = workspaceVisual(ws)
            return (
            <Link
              key={ws.id}
              to={`/ws/${ws.id}/cases`}
              data-testid={`workspace-item-${ws.id}`}
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
                  background: visual.background,
                  color: visual.kind === 'emoji' ? 'inherit' : 'white',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  fontSize: visual.kind === 'emoji' ? 20 : 13, fontWeight: 700,
                }}
              >
                {visual.kind === 'emoji' ? visual.emoji : visual.mark}
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontWeight: 600, fontSize: 14 }} className="truncate">{ws.name}</div>
                <div style={{ fontSize: 12, color: 'var(--fg-muted)' }} className="truncate">{ws.id}</div>
              </div>
              <IconChevRight size={16} style={{ color: 'var(--fg-soft)' }} />
            </Link>
            )
          })}
        </div>
        <div style={{ marginTop: 20, textAlign: 'center', color: 'var(--fg-soft)', fontSize: 12 }}>
          <button
            type="button"
            data-testid="workspace-signout"
            onClick={logout}
            style={{
              background: 'none', border: 'none', color: 'inherit',
              cursor: 'pointer', textDecoration: 'underline', fontSize: 12,
              fontFamily: 'inherit', padding: 0,
            }}
          >
            {t('btnLogout')}
          </button>
        </div>
      </div>
    </div>
  )
}

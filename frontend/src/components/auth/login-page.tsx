import { IconSlack } from '../Icons'
import { useTranslation } from '../../i18n'

export function LoginPage() {
  const { t } = useTranslation()

  const handleLogin = () => {
    const here = window.location.pathname + window.location.search + window.location.hash
    if (here && here !== '/') {
      const params = new URLSearchParams({ return_to: here })
      window.location.href = `/api/auth/login?${params.toString()}`
    } else {
      window.location.href = '/api/auth/login'
    }
  }

  return (
    <div className="login-stage" style={{ minHeight: '100vh' }}>
      <div className="login-card">
        <div className="login-hero">
          <img src="/logo-fullbody.png" alt={t('appName')} />
        </div>
        <h1>{t('appName')}</h1>
        <p className="tag">{t('appSubtitle')}</p>
        <button className="btn slack" onClick={handleLogin} style={{ margin: '0 auto' }}>
          <IconSlack size={20} />
          {t('btnSignInSlack')}
        </button>
      </div>
      <div className="login-foot">© 2026 Hecatoncheires</div>
    </div>
  )
}

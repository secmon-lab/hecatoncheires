import { SlackIcon } from '../icons/SlackIcon';
import { useTranslation } from '../../i18n';
import './login-page.css';

export function LoginPage() {
  const { t } = useTranslation();

  const handleLogin = () => {
    window.location.href = '/api/auth/login';
  };

  return (
    <div className="login-stage">
      <div className="login-card">
        <div className="login-hero">
          <img src="/logo-fullbody.png" alt={t('appName')} />
        </div>
        <h1>{t('appName')}</h1>
        <p className="tag">{t('appSubtitle')}</p>
        <button className="btn slack" onClick={handleLogin} style={{ margin: '0 auto' }}>
          <SlackIcon style={{ marginRight: 0 }} />
          {t('btnSignInSlack')}
        </button>
      </div>
      <div className="login-foot">© 2026 Hecatoncheires</div>
    </div>
  );
}

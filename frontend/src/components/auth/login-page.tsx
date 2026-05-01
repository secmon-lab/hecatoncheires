import { SlackIcon } from '../icons/SlackIcon';
import { useTranslation } from '../../i18n';
import './login-page.css';

export function LoginPage() {
  const { t } = useTranslation();

  const handleLogin = () => {
    window.location.href = "/api/auth/login";
  };

  return (
    <div className="login-container">
      <div className="login-card">
        <div className="login-header">
          <h2 className="login-title">{t('appName')}</h2>
          <p className="login-subtitle">{t('appSubtitle')}</p>
        </div>

        <button onClick={handleLogin} className="login-button">
          <SlackIcon style={{ marginRight: '8px' }} />
          {t('btnSignInSlack')}
        </button>

        <p className="login-description">
          {t('loginDescription')}
        </p>
      </div>
    </div>
  );
}

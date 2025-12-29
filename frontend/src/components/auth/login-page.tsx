import { SlackIcon } from '../icons/SlackIcon';
import './login-page.css';

export function LoginPage() {
  const handleLogin = () => {
    window.location.href = "/api/auth/login";
  };

  return (
    <div className="login-container">
      <div className="login-card">
        <div className="login-header">
          <h2 className="login-title">Hecatoncheires</h2>
          <p className="login-subtitle">AI native risk management system</p>
        </div>

        <button onClick={handleLogin} className="login-button">
          <SlackIcon style={{ marginRight: '8px' }} />
          Sign in with Slack
        </button>

        <p className="login-description">
          Authenticate using your Slack workspace
        </p>
      </div>
    </div>
  );
}

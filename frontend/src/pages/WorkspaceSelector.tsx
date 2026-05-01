import { useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ChevronRight } from 'lucide-react';
import { useWorkspace } from '../contexts/workspace-context';
import { useTranslation } from '../i18n';
import styles from './WorkspaceSelector.module.css';

export default function WorkspaceSelector() {
  const { workspaces, isLoading } = useWorkspace();
  const { t } = useTranslation();
  const navigate = useNavigate();

  useEffect(() => {
    if (isLoading) return;

    if (workspaces.length === 1) {
      navigate(`/ws/${workspaces[0].id}/cases`, { replace: true });
      return;
    }

    const lastWorkspaceId = localStorage.getItem('lastWorkspaceId');
    if (lastWorkspaceId && workspaces.find((w) => w.id === lastWorkspaceId)) {
      navigate(`/ws/${lastWorkspaceId}/cases`, { replace: true });
    }
  }, [workspaces, isLoading, navigate]);

  if (isLoading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading workspaces...</div>
      </div>
    );
  }

  if (workspaces.length === 0) {
    return (
      <div className={styles.container}>
        <div className={styles.empty}>
          <h2>No workspaces configured</h2>
          <p>Please configure at least one workspace in your configuration files.</p>
        </div>
      </div>
    );
  }

  const mark = (name: string) => name.slice(0, 2).toUpperCase();

  return (
    <div className={styles.container}>
      <div className={styles.inner}>
        <div className={styles.header}>
          <img src="/logo-three-heads.png" alt={t('appName')} className={styles.logo} />
          <div className={styles.titleBlock}>
            <h1 className={styles.title}>{t('appName')}</h1>
            <span className={styles.subtitle}>Select a workspace to continue</span>
          </div>
        </div>
        <div className={styles.list}>
          {workspaces.map((ws) => (
            <Link
              key={ws.id}
              to={`/ws/${ws.id}/cases`}
              className={styles.card}
              onClick={() => localStorage.setItem('lastWorkspaceId', ws.id)}
            >
              <div className={styles.mark}>{mark(ws.name)}</div>
              <div className={styles.cardBody}>
                <div className={styles.cardTitle}>{ws.name}</div>
                <div className={styles.cardId}>{ws.id}</div>
              </div>
              <ChevronRight size={16} className={styles.chev} />
            </Link>
          ))}
        </div>
      </div>
    </div>
  );
}

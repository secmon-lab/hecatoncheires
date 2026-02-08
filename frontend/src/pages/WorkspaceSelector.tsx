import { useEffect } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useWorkspace } from "../contexts/workspace-context";
import styles from "./WorkspaceSelector.module.css";

export default function WorkspaceSelector() {
  const { workspaces, isLoading } = useWorkspace();
  const navigate = useNavigate();

  useEffect(() => {
    if (isLoading) return;

    if (workspaces.length === 1) {
      navigate(`/ws/${workspaces[0].id}/cases`, { replace: true });
      return;
    }

    const lastWorkspaceId = localStorage.getItem("lastWorkspaceId");
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

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <img src="/logo-center.png" alt="Hecatoncheires" className={styles.logo} />
        <h1 className={styles.title}>Select Workspace</h1>
        <p className={styles.subtitle}>Choose a workspace to get started</p>
      </div>
      <div className={styles.grid}>
        {workspaces.map((ws) => (
          <Link
            key={ws.id}
            to={`/ws/${ws.id}/cases`}
            className={styles.card}
            onClick={() => localStorage.setItem("lastWorkspaceId", ws.id)}
          >
            <h3 className={styles.cardTitle}>{ws.name}</h3>
            <span className={styles.cardId}>{ws.id}</span>
          </Link>
        ))}
      </div>
    </div>
  );
}

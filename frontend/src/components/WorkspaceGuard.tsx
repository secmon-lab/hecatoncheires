import { useEffect } from "react";
import { useParams, Navigate } from "react-router-dom";
import { useWorkspace } from "../contexts/workspace-context";

interface WorkspaceGuardProps {
  children: React.ReactNode;
}

export default function WorkspaceGuard({ children }: WorkspaceGuardProps) {
  const { workspaceId } = useParams<{ workspaceId: string }>();
  const { currentWorkspace, workspaces, isLoading, setCurrentWorkspace } = useWorkspace();

  const workspace = workspaces.find((w) => w.id === workspaceId);

  useEffect(() => {
    if (workspace) {
      setCurrentWorkspace(workspace);
    }
  }, [workspace, setCurrentWorkspace]);

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (!workspace) {
    return <Navigate to="/" replace />;
  }

  // Don't render children until currentWorkspace is actually set in context
  // (useEffect runs after render, so there's a one-frame delay)
  if (!currentWorkspace || currentWorkspace.id !== workspaceId) {
    return <div>Loading...</div>;
  }

  return <>{children}</>;
}

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  useMemo,
  ReactNode,
} from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery } from "@apollo/client";
import {
  GET_FAVORITE_WORKSPACE_IDS,
  SET_FAVORITE_WORKSPACES,
} from "../graphql/dashboard";

interface Workspace {
  id: string;
  name: string;
  // Optional display hints from /api/workspaces. emoji and color are mutually
  // exclusive (enforced server-side); the UI resolves them via
  // utils/workspace.ts workspaceVisual().
  emoji?: string | null;
  color?: string | null;
}

interface WorkspaceContextType {
  currentWorkspace: Workspace | null;
  workspaces: Workspace[];
  isLoading: boolean;
  setCurrentWorkspace: (workspace: Workspace | null) => void;
  switchWorkspace: (workspaceId: string) => void;
  // Favorite workspace ids for the Home page's workspace chooser (see
  // pages/Home.tsx). Lives here rather than on Home itself so the favorite
  // set is available to any consumer, not just the page that first loads it.
  favoriteWorkspaceIds: string[];
  toggleFavorite: (workspaceId: string) => void;
}

const WorkspaceContext = createContext<WorkspaceContextType | undefined>(
  undefined
);

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [currentWorkspace, setCurrentWorkspace] = useState<Workspace | null>(
    null
  );
  const [isLoading, setIsLoading] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    const fetchWorkspaces = async () => {
      try {
        const response = await fetch("/api/workspaces", {
          credentials: "include",
        });

        if (response.ok) {
          const data = await response.json();
          setWorkspaces(data.workspaces || []);
        } else {
          setWorkspaces([]);
        }
      } catch (error) {
        console.error("Failed to fetch workspaces:", error);
        setWorkspaces([]);
      } finally {
        setIsLoading(false);
      }
    };

    fetchWorkspaces();
  }, []);

  const switchWorkspace = useCallback(
    (workspaceId: string) => {
      const ws = workspaces.find((w) => w.id === workspaceId);
      if (ws) {
        setCurrentWorkspace(ws);
        navigate(`/ws/${workspaceId}/cases`);
      }
    },
    [workspaces, navigate]
  );

  const { data: favoriteData } = useQuery<{ favoriteWorkspaceIds: string[] }>(
    GET_FAVORITE_WORKSPACE_IDS
  );
  const favoriteWorkspaceIds = useMemo(
    () => favoriteData?.favoriteWorkspaceIds ?? [],
    [favoriteData]
  );
  const [setFavoriteWorkspacesMutation] = useMutation<
    { setFavoriteWorkspaces: string[] },
    { workspaceIds: string[] }
  >(SET_FAVORITE_WORKSPACES);

  const toggleFavorite = useCallback(
    (workspaceId: string) => {
      const next = favoriteWorkspaceIds.includes(workspaceId)
        ? favoriteWorkspaceIds.filter((id) => id !== workspaceId)
        : [...favoriteWorkspaceIds, workspaceId];
      setFavoriteWorkspacesMutation({
        variables: { workspaceIds: next },
        // Optimistic so the star flips instantly; Apollo automatically
        // rolls the optimistic layer back if the real mutation errors, so
        // no manual revert/catch is needed here.
        optimisticResponse: { setFavoriteWorkspaces: next },
        update: (cache) => {
          cache.writeQuery({
            query: GET_FAVORITE_WORKSPACE_IDS,
            data: { favoriteWorkspaceIds: next },
          });
        },
      }).catch((error) => {
        console.error("Failed to update favorite workspaces:", error);
      });
    },
    [favoriteWorkspaceIds, setFavoriteWorkspacesMutation]
  );

  const value: WorkspaceContextType = {
    currentWorkspace,
    workspaces,
    isLoading,
    setCurrentWorkspace,
    switchWorkspace,
    favoriteWorkspaceIds,
    toggleFavorite,
  };

  return (
    <WorkspaceContext.Provider value={value}>
      {children}
    </WorkspaceContext.Provider>
  );
}

export function useWorkspace() {
  const context = useContext(WorkspaceContext);
  if (context === undefined) {
    throw new Error("useWorkspace must be used within a WorkspaceProvider");
  }
  return context;
}

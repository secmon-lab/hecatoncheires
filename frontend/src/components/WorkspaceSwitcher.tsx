import { useState, useRef, useEffect } from "react";
import { Link } from "react-router-dom";
import { ChevronDown } from "lucide-react";
import styles from "./WorkspaceSwitcher.module.css";

interface Workspace {
  id: string;
  name: string;
}

interface WorkspaceSwitcherProps {
  current: Workspace | null;
  workspaces: Workspace[];
}

export default function WorkspaceSwitcher({
  current,
  workspaces,
}: WorkspaceSwitcherProps) {
  const [isOpen, setIsOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };

    if (isOpen) {
      document.addEventListener("mousedown", handleClickOutside);
    }

    return () => {
      document.removeEventListener("mousedown", handleClickOutside);
    };
  }, [isOpen]);

  if (!current) return null;

  if (workspaces.length <= 1) {
    return (
      <div className={styles.container}>
        <span className={styles.label}>{current.name}</span>
      </div>
    );
  }

  return (
    <div className={styles.container} ref={ref}>
      <button
        className={styles.trigger}
        onClick={() => setIsOpen(!isOpen)}
      >
        <span className={styles.label}>{current.name}</span>
        <ChevronDown size={16} className={styles.icon} />
      </button>
      {isOpen && (
        <div className={styles.dropdown}>
          {workspaces.map((ws) => (
            <Link
              key={ws.id}
              to={`/ws/${ws.id}/cases`}
              className={`${styles.item} ${ws.id === current.id ? styles.active : ""}`}
              onClick={() => setIsOpen(false)}
            >
              {ws.name}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

import { useState, useEffect, useRef } from 'react';
import { User, LogOut } from 'lucide-react';
import { useAuth } from '../contexts/auth-context';
import styles from './UserMenu.module.css';

interface UserInfo {
  id: string;
  name: string;
  profile: {
    image_48: string;
  };
}

export function UserMenu() {
  const { user, logout } = useAuth();
  const [isOpen, setIsOpen] = useState(false);
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Fetch user info if authenticated
    if (user) {
      fetch(`/api/auth/user-info?user=${user.sub}`, {
        credentials: 'include',
      })
        .then((res) => res.json())
        .then((data) => setUserInfo(data))
        .catch((err) => console.error('Failed to fetch user info:', err));
    }
  }, [user]);

  useEffect(() => {
    // Close menu when clicking outside
    function handleClickOutside(event: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    }

    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [isOpen]);

  const handleLogout = async () => {
    await logout();
  };

  return (
    <div className={styles.userMenu} ref={menuRef}>
      <button
        className={styles.userButton}
        onClick={() => setIsOpen(!isOpen)}
        aria-label="User menu"
      >
        {userInfo?.profile.image_48 ? (
          <img
            src={userInfo.profile.image_48}
            alt={user?.name || 'User'}
            className={styles.avatar}
          />
        ) : (
          <div className={styles.avatarPlaceholder}>
            <User size={20} />
          </div>
        )}
      </button>

      {isOpen && (
        <div className={styles.dropdown}>
          <div className={styles.userInfo}>
            <div className={styles.userName}>{user?.name || 'User'}</div>
            {user && (
              <div className={styles.userEmail}>{user.email}</div>
            )}
          </div>
          <div className={styles.divider} />
          <button className={styles.menuItem} onClick={handleLogout}>
            <LogOut size={16} />
            <span>Logout</span>
          </button>
        </div>
      )}
    </div>
  );
}

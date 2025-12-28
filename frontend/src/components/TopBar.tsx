import { Bell, User } from 'lucide-react'
import styles from './TopBar.module.css'

interface TopBarProps {
  title: string
}

export default function TopBar({ title }: TopBarProps) {
  return (
    <header className={styles.topBar}>
      <h1 className={styles.title}>{title}</h1>

      <div className={styles.actions}>
        <button className={styles.iconButton}>
          <Bell size={20} />
        </button>
        <button className={styles.iconButton}>
          <User size={20} />
        </button>
      </div>
    </header>
  )
}

import { Shield, ListTodo } from 'lucide-react'
import { NavLink } from 'react-router-dom'
import styles from './Sidebar.module.css'

export default function Sidebar() {
  return (
    <aside className={styles.sidebar}>
      <nav className={styles.nav}>
        <NavLink
          to="/risks"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
        >
          <Shield size={20} />
          <span>Risks</span>
        </NavLink>
        <NavLink
          to="/responses"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ''}`
          }
        >
          <ListTodo size={20} />
          <span>Responses</span>
        </NavLink>
      </nav>
    </aside>
  )
}

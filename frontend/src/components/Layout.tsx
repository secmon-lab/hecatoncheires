import { ReactNode } from 'react'
import { Outlet } from 'react-router-dom'
import Sidebar from './Sidebar'
import TopBar from './TopBar'
import { useSidebarState } from '../hooks/useSidebarState'
import styles from './Layout.module.css'

interface LayoutProps {
  title?: string
  children?: ReactNode
}

export default function Layout({ children }: LayoutProps) {
  const { isOpen, toggle, close, isMobileMenuOpen } = useSidebarState()

  return (
    <div className={styles.layout}>
      {isMobileMenuOpen && <div className={styles.backdrop} onClick={close} />}
      <div className={`${styles.side} ${isOpen ? styles.open : ''}`}>
        <Sidebar isOpen={isOpen} onClose={close} />
      </div>
      <div className={styles.top}>
        <TopBar onToggleSidebar={toggle} />
      </div>
      <main className={styles.main}>{children || <Outlet />}</main>
    </div>
  )
}

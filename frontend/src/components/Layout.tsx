import { ReactNode } from 'react'
import { Outlet } from 'react-router-dom'
import Sidebar from './Sidebar'
import TopBar from './TopBar'
import styles from './Layout.module.css'

interface LayoutProps {
  title?: string
  children?: ReactNode
}

export default function Layout({ title = 'Dashboard', children }: LayoutProps) {
  return (
    <div className={styles.layout}>
      <Sidebar />
      <div className={styles.main}>
        <TopBar title={title} />
        <main className={styles.content}>
          {children || <Outlet />}
        </main>
      </div>
    </div>
  )
}

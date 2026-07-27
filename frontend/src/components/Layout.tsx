import { Outlet } from 'react-router'
import Sidebar from './Sidebar'
import TopBar from './TopBar'

export default function Layout() {
  return (
    <div className="h-app">
      <Sidebar />
      <TopBar />
      <main className="h-main">
        <Outlet />
      </main>
    </div>
  )
}

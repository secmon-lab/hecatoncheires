import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Top from './pages/Top'
import RiskList from './pages/RiskList'
import { AuthGuard } from './components/auth/auth-guard'

function App() {
  return (
    <BrowserRouter>
      <AuthGuard>
        <Routes>
          <Route path="/" element={<Layout title="Hecatoncheires" />}>
            <Route index element={<Top />} />
          </Route>
          <Route path="/risks" element={<Layout title="Risk Management" />}>
            <Route index element={<RiskList />} />
          </Route>
        </Routes>
      </AuthGuard>
    </BrowserRouter>
  )
}

export default App

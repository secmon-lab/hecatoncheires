import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Top from './pages/Top'
import RiskList from './pages/RiskList'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Layout title="Hecatoncheires" />}>
          <Route index element={<Top />} />
        </Route>
        <Route path="/risks" element={<Layout title="Risk Management" />}>
          <Route index element={<RiskList />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}

export default App

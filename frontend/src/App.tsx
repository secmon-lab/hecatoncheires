import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Top from './pages/Top'
import RiskList from './pages/RiskList'
import RiskDetail from './pages/RiskDetail'
import ResponseList from './pages/ResponseList'
import ResponseDetail from './pages/ResponseDetail'
import KnowledgeList from './pages/KnowledgeList'
import KnowledgeDetail from './pages/KnowledgeDetail'
import SourceList from './pages/SourceList'
import SourceDetail from './pages/SourceDetail'
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
            <Route path=":id" element={<RiskDetail />} />
          </Route>
          <Route path="/responses" element={<Layout title="Response Management" />}>
            <Route index element={<ResponseList />} />
            <Route path=":id" element={<ResponseDetail />} />
          </Route>
          <Route path="/knowledges" element={<Layout title="Knowledge Base" />}>
            <Route index element={<KnowledgeList />} />
            <Route path=":id" element={<KnowledgeDetail />} />
          </Route>
          <Route path="/sources" element={<Layout title="Sources" />}>
            <Route index element={<SourceList />} />
            <Route path=":id" element={<SourceDetail />} />
          </Route>
        </Routes>
      </AuthGuard>
    </BrowserRouter>
  )
}

export default App

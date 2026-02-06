import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Top from './pages/Top'
import CaseList from './pages/CaseList'
import CaseDetail from './pages/CaseDetail'
import ActionList from './pages/ActionList'
import ActionDetail from './pages/ActionDetail'
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
          <Route path="/cases" element={<Layout title="Case Management" />}>
            <Route index element={<CaseList />} />
            <Route path=":id" element={<CaseDetail />} />
          </Route>
          <Route path="/actions" element={<Layout title="Action Management" />}>
            <Route index element={<ActionList />} />
            <Route path=":id" element={<ActionDetail />} />
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

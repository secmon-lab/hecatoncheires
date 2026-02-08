import { Routes, Route } from 'react-router-dom'
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
import WorkspaceSelector from './pages/WorkspaceSelector'
import WorkspaceGuard from './components/WorkspaceGuard'
import { AuthGuard } from './components/auth/auth-guard'

function App() {
  return (
    <AuthGuard>
      <Routes>
        <Route path="/" element={<WorkspaceSelector />} />
        <Route path="/ws/:workspaceId" element={<WorkspaceGuard><Layout /></WorkspaceGuard>}>
          <Route index element={<Top />} />
          <Route path="cases" element={<CaseList />} />
          <Route path="cases/:id" element={<CaseDetail />} />
          <Route path="actions" element={<ActionList />} />
          <Route path="actions/:id" element={<ActionDetail />} />
          <Route path="knowledges" element={<KnowledgeList />} />
          <Route path="knowledges/:id" element={<KnowledgeDetail />} />
          <Route path="sources" element={<SourceList />} />
          <Route path="sources/:id" element={<SourceDetail />} />
        </Route>
      </Routes>
    </AuthGuard>
  )
}

export default App

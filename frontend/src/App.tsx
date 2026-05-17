import { Navigate, Routes, Route, useParams } from 'react-router-dom'
import Layout from './components/Layout'
import CaseList from './pages/CaseList'
import CaseDetail from './pages/CaseDetail'
import ActionList from './pages/ActionList'
import AssistLogList from './pages/AssistLogList'
import SourceList from './pages/SourceList'
import SourceDetail from './pages/SourceDetail'
import WorkspaceSelector from './pages/WorkspaceSelector'
import WorkspaceGuard from './components/WorkspaceGuard'
import { AuthGuard } from './components/auth/auth-guard'

// Legacy /drafts/:id URLs forward to the unified /cases/:id page so old
// links and Slack ephemerals stay valid after the dedicated draft pages
// were retired.
function DraftDetailRedirect() {
  const { id } = useParams<{ id: string }>()
  return <Navigate to={`../cases/${id ?? ''}`} replace />
}

function App() {
  return (
    <AuthGuard>
      <Routes>
        <Route path="/" element={<WorkspaceSelector />} />
        <Route path="/ws/:workspaceId" element={<WorkspaceGuard><Layout /></WorkspaceGuard>}>
          <Route index element={<Navigate to="cases" replace />} />
          <Route path="cases" element={<CaseList />} />
          <Route path="cases/:id" element={<CaseDetail />} />
          <Route path="cases/:id/actions/:actionId" element={<CaseDetail />} />
          <Route path="cases/:id/assists" element={<AssistLogList />} />
          <Route path="actions" element={<ActionList />} />
          <Route path="actions/:actionId" element={<ActionList />} />
          {/* Drafts live inside the regular Case list/detail pages; the
              Drafts tab in CaseList filters by status and individual draft
              cases open at /cases/:id like any other case. */}
          <Route path="drafts" element={<Navigate to="../cases" replace />} />
          <Route path="drafts/:id" element={<DraftDetailRedirect />} />
          <Route path="sources" element={<SourceList />} />
          <Route path="sources/:id" element={<SourceDetail />} />
        </Route>
      </Routes>
    </AuthGuard>
  )
}

export default App

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { ApolloClient, InMemoryCache, ApolloProvider } from '@apollo/client'
import App from './App.tsx'
import { AuthProvider } from './contexts/auth-context.tsx'
import { WorkspaceProvider } from './contexts/workspace-context.tsx'
import './styles/global.css'

const client = new ApolloClient({
  uri: '/graphql',
  cache: new InMemoryCache(),
  credentials: 'include', // Include cookies for authentication
})

const rootElement = document.getElementById('root')
if (!rootElement) {
  throw new Error('Failed to find the root element')
}

createRoot(rootElement).render(
  <StrictMode>
    <AuthProvider>
      <ApolloProvider client={client}>
        <BrowserRouter>
          <WorkspaceProvider>
            <App />
          </WorkspaceProvider>
        </BrowserRouter>
      </ApolloProvider>
    </AuthProvider>
  </StrictMode>,
)

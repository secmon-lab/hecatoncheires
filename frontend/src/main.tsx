import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ApolloClient, InMemoryCache, ApolloProvider } from '@apollo/client'
import App from './App.tsx'

const client = new ApolloClient({
  uri: '/graphql',
  cache: new InMemoryCache(),
})

const rootElement = document.getElementById('root')
if (!rootElement) {
  throw new Error('Failed to find the root element')
}

createRoot(rootElement).render(
  <StrictMode>
    <ApolloProvider client={client}>
      <App />
    </ApolloProvider>
  </StrictMode>,
)

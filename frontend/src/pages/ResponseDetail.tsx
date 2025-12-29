import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation } from '@apollo/client'
import { ArrowLeft, Edit, MoreVertical, Trash2, ExternalLink } from 'lucide-react'
import { useState, useRef, useEffect } from 'react'
import Button from '../components/Button'
import ResponseForm from './ResponseForm'
import ResponseDeleteDialog from './ResponseDeleteDialog'
import { GET_RESPONSE, UPDATE_RESPONSE } from '../graphql/response'
import styles from './ResponseDetail.module.css'

interface Response {
  id: number
  title: string
  description: string
  responders: Array<{ id: string; name: string; realName: string; imageUrl?: string }>
  url?: string
  status: string
  risks: Array<{ id: number; name: string; description: string }>
  createdAt: string
  updatedAt: string
}

const STATUS_LABELS: Record<string, string> = {
  BACKLOG: 'Backlog',
  TODO: 'To Do',
  IN_PROGRESS: 'In Progress',
  BLOCKED: 'Blocked',
  COMPLETED: 'Completed',
  ABANDONED: 'Abandoned',
}

export default function ResponseDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  const { data, loading, error } = useQuery(GET_RESPONSE, {
    variables: { id: parseInt(id || '0') },
    skip: !id,
  })

  const response: Response | undefined = data?.response

  const [updateResponse] = useMutation(UPDATE_RESPONSE, {
    onError: (error) => {
      console.error('Update status error:', error)
    },
  })

  const handleBack = () => {
    navigate('/responses')
  }

  const handleEdit = () => {
    setIsFormOpen(true)
  }

  const handleDelete = () => {
    setIsDeleteDialogOpen(true)
  }

  const handleDeleteConfirm = () => {
    setIsDeleteDialogOpen(false)
    navigate('/responses')
  }

  const handleRiskClick = (riskId: number) => {
    navigate(`/risks/${riskId}`)
  }

  const handleStatusChange = async (newStatus: string) => {
    if (!response) return

    await updateResponse({
      variables: {
        input: {
          id: response.id,
          status: newStatus,
        },
      },
    })
  }

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsMenuOpen(false)
      }
    }

    if (isMenuOpen) {
      document.addEventListener('mousedown', handleClickOutside)
    }

    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [isMenuOpen])

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading...</div>
      </div>
    )
  }

  if (error || !response) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          {error ? `Error: ${error.message}` : 'Response not found'}
        </div>
        <Button variant="outline" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          Back to List
        </Button>
      </div>
    )
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <Button variant="ghost" icon={<ArrowLeft size={20} />} onClick={handleBack}>
          Back
        </Button>
        <div className={styles.actions}>
          <Button variant="outline" icon={<Edit size={18} />} onClick={handleEdit}>
            Edit
          </Button>
          <div className={styles.menuContainer} ref={menuRef}>
            <Button
              variant="ghost"
              icon={<MoreVertical size={18} />}
              onClick={() => setIsMenuOpen(!isMenuOpen)}
            />
            {isMenuOpen && (
              <div className={styles.menu}>
                <button className={styles.menuItem} onClick={handleDelete}>
                  <Trash2 size={16} />
                  Delete
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      <div className={styles.content}>
        <div className={styles.titleSection}>
          <h1>{response.title}</h1>
        </div>

        <div className={styles.section}>
          <h2>Status</h2>
          <select
            value={response.status}
            onChange={(e) => handleStatusChange(e.target.value)}
            className={styles.statusSelect}
          >
            {Object.entries(STATUS_LABELS).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </div>

        <div className={styles.section}>
          <h2>Description</h2>
          <p>{response.description || 'No description provided'}</p>
        </div>

        {response.responders && response.responders.length > 0 && (
          <div className={styles.section}>
            <h2>Responders</h2>
            <div className={styles.responders}>
              {response.responders.map((user) => (
                <div key={user.id} className={styles.responder}>
                  {user.imageUrl && (
                    <img src={user.imageUrl} alt={user.realName} className={styles.avatar} />
                  )}
                  <span>{user.realName}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {response.url && (
          <div className={styles.section}>
            <h2>URL</h2>
            <a href={response.url} target="_blank" rel="noopener noreferrer" className={styles.link}>
              {response.url}
              <ExternalLink size={16} />
            </a>
          </div>
        )}

        {response.risks && response.risks.length > 0 && (
          <div className={styles.section}>
            <h2>Related Risks</h2>
            <div className={styles.risks}>
              {response.risks.map((risk) => (
                <div
                  key={risk.id}
                  className={styles.riskCard}
                  onClick={() => handleRiskClick(risk.id)}
                >
                  <h3>{risk.name}</h3>
                  <p>{risk.description}</p>
                </div>
              ))}
            </div>
          </div>
        )}

        <div className={styles.metadata}>
          <div>
            <strong>Created:</strong>{' '}
            {new Date(response.createdAt).toLocaleString()}
          </div>
          <div>
            <strong>Updated:</strong>{' '}
            {new Date(response.updatedAt).toLocaleString()}
          </div>
        </div>
      </div>

      {isFormOpen && (
        <ResponseForm response={response} onClose={() => setIsFormOpen(false)} />
      )}

      {isDeleteDialogOpen && (
        <ResponseDeleteDialog
          response={response}
          onClose={() => setIsDeleteDialogOpen(false)}
          onConfirm={handleDeleteConfirm}
        />
      )}
    </div>
  )
}

import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@apollo/client'
import { GET_KNOWLEDGES, SEARCH_KNOWLEDGE } from '../graphql/knowledge'
import { GET_TAGS } from '../graphql/tag'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import Button from '../components/Button'
import { IconPlus } from '../components/Icons'

interface TagRef {
  id: string
  name: string | null
}

interface KnowledgeRow {
  id: string
  title: string
  claim: string
  tags: TagRef[]
  createdAt: string
  updatedAt: string
}

// tagLabel renders a tag's display text, falling back to its id when the
// (optional) name is empty.
function tagLabel(tag: TagRef): string {
  return tag.name && tag.name.length > 0 ? tag.name : tag.id
}

function formatDate(iso: string) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString()
}

// Strip Markdown to plain text for card preview
function stripMarkdown(md: string): string {
  return md
    // Remove headings
    .replace(/^#{1,6}\s+/gm, '')
    // Remove bold/italic
    .replace(/\*{1,3}([^*]+)\*{1,3}/g, '$1')
    .replace(/_{1,3}([^_]+)_{1,3}/g, '$1')
    // Remove inline code
    .replace(/`([^`]+)`/g, '$1')
    // Remove links/images
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
    // Remove blockquotes
    .replace(/^>\s+/gm, '')
    // Remove horizontal rules
    .replace(/^---+$/gm, '')
    // Flatten list bullets
    .replace(/^[*\-+]\s+/gm, '')
    .replace(/^\d+\.\s+/gm, '')
    // Collapse multiple newlines
    .replace(/\n{2,}/g, ' ')
    .replace(/\n/g, ' ')
    .trim()
}

export default function KnowledgeList() {
  const { currentWorkspace } = useWorkspace()
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [searchQuery, setSearchQuery] = useState('')
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>([])
  const [debouncedQuery, setDebouncedQuery] = useState('')

  // Debounce search input
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(searchQuery), 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  const { data: tagsData } = useQuery(GET_TAGS, {
    variables: { workspaceId: currentWorkspace?.id },
    skip: !currentWorkspace,
  })

  const { data: listData, loading: listLoading } = useQuery(GET_KNOWLEDGES, {
    variables: { workspaceId: currentWorkspace?.id, tagIds: selectedTagIds.length > 0 ? selectedTagIds : undefined },
    skip: !currentWorkspace || debouncedQuery.trim().length > 0,
  })

  const { data: searchData, loading: searchLoading } = useQuery(SEARCH_KNOWLEDGE, {
    variables: {
      workspaceId: currentWorkspace?.id,
      query: debouncedQuery,
      tagIds: selectedTagIds.length > 0 ? selectedTagIds : undefined,
      limit: 50,
    },
    skip: !currentWorkspace || debouncedQuery.trim().length === 0,
  })

  const isSearching = debouncedQuery.trim().length > 0
  const loading = isSearching ? searchLoading : listLoading
  const knowledges: KnowledgeRow[] = isSearching
    ? (searchData?.searchKnowledge ?? [])
    : (listData?.knowledges ?? [])
  const allTags: TagRef[] = tagsData?.tags ?? []

  const toggleTag = (tagId: string) => {
    setSelectedTagIds((prev) =>
      prev.includes(tagId) ? prev.filter((t) => t !== tagId) : [...prev, tagId],
    )
  }

  const isAllSelected = selectedTagIds.length === 0

  return (
    <div className="h-main-inner">
      <div className="h-page-h">
        <div>
          <h1>{t('titleKnowledge')}</h1>
          <div className="sub">{t('subtitleKnowledge')}</div>
        </div>
        <div className="actions">
          <Button
            variant="primary"
            icon={<IconPlus size={14} />}
            onClick={() => navigate(`/ws/${currentWorkspace!.id}/knowledge/new`)}
          >
            {t('btnAddKnowledge')}
          </Button>
        </div>
      </div>

      {/* Search bar */}
      <div style={{ marginBottom: 12 }}>
        <input
          type="search"
          className="h-search"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder={t('placeholderKnowledgeSearch')}
          style={{ width: '100%', maxWidth: '24rem' }}
        />
      </div>

      {/* Tag filter chips */}
      {allTags.length > 0 && (
        <div className="row" style={{ flexWrap: 'wrap', gap: 6, marginBottom: 16 }}>
          <button
            type="button"
            className={`chip${isAllSelected ? ' chip-active' : ''}`}
            onClick={() => setSelectedTagIds([])}
            style={{
              cursor: 'pointer',
              background: isAllSelected ? 'var(--accent)' : undefined,
              color: isAllSelected ? 'var(--bg-elev)' : undefined,
              border: 'none',
              fontWeight: isAllSelected ? 600 : undefined,
            }}
          >
            {t('knowledgeFilterAll')}
          </button>
          {allTags.map((tag) => {
            const active = selectedTagIds.includes(tag.id)
            return (
              <button
                key={tag.id}
                type="button"
                className="chip"
                onClick={() => toggleTag(tag.id)}
                style={{
                  cursor: 'pointer',
                  background: active ? 'var(--accent)' : undefined,
                  color: active ? 'var(--bg-elev)' : undefined,
                  border: 'none',
                  fontWeight: active ? 600 : undefined,
                }}
              >
                {tagLabel(tag)}
              </button>
            )
          })}
        </div>
      )}

      {/* Knowledge cards */}
      {loading && (
        <div className="muted" style={{ padding: 32, textAlign: 'center' }}>
          {t('loading')}
        </div>
      )}

      {!loading && knowledges.length === 0 && (
        <div style={{ padding: 48, textAlign: 'center', color: 'var(--fg-soft)' }}>
          {isSearching ? t('emptyKnowledgeSearch') : t('emptyKnowledge')}
        </div>
      )}

      {!loading && knowledges.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {knowledges.map((k) => (
            <div
              key={k.id}
              className="card"
              style={{ padding: '14px 16px', cursor: 'pointer' }}
              onClick={() => navigate(`/ws/${currentWorkspace!.id}/knowledge/${k.id}`)}
            >
              <div style={{ fontWeight: 600, fontSize: 'var(--t-base)', marginBottom: 4 }}>
                {k.title}
              </div>
              {k.claim && (
                <div
                  className="soft"
                  style={{
                    fontSize: 'var(--t-sm)',
                    display: '-webkit-box',
                    WebkitLineClamp: 2,
                    WebkitBoxOrient: 'vertical',
                    overflow: 'hidden',
                    lineHeight: 1.5,
                    marginBottom: 8,
                  }}
                >
                  {stripMarkdown(k.claim)}
                </div>
              )}
              <div className="row" style={{ flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
                {k.tags.map((tag) => (
                  <span key={tag.id} className="chip" style={{ fontSize: 'var(--t-xs)' }}>
                    {tagLabel(tag)}
                  </span>
                ))}
                <span className="spacer" />
                <span className="mono soft" style={{ fontSize: 'var(--t-xs)' }}>
                  {formatDate(k.updatedAt)}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

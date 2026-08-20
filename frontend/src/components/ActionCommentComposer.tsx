import { useState, type CSSProperties } from 'react'
import { useMutation } from '@apollo/client'
import { CREATE_ACTION_COMMENT, GET_ACTION_COMMENTS } from '../graphql/action'
import { useTranslation } from '../i18n'
import { commitOnEnter } from '../utils/keyboard'
import MarkdownEditor from './markdown/MarkdownEditor'
import Button from './Button'

interface ActionCommentComposerProps {
  workspaceId: string
  actionId: number
  /** Must match the limit the surrounding feed queries with, so the refetch
   * after a successful post replaces exactly the page already in the cache. */
  pageSize: number
}

const styles: Record<string, CSSProperties> = {
  root: { display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 },
  footer: { display: 'flex', alignItems: 'center', gap: 12 },
  hint: { fontSize: 12, color: 'var(--text-muted)' },
  error: { fontSize: 12, color: 'var(--color-error)' },
  spacer: { marginLeft: 'auto' },
}

// Composer for a new comment on an Action. Holds only the draft body; the
// posted comment is read back through the feed's own query rather than being
// merged optimistically, so the server-assigned id and timestamps are the ones
// rendered.
export default function ActionCommentComposer({ workspaceId, actionId, pageSize }: ActionCommentComposerProps) {
  const { t } = useTranslation()
  const [body, setBody] = useState('')

  const [createComment, createState] = useMutation(CREATE_ACTION_COMMENT, {
    refetchQueries: [
      { query: GET_ACTION_COMMENTS, variables: { workspaceId, id: actionId, limit: pageSize, cursor: null } },
    ],
  })

  const canSubmit = body.trim() !== '' && !createState.loading

  const handleSubmit = () => {
    if (!canSubmit) return
    void createComment({ variables: { workspaceId, input: { actionId, body } } })
      .then(() => setBody(''))
      .catch(() => {
        // Apollo surfaces the failure through createState.error, which the
        // inline message below renders. The draft is kept so the author does
        // not lose what they wrote.
      })
  }

  return (
    <div
      style={styles.root}
      data-testid="action-comment-composer"
      onKeyDown={commitOnEnter({ onCommit: handleSubmit, requireModifier: true })}
    >
      <MarkdownEditor
        value={body}
        onChange={setBody}
        disabled={createState.loading}
        placeholder={t('activityCommentPlaceholder')}
        testId="action-comment-body"
      />
      <div style={styles.footer}>
        <span style={styles.hint}>{t('activityCommentHint')}</span>
        {createState.error && (
          <span style={styles.error} role="alert" data-testid="action-comment-post-error">
            {t('errCommentPostFailed')}
          </span>
        )}
        <span style={styles.spacer} />
        <Button
          variant="primary"
          size="sm"
          onClick={handleSubmit}
          disabled={!canSubmit}
          data-testid="action-comment-submit"
        >
          {createState.loading ? t('btnSaving') : t('btnPostComment')}
        </Button>
      </div>
    </div>
  )
}

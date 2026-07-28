import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import { useNavigate } from 'react-router'
import { useWorkspace } from '../contexts/workspace-context'
import { useTranslation } from '../i18n'
import { CREATE_CASE_IMPORT } from '../graphql/import'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import Button from '../components/Button'
import {
  buildImportJsonSchema,
  buildYamlExample,
  type ImportFieldDefinition,
} from '../components/import/yamlSchema'

type DropzoneState = 'idle' | 'dragOver' | 'uploading'

/** Which input surface is shown: the file dropzone or the paste textarea. */
type InputMode = 'file' | 'paste'

export default function ImportNew() {
  const { currentWorkspace } = useWorkspace()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const [state, setState] = useState<DropzoneState>('idle')
  const [mode, setMode] = useState<InputMode>('file')
  const [pastedText, setPastedText] = useState('')
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const [createImport] = useMutation(CREATE_CASE_IMPORT)
  const [schemaCopied, setSchemaCopied] = useState(false)

  // Pull the workspace's field configuration so the JSON Schema panel
  // can describe the per-Case `fields` block exactly (allowed keys,
  // types, enums, required-ness) instead of falling back to an empty
  // `additionalProperties: true` stub.
  const { data: fieldConfigData } = useQuery<{
    fieldConfiguration: { fields: ImportFieldDefinition[] }
  }>(GET_FIELD_CONFIGURATION, {
    variables: { workspaceId: currentWorkspace?.id ?? '' },
    skip: !currentWorkspace,
  })
  const fields = useMemo(
    () => fieldConfigData?.fieldConfiguration.fields ?? [],
    [fieldConfigData],
  )
  const jsonSchema = useMemo(() => buildImportJsonSchema(fields), [fields])
  const yamlExample = useMemo(() => buildYamlExample(fields), [fields])

  // Claimed for the whole submit, from the first byte read off the File
  // to the navigate. A ref rather than `state` because reading a File is
  // async: between the drop and the `uploading` re-render, a second drop
  // or a page-level paste would otherwise start a competing
  // createCaseImport and the page would land on whichever finished first.
  const submittingRef = useRef(false)

  const beginSubmit = useCallback(() => {
    if (submittingRef.current || !currentWorkspace) return false
    submittingRef.current = true
    setState('uploading')
    setErrorMessage(null)
    return true
  }, [currentWorkspace])

  const failSubmit = useCallback((e: unknown) => {
    submittingRef.current = false
    setErrorMessage(e instanceof Error ? e.message : String(e))
    setState('idle')
  }, [])

  // Both entry points (file and paste) funnel through here, with the
  // submit already claimed by beginSubmit. The mutation input carries
  // `originalFileName` only when the content really came from a file —
  // for pasted YAML the field is omitted rather than sent as null (see
  // .claude/rules/graphql-schema.md on optional inputs).
  const submitContent = useCallback(
    async (content: string, originalFileName?: string) => {
      const workspaceId = currentWorkspace?.id
      if (!workspaceId) return
      try {
        const res = await createImport({
          variables: {
            workspaceId,
            input: originalFileName ? { content, originalFileName } : { content },
          },
        })
        const id = res.data?.createCaseImport?.id as string | undefined
        if (!id) {
          throw new Error('createCaseImport returned no id')
        }
        navigate(`/ws/${workspaceId}/imports/${id}`)
      } catch (e) {
        failSubmit(e)
      }
    },
    [createImport, currentWorkspace, failSubmit, navigate],
  )

  const handleFiles = useCallback(
    (files: FileList | File[] | null) => {
      if (!files || files.length === 0) return
      if (!beginSubmit()) return
      const f = files[0]
      void (async () => {
        try {
          const content = await f.text()
          await submitContent(content, f.name)
        } catch (e) {
          // Reading the File itself can fail (permissions, removed
          // media); surface it the same way a failed upload is surfaced.
          failSubmit(e)
        }
      })()
    },
    [beginSubmit, failSubmit, submitContent],
  )

  // The pasted text is submitted verbatim: trimming would strip trailing
  // newlines that a `|+` block scalar keeps as part of its value, so the
  // same YAML would import differently depending on whether it arrived
  // as a file or as a paste. Only the empty check ignores whitespace.
  const submitPastedText = useCallback(() => {
    if (pastedText.trim().length === 0) return
    if (!beginSubmit()) return
    void submitContent(pastedText)
  }, [beginSubmit, pastedText, submitContent])

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setState('idle')
    handleFiles(e.dataTransfer.files)
  }
  const handleDragOver = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    if (state === 'uploading') return
    setState('dragOver')
  }
  const handleDragLeave = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    if (state === 'uploading') return
    setState('idle')
  }

  // Paste anywhere on the page (⌘V / Ctrl+V) lands in the paste box.
  // The listener sits on `document` because a page-level paste targets
  // <body>, which is outside this component's React tree — an onPaste
  // prop on the wrapper would never see it. Pastes that already have a
  // home (the textarea, any input) are left alone, and nothing is
  // submitted automatically: the user still confirms.
  useEffect(() => {
    const onDocumentPaste = (e: ClipboardEvent) => {
      if (submittingRef.current) return
      const target = e.target
      if (
        target instanceof HTMLElement &&
        target.closest('input, textarea, [contenteditable="true"]')
      ) {
        return
      }
      const files = e.clipboardData?.files
      if (files && files.length > 0) {
        e.preventDefault()
        handleFiles(files)
        return
      }
      const text = e.clipboardData?.getData('text/plain') ?? ''
      if (!text.trim()) return
      e.preventDefault()
      setMode('paste')
      setPastedText(text)
      setErrorMessage(null)
    }
    document.addEventListener('paste', onDocumentPaste)
    return () => document.removeEventListener('paste', onDocumentPaste)
  }, [handleFiles])

  // Focus the textarea once it is mounted, so a page-level paste (or a
  // click on the Paste tab) leaves the caret where editing continues.
  useEffect(() => {
    if (mode === 'paste') textareaRef.current?.focus()
  }, [mode])

  const copySchema = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(jsonSchema, null, 2))
      setSchemaCopied(true)
      setTimeout(() => setSchemaCopied(false), 1800)
    } catch {
      // clipboard API not available — fall through silently.
    }
  }, [jsonSchema])

  // Drop zone visuals. The base border uses --line-strong (a real token,
  // not the optional --border-strong) so the dashed outline is visible
  // against the white page background; --border-default alone is too
  // faint to read as a dashed boundary.
  const border =
    state === 'dragOver'
      ? 'var(--color-primary, var(--accent))'
      : 'var(--line-strong)'
  const background =
    state === 'dragOver'
      ? 'color-mix(in oklch, var(--color-primary, var(--accent)) 6%, var(--bg-paper))'
      : state === 'uploading'
        ? 'var(--bg-paper)'
        : 'var(--bg-subtle)'

  // Shared styles for the two code blocks below.
  const codeBlockStyle: React.CSSProperties = {
    margin: 0,
    padding: 14,
    background: 'var(--bg-sunken, var(--bg-subtle))',
    border: '1px solid var(--line-strong, var(--border-default))',
    borderRadius: 6,
    fontSize: 11.5,
    lineHeight: 1.6,
    overflow: 'auto',
    maxHeight: 320,
    color: 'var(--fg, var(--text-body))',
    whiteSpace: 'pre',
  }
  const cardLabelStyle: React.CSSProperties = {
    fontSize: 11,
    fontWeight: 600,
    textTransform: 'uppercase',
    letterSpacing: 0.08,
    color: 'var(--text-muted)',
  }

  return (
    <div className="h-main-inner" style={{ maxWidth: 720 }}>
      <div className="col" style={{ gap: 4, marginBottom: 22 }}>
        <h1 style={{ margin: 0, fontSize: 22, fontWeight: 600, letterSpacing: -0.015 }}>
          {t('titleImportNew')}
        </h1>
        <p
          style={{
            margin: 0,
            color: 'var(--text-muted, var(--text-body))',
            fontSize: 13,
            lineHeight: 1.6,
          }}
        >
          {t('subtitleImportNew')}
        </p>
      </div>

      <div className="seg" style={{ marginBottom: 12 }} role="group">
        <button
          type="button"
          className={mode === 'file' ? 'on' : ''}
          aria-pressed={mode === 'file'}
          data-testid="import-mode-file"
          disabled={state === 'uploading'}
          onClick={() => setMode('file')}
        >
          {t('importModeFile')}
        </button>
        <button
          type="button"
          className={mode === 'paste' ? 'on' : ''}
          aria-pressed={mode === 'paste'}
          data-testid="import-mode-paste"
          disabled={state === 'uploading'}
          onClick={() => setMode('paste')}
        >
          {t('importModePaste')}
        </button>
      </div>

      {mode === 'paste' ? (
        <div className="col" style={{ gap: 10 }}>
          <textarea
            ref={textareaRef}
            className="mono"
            data-testid="import-paste-textarea"
            value={pastedText}
            onChange={(e) => setPastedText(e.target.value)}
            placeholder={t('importPastePlaceholder')}
            aria-label={t('importModePaste')}
            spellCheck={false}
            disabled={state === 'uploading'}
            style={{
              width: '100%',
              minHeight: 260,
              resize: 'vertical',
              padding: 14,
              borderRadius: 12,
              border: '1px solid var(--line-strong, var(--border-default))',
              background: 'var(--bg-paper)',
              color: 'var(--fg, var(--text-body))',
              fontSize: 12.5,
              lineHeight: 1.6,
              tabSize: 2,
            }}
          />
          <div
            className="row"
            style={{ justifyContent: 'space-between', alignItems: 'center', gap: 12 }}
          >
            <span
              className="soft"
              style={{ fontSize: 12.5, color: 'var(--text-muted, var(--text-body))' }}
            >
              {t('importPasteHint')}
            </span>
            <Button
              variant="primary"
              data-testid="import-paste-submit"
              onClick={submitPastedText}
              disabled={pastedText.trim().length === 0 || state === 'uploading'}
            >
              {state === 'uploading' ? t('importValidating') : t('importPasteSubmit')}
            </Button>
          </div>
        </div>
      ) : (
        <div
          role="button"
          tabIndex={0}
          onClick={() => state !== 'uploading' && fileInputRef.current?.click()}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
          aria-label={t('importDropPrompt')}
          style={{
            border: `2px dashed ${border}`,
            background,
            borderRadius: 12,
            padding: '48px 24px',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 14,
            textAlign: 'center',
            transition: 'background .15s, border-color .15s',
            cursor: state === 'uploading' ? 'default' : 'pointer',
          }}
        >
          <input
            ref={fileInputRef}
            type="file"
            accept=".yaml,.yml"
            style={{ display: 'none' }}
            onChange={(e) => handleFiles(e.target.files)}
          />
          {/* Upload icon (round) */}
          <span
            aria-hidden
            style={{
              width: 52,
              height: 52,
              borderRadius: 99,
              background: 'var(--bg-elev, var(--bg-paper))',
              border: '1px solid var(--line-strong, var(--border-default))',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              color:
                state === 'uploading'
                  ? 'var(--color-primary, var(--accent))'
                  : 'var(--text-muted)',
              fontSize: 24,
              lineHeight: 1,
            }}
          >
            ⬆
          </span>
          <div style={{ fontSize: 15, fontWeight: 500 }}>
            {state === 'uploading'
              ? t('importValidating')
              : state === 'dragOver'
                ? t('importDropPromptHover')
                : t('importDropPrompt')}
          </div>
          <div
            className="soft"
            style={{ fontSize: 12.5, color: 'var(--text-muted, var(--text-body))' }}
          >
            {state === 'uploading' ? t('importDropAccepted') : t('importChooseLink')}
            {' · '}
            <span className="mono">.yaml</span> <span className="mono">.yml</span>
          </div>
        </div>
      )}

      {errorMessage && (
        <div
          style={{
            marginTop: 16,
            padding: '10px 12px',
            borderRadius: 6,
            background:
              'color-mix(in oklch, var(--color-error, #b91c1c) 10%, var(--bg-paper))',
            border:
              '1px solid color-mix(in oklch, var(--color-error, #b91c1c) 30%, var(--border-default))',
            color: 'var(--color-error, #b91c1c)',
            fontSize: 13,
          }}
        >
          {t('importSubmitError')}
          {errorMessage}
        </div>
      )}

      {/* YAML schema reference. The schema and the example live in two
          clearly separated cards so users do not confuse the JSON Schema
          (machine-readable contract, copyable) with the YAML example
          (what they actually paste into their editor). */}
      <details style={{ marginTop: 24 }}>
        <summary
          style={{
            fontSize: 13,
            color: 'var(--text-muted, var(--text-body))',
            cursor: 'pointer',
            userSelect: 'none',
            padding: '4px 0',
          }}
        >
          {t('importShowSchema')}
        </summary>

        {/* JSON Schema card */}
        <section
          style={{
            marginTop: 12,
            padding: 14,
            background: 'var(--bg-paper)',
            border: '1px solid var(--line-strong, var(--border-default))',
            borderRadius: 8,
          }}
        >
          <div
            className="row"
            style={{
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: 8,
              marginBottom: 10,
            }}
          >
            <span style={cardLabelStyle}>JSON Schema</span>
            <Button variant="ghost" size="sm" onClick={copySchema}>
              {schemaCopied ? t('importShowSchemaCopied') : t('importShowSchemaCopy')}
            </Button>
          </div>
          <pre className="mono" style={codeBlockStyle}>
            {JSON.stringify(jsonSchema, null, 2)}
          </pre>
        </section>

        {/* YAML example card */}
        <section
          style={{
            marginTop: 12,
            padding: 14,
            background: 'var(--bg-paper)',
            border: '1px solid var(--line-strong, var(--border-default))',
            borderRadius: 8,
          }}
        >
          <div style={{ marginBottom: 10 }}>
            <span style={cardLabelStyle}>{t('importSchemaExampleLabel')}</span>
          </div>
          <pre className="mono" style={{ ...codeBlockStyle, maxHeight: 280 }}>
            {yamlExample}
          </pre>
        </section>
      </details>

      <div className="row" style={{ marginTop: 22, justifyContent: 'flex-end', gap: 8 }}>
        <Button
          variant="ghost"
          onClick={() => navigate(`/ws/${currentWorkspace?.id ?? ''}/cases`)}
          disabled={state === 'uploading'}
        >
          {t('btnCancel')}
        </Button>
      </div>
    </div>
  )
}

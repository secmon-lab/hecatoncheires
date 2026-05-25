import { useCallback, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client'
import { useNavigate } from 'react-router-dom'
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

export default function ImportNew() {
  const { currentWorkspace } = useWorkspace()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [state, setState] = useState<DropzoneState>('idle')
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

  const uploadFile = useCallback(
    async (file: File) => {
      if (!currentWorkspace) return
      setState('uploading')
      setErrorMessage(null)
      try {
        const content = await file.text()
        const res = await createImport({
          variables: {
            workspaceId: currentWorkspace.id,
            input: {
              content,
              originalFileName: file.name,
            },
          },
        })
        const id = res.data?.createCaseImport?.id as string | undefined
        if (!id) {
          throw new Error('createCaseImport returned no id')
        }
        navigate(`/ws/${currentWorkspace.id}/imports/${id}`)
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e)
        setErrorMessage(msg)
        setState('idle')
      }
    },
    [createImport, currentWorkspace, navigate],
  )

  const handleFiles = useCallback(
    (files: FileList | null) => {
      if (!files || files.length === 0) return
      const f = files[0]
      void uploadFile(f)
    },
    [uploadFile],
  )

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

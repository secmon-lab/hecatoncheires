import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { IconChevDown } from './Icons'
import { workspaceVisual, type WorkspaceVisualInput } from '../utils/workspace'

interface Workspace {
  id: string
  name: string
  emoji?: string | null
  color?: string | null
}

interface Props {
  current: Workspace | null
  workspaces: Workspace[]
}

function WorkspaceBadge({ ws }: { ws: WorkspaceVisualInput }) {
  const v = workspaceVisual(ws)
  return (
    <span
      className={v.kind === 'emoji' ? 'h-ws-mark is-emoji' : 'h-ws-mark'}
      style={{ background: v.background }}
    >
      {v.kind === 'emoji' ? v.emoji : v.mark}
    </span>
  )
}

export default function WorkspaceSwitcher({ current, workspaces }: Props) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [open])

  if (!current) return null

  const mark = <WorkspaceBadge ws={current} />

  if (workspaces.length <= 1) {
    return (
      <button type="button" className="h-ws" tabIndex={-1} style={{ cursor: 'default' }}>
        {mark}
        <span>{current.name}</span>
      </button>
    )
  }

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button type="button" className="h-ws" onClick={() => setOpen((v) => !v)}>
        {mark}
        <span>{current.name}</span>
        <IconChevDown size={14} />
      </button>
      {open && (
        <div
          style={{
            position: 'absolute', top: 'calc(100% + 6px)', left: 0,
            minWidth: 220, padding: 4,
            background: 'var(--bg-elev)', border: '1px solid var(--line)',
            borderRadius: 'var(--radius-md)', boxShadow: 'var(--shadow-lg)',
            zIndex: 1001,
          }}
        >
          {workspaces.map((ws) => (
            <Link
              key={ws.id}
              to={`/ws/${ws.id}/cases`}
              onClick={() => setOpen(false)}
              style={{
                display: 'flex', alignItems: 'center', gap: 8,
                padding: '8px 10px', borderRadius: 4,
                fontSize: 13, color: 'var(--fg)', textDecoration: 'none',
                background: ws.id === current.id ? 'var(--side-active-bg)' : undefined,
                fontWeight: ws.id === current.id ? 600 : 500,
              }}
            >
              <WorkspaceBadge ws={ws} />
              <span>{ws.name}</span>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}

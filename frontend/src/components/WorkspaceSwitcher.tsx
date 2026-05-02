import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { IconChevDown } from './Icons'

interface Workspace {
  id: string
  name: string
}

interface Props {
  current: Workspace | null
  workspaces: Workspace[]
}

function workspaceMark(name: string): string {
  const trimmed = (name || '').trim()
  if (!trimmed) return '?'
  const parts = trimmed.split(/\s+/)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return trimmed.slice(0, 2).toUpperCase()
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

  const mark = <span className="h-ws-mark">{workspaceMark(current.name)}</span>

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
              <span className="h-ws-mark">{workspaceMark(ws.name)}</span>
              <span>{ws.name}</span>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}

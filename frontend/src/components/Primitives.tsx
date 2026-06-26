import type { CSSProperties, ReactNode } from 'react'
import { IconLock, IconSlack, IconFlask } from './Icons'
import { displayName } from '../utils/user'

/* ─── Avatar ─── */
type AvatarSize = '' | 'sm' | 'lg' | 'xl'

interface AvatarProps {
  name?: string | null
  realName?: string | null
  imageUrl?: string | null
  size?: AvatarSize
  title?: string
}

function avatarHue(seed: string) {
  let h = 0
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0
  return h % 360
}

export function Avatar({ name, realName, imageUrl, size = '', title }: AvatarProps) {
  const display = (realName || name || '?').trim() || '?'
  const initial = display.slice(0, 1).toUpperCase()
  const cls = size ? `h-avatar ${size}` : 'h-avatar'

  if (imageUrl) {
    return (
      <span
        className={cls}
        style={{
          background: 'transparent',
          backgroundImage: `url("${imageUrl}")`,
          backgroundSize: 'cover',
          backgroundPosition: 'center',
        }}
        title={title || display}
        aria-label={display}
      />
    )
  }

  const hue1 = avatarHue(display)
  const hue2 = (hue1 + 80) % 360
  const style: CSSProperties = {
    background: `linear-gradient(135deg, oklch(0.65 0.16 ${hue1}), oklch(0.5 0.18 ${hue2}))`,
  }
  return (
    <span className={cls} style={style} title={title || display} aria-label={display}>
      {initial}
    </span>
  )
}

interface AvatarStackProps {
  users: Array<{ id: string; name?: string | null; realName?: string | null; imageUrl?: string | null }>
  max?: number
  size?: AvatarSize
}

export function AvatarStack({ users, max = 3, size = 'sm' }: AvatarStackProps) {
  if (!users || users.length === 0) return null
  const shown = users.slice(0, max)
  const overflow = users.length - shown.length
  return (
    <div className="h-avatar-stack">
      {shown.map((u) => (
        <Avatar key={u.id} name={u.name} realName={u.realName} imageUrl={u.imageUrl} size={size} />
      ))}
      {overflow > 0 && (
        <span
          className={size ? `h-avatar ${size}` : 'h-avatar'}
          style={{ background: 'var(--bg-sunken)', color: 'var(--fg-muted)' }}
        >
          +{overflow}
        </span>
      )}
    </div>
  )
}

interface AssigneeNamesStackProps {
  users: Array<{ id: string; name?: string | null; realName?: string | null; imageUrl?: string | null }>
  max?: number
  maxLabelWidth?: number
  testId?: string
}

export function AssigneeNamesStack({
  users,
  max = 3,
  maxLabelWidth = 100,
  testId,
}: AssigneeNamesStackProps) {
  if (!users || users.length === 0) return <span className="soft">—</span>
  const visible = users.slice(0, max)
  const overflow = users.length - visible.length
  return (
    <div
      className="col"
      style={{ gap: 2, alignItems: 'flex-start' }}
      data-testid={testId}
    >
      {visible.map((u) => (
        <div key={u.id} className="row" style={{ gap: 6, fontSize: 12 }}>
          <Avatar size="sm" name={u.name} realName={u.realName} imageUrl={u.imageUrl} />
          <span className="truncate" style={{ maxWidth: maxLabelWidth }}>{displayName(u)}</span>
        </div>
      ))}
      {overflow > 0 && (
        <span className="soft" style={{ fontSize: 11 }}>+{overflow}</span>
      )}
    </div>
  )
}

/* ─── Badge ─── */
type BadgeKind = 'neutral' | 'open' | 'closed' | 'private' | 'danger' | 'info' | 'ai'
interface BadgeProps {
  kind?: BadgeKind
  dot?: boolean
  children: ReactNode
}

export function Badge({ kind = 'neutral', dot = true, children }: BadgeProps) {
  const cls = kind === 'neutral' ? 'badge' : `badge ${kind}`
  return (
    <span className={cls}>
      {dot && <span className="dot" />}
      {children}
    </span>
  )
}

interface StatusBadgeProps {
  status: 'OPEN' | 'CLOSED' | 'DRAFT' | string
  labelOpen?: string
  labelClosed?: string
  labelDraft?: string
}

export function StatusBadge({ status, labelOpen = 'Open', labelClosed = 'Closed', labelDraft = 'Draft' }: StatusBadgeProps) {
  if (status === 'OPEN') return <Badge kind="open">{labelOpen}</Badge>
  if (status === 'CLOSED') return <Badge kind="closed">{labelClosed}</Badge>
  if (status === 'DRAFT') return <Badge>{labelDraft}</Badge>
  return <Badge>{status}</Badge>
}

export function PrivateBadge({ label = 'Private' }: { label?: string }) {
  return (
    <span className="badge private">
      <IconLock size={11} sw={2} />
      {label}
    </span>
  )
}

export function TestBadge({ label = 'Test' }: { label?: string }) {
  return (
    <span className="badge info">
      <IconFlask size={11} sw={2} />
      {label}
    </span>
  )
}

/* ─── Status pip (Action) ─── */
const STATUS_PIP_CLASS: Record<string, string> = {
  BACKLOG: 'pip-bg',
  TODO: 'pip-todo',
  IN_PROGRESS: 'pip-prog',
  BLOCKED: 'pip-block',
  COMPLETED: 'pip-done',
}

export function StatusPip({ status }: { status: string }) {
  const cls = STATUS_PIP_CLASS[status] || 'pip-bg'
  return (
    <span
      className={`pip ${cls}`}
      style={{ width: 8, height: 8, borderRadius: '50%', display: 'inline-block', flexShrink: 0 }}
    />
  )
}

/* ─── Slack channel link pill ─── */
interface SlackLinkProps {
  name: string
  href?: string
  ext?: boolean
}

export function SlackLink({ name, href, ext }: SlackLinkProps) {
  const iconOnly = !name
  return (
    <a
      className={`slack-link${iconOnly ? ' icon-only' : ''}`}
      href={href || '#'}
      aria-label={iconOnly ? 'Slack channel' : undefined}
      target={href ? '_blank' : undefined}
      rel={href ? 'noreferrer noopener' : undefined}
      onClick={(e) => {
        if (!href) e.preventDefault()
      }}
    >
      <IconSlack size={iconOnly ? 14 : 11} />
      {name ? <>#{name}</> : <span style={{ fontFamily: 'inherit', fontWeight: 500 }}>Slack</span>}
      {(ext || iconOnly) && (
        <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M14 4h6v6M10 14L20 4" />
        </svg>
      )}
    </a>
  )
}

/* ─── Generic placeholder ─── */
export function Placeholder({ label, h = 80 }: { label: string; h?: number }) {
  return (
    <div className="placeholder" style={{ height: h, width: '100%' }}>
      {label}
    </div>
  )
}

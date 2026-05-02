import { Avatar } from '../Primitives'
import { IconExt } from '../Icons'

interface FieldOption {
  id: string
  name: string
  color?: string
}

interface FieldDef {
  id: string
  name: string
  type: string
  options?: FieldOption[]
}

interface User {
  id: string
  name: string
  realName: string
  imageUrl?: string | null
}

interface Props {
  field: FieldDef
  value: any
  users?: User[]
}

// Read-only display for a custom field value, used in the case detail
// sidebar. Mirrors the design's flat label / value layout (no input chrome).
export default function FieldDisplay({ field, value, users = [] }: Props) {
  if (value === undefined || value === null || value === '' || (Array.isArray(value) && value.length === 0)) {
    return <span className="soft" style={{ fontSize: 13 }}>—</span>
  }

  switch (field.type) {
    case 'TEXT':
      return <span style={{ fontSize: 13, whiteSpace: 'pre-wrap' }}>{String(value)}</span>

    case 'NUMBER':
      return <span className="mono" style={{ fontSize: 13 }}>{String(value)}</span>

    case 'DATE': {
      const d = new Date(String(value))
      const label = Number.isNaN(d.getTime())
        ? String(value)
        : d.toLocaleDateString()
      return <span className="mono" style={{ fontSize: 13 }}>{label}</span>
    }

    case 'URL':
      return (
        <a
          href={String(value)}
          target="_blank"
          rel="noreferrer noopener"
          style={{ color: 'var(--accent)', fontSize: 13 }}
        >
          {String(value)}
          <IconExt size={10} style={{ verticalAlign: -1, marginLeft: 4 }} />
        </a>
      )

    case 'SELECT': {
      const opt = field.options?.find((o) => o.id === value)
      return (
        <span style={{ fontSize: 13 }}>
          <span
            className="pip"
            style={{
              display: 'inline-block', width: 6, height: 6, borderRadius: '50%',
              background: opt?.color || 'var(--accent)', marginRight: 6, verticalAlign: 'middle',
            }}
          />
          {opt ? opt.name : String(value)}
        </span>
      )
    }

    case 'MULTI_SELECT': {
      const ids: string[] = Array.isArray(value) ? value : []
      const opts = ids
        .map((id) => field.options?.find((o) => o.id === id))
        .filter(Boolean) as FieldOption[]
      if (opts.length === 0) {
        return <span style={{ fontSize: 13 }}>{ids.join(', ')}</span>
      }
      return (
        <div className="row" style={{ gap: 4, flexWrap: 'wrap' }}>
          {opts.map((o) => (
            <span
              key={o.id}
              className="chip"
              style={{
                fontSize: 11, padding: '1px 8px', borderRadius: 999,
                border: '1px solid var(--line)', background: 'var(--bg-sunken)',
              }}
            >
              {o.name}
            </span>
          ))}
        </div>
      )
    }

    case 'USER': {
      const u = users.find((x) => x.id === value)
      if (!u) return <span className="mono" style={{ fontSize: 13 }}>{String(value)}</span>
      return (
        <div className="row" style={{ gap: 6, alignItems: 'center' }}>
          <Avatar size="sm" name={u.name} realName={u.realName} imageUrl={u.imageUrl} />
          <span style={{ fontSize: 13 }}>{u.realName || u.name}</span>
        </div>
      )
    }

    case 'MULTI_USER': {
      const ids: string[] = Array.isArray(value) ? value : []
      const found = ids.map((id) => users.find((u) => u.id === id)).filter(Boolean) as User[]
      if (found.length === 0) {
        return <span style={{ fontSize: 13 }}>{ids.join(', ')}</span>
      }
      return (
        <div className="col" style={{ gap: 4 }}>
          {found.map((u) => (
            <div key={u.id} className="row" style={{ gap: 6, alignItems: 'center' }}>
              <Avatar size="sm" name={u.name} realName={u.realName} imageUrl={u.imageUrl} />
              <span style={{ fontSize: 13 }}>{u.realName || u.name}</span>
            </div>
          ))}
        </div>
      )
    }

    default:
      return <span style={{ fontSize: 13 }}>{String(value)}</span>
  }
}

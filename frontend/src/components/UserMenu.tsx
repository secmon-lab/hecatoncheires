import { useEffect, useRef, useState } from 'react'
import { useAuth } from '../contexts/auth-context'
import { useTranslation } from '../i18n'
import type { Lang } from '../i18n'
import { Avatar } from './Primitives'

export function UserMenu() {
  const { user, logout } = useAuth()
  const { t, lang, setLang } = useTranslation()
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

  if (!user) return null

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button
        type="button"
        className="h-icon-btn"
        style={{ width: 30, height: 30, padding: 0 }}
        onClick={() => setOpen((v) => !v)}
        aria-label={t('ariaUserMenu')}
      >
        <Avatar size="sm" name={user.name} />
      </button>
      {open && (
        <div
          style={{
            position: 'absolute', top: 'calc(100% + 6px)', right: 0,
            minWidth: 220,
            background: 'var(--bg-elev)', border: '1px solid var(--line)',
            borderRadius: 'var(--radius-md)', boxShadow: 'var(--shadow-lg)',
            zIndex: 1001, overflow: 'hidden',
          }}
        >
          <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--line)' }}>
            <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--fg)' }}>{user.name}</div>
            <div style={{ fontSize: 11, color: 'var(--fg-muted)', marginTop: 2 }}>{user.email}</div>
          </div>
          <div style={{ padding: '8px 14px' }}>
            <div style={{ fontSize: 11, color: 'var(--fg-soft)', marginBottom: 6 }}>Language</div>
            <div className="seg" style={{ width: '100%' }}>
              {(['en', 'ja'] as Lang[]).map((l) => (
                <button
                  key={l}
                  className={lang === l ? 'on' : ''}
                  style={{ flex: 1 }}
                  onClick={() => setLang(l)}
                >
                  {l.toUpperCase()}
                </button>
              ))}
            </div>
          </div>
          <button
            type="button"
            onClick={() => { setOpen(false); logout() }}
            style={{
              width: '100%', textAlign: 'left',
              padding: '10px 14px', border: 'none', background: 'transparent',
              color: 'var(--fg)', fontSize: 13, fontFamily: 'inherit',
              cursor: 'pointer', borderTop: '1px solid var(--line)',
            }}
          >
            {t('btnLogout')}
          </button>
        </div>
      )}
    </div>
  )
}

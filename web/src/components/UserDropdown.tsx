import { useEffect, useRef, type RefObject } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { LogOut, Shield } from 'lucide-react'
import { useSession } from '../session'

interface UserDropdownProps {
  id: string
  onClose: () => void
  triggerRef: RefObject<HTMLButtonElement | null>
}

export default function UserDropdown({ id, onClose, triggerRef }: UserDropdownProps) {
  const { user, logout } = useSession()
  const navigate = useNavigate()
  const dropdownRef = useRef<HTMLDivElement>(null)
  const isAdmin = user?.is_admin === true

  useEffect(() => {
    function handleMouseDown(e: MouseEvent) {
      const target = e.target as Node
      if (dropdownRef.current?.contains(target) || triggerRef.current?.contains(target)) return
      onClose()
    }
    document.addEventListener('mousedown', handleMouseDown)
    return () => document.removeEventListener('mousedown', handleMouseDown)
  }, [onClose, triggerRef])

  return (
    <div
      id={id}
      role="menu"
      ref={dropdownRef}
      style={{
        position: 'absolute',
        top: 'calc(100% + 6px)',
        right: 0,
        width: 210,
        background: '#0D0D0D',
        border: '1px solid #2a2a2a',
        zIndex: 200,
        boxShadow: '0 8px 32px rgba(0,0,0,0.65)',
      }}
    >
      {/* User identity */}
      <div style={{
        padding: '12px 16px',
        borderBottom: '1px solid #1e1e1e',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
      }}>
        <span style={{ fontSize: 11, color: 'var(--text)', letterSpacing: '0.08em' }}>
          {user?.username ?? 'USER'}
        </span>
        {isAdmin && (
          <span style={{
            fontSize: 9,
            color: 'var(--accent)',
            letterSpacing: '0.14em',
            border: '1px solid var(--accent)',
            padding: '1px 5px',
            lineHeight: '1.6',
          }}>
            ADMIN
          </span>
        )}
      </div>

      {/* User items */}
      <div style={{ padding: '6px 0', borderBottom: '1px solid #1e1e1e' }}>
        <NavLink to="/account" onClick={onClose} className="dropdown-item">
          ACCOUNT
        </NavLink>
        <NavLink to="/diagnostics" onClick={onClose} className="dropdown-item">
          DIAGNOSTICS
        </NavLink>
      </div>

      {/* Admin section */}
      {isAdmin && (
        <div style={{ borderBottom: '1px solid #1e1e1e' }}>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            padding: '10px 16px 6px',
          }}>
            <Shield size={11} style={{ color: 'var(--accent)', flexShrink: 0 }} />
            <span style={{ fontSize: 9, color: 'var(--accent)', letterSpacing: '0.16em' }}>
              ADMIN
            </span>
          </div>
          <NavLink to="/admin" onClick={onClose} className="dropdown-item" style={{ paddingTop: 6 }}>
            ADMIN PANEL
          </NavLink>
          <NavLink to="/webhooks" onClick={onClose} className="dropdown-item" style={{ paddingBottom: 8 }}>
            WEBHOOKS
          </NavLink>
        </div>
      )}

      {/* Logout */}
      <div style={{ padding: '6px 0' }}>
        <button
          type="button"
          className="dropdown-item dropdown-item-danger"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            border: 'none',
            width: '100%',
            fontFamily: 'inherit',
            textAlign: 'left',
          }}
          onClick={() => {
            void logout()
              .then(() => navigate('/login'))
              .catch(err => {
                console.error('logout:', err)
              })
          }}
        >
          <LogOut size={14} />
          LOGOUT
        </button>
      </div>
    </div>
  )
}

import { useEffect, useRef, type RefObject } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { LogOut } from 'lucide-react'
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
        top: 'calc(100% + 4px)',
        right: 0,
        width: 180,
        background: '#0D0D0D',
        border: '1px solid #242424',
        zIndex: 200,
        boxShadow: '0 8px 24px rgba(0,0,0,0.5)',
      }}
    >
      <div style={{ padding: '10px 16px 8px', borderBottom: '1px solid #1a1a1a' }}>
        <span style={{ fontSize: 11, color: 'var(--text)', letterSpacing: '0.08em' }}>
          {user?.username ?? 'USER'}
        </span>
      </div>

      <NavLink to="/account" onClick={onClose} className="dropdown-item">
        ACCOUNT
      </NavLink>

      {isAdmin && (
        <>
          <div style={{ padding: '8px 16px 2px', fontSize: 10, color: '#333', letterSpacing: '0.12em' }}>
            ADMIN
          </div>
          <NavLink to="/admin" onClick={onClose} className="dropdown-item">
            ADMIN PANEL
          </NavLink>
          <NavLink to="/webhooks" onClick={onClose} className="dropdown-item">
            WEBHOOKS
          </NavLink>
        </>
      )}

      <NavLink to="/diagnostics" onClick={onClose} className="dropdown-item">
        DIAGNOSTICS
      </NavLink>

      <div style={{ borderTop: '1px solid #1a1a1a', padding: '4px 0' }}>
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

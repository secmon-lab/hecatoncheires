import { useState, useEffect } from 'react'
import { useIsDesktop } from './useMediaQuery'

export function useSidebarState() {
  const isDesktop = useIsDesktop()
  const [isOpen, setIsOpen] = useState(false)

  // Auto-close sidebar when switching to mobile/tablet
  useEffect(() => {
    if (!isDesktop) {
      setIsOpen(false)
    }
  }, [isDesktop])

  const toggle = () => setIsOpen(!isOpen)
  const close = () => setIsOpen(false)
  const open = () => setIsOpen(true)

  return {
    isOpen: isDesktop || isOpen,
    toggle,
    close,
    open,
    isMobileMenuOpen: !isDesktop && isOpen,
  }
}

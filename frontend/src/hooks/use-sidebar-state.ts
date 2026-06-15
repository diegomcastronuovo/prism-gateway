'use client'

import { useState, useEffect } from 'react'

export function useSidebarState() {
  const [isCollapsed, setIsCollapsed] = useState(false)

  useEffect(() => {
    const stored = localStorage.getItem('sidebar-collapsed')
    if (stored) {
      setIsCollapsed(stored === 'true')
    }
  }, [])

  const toggle = () => {
    const newState = !isCollapsed
    setIsCollapsed(newState)
    localStorage.setItem('sidebar-collapsed', String(newState))
  }

  return { isCollapsed, toggle }
}

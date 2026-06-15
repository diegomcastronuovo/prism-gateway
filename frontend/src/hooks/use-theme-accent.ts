'use client'

import { useState, useEffect } from 'react'
import { ThemeAccent, applyThemeAccent, getStoredAccent, setStoredAccent } from '@/lib/themes/theme-config'

export function useThemeAccent() {
  const [accent, setAccent] = useState<ThemeAccent>('blue')

  useEffect(() => {
    const stored = getStoredAccent()
    setAccent(stored)
    applyThemeAccent(stored)
  }, [])

  const changeAccent = (newAccent: ThemeAccent) => {
    setAccent(newAccent)
    setStoredAccent(newAccent)
    applyThemeAccent(newAccent)
  }

  return { accent, setAccent: changeAccent }
}

export type ThemeAccent = 'magenta' | 'blue' | 'violet' | 'green'

export const themeAccents: Record<ThemeAccent, { primary: string; ring: string }> = {
  // Magenta accent (#E6399B)
  magenta: {
    primary: '326 78% 56%',
    ring: '326 78% 56%',
  },
  blue: {
    primary: '221.2 83.2% 53.3%',
    ring: '221.2 83.2% 53.3%',
  },
  violet: {
    primary: '262.1 83.3% 57.8%',
    ring: '262.1 83.3% 57.8%',
  },
  green: {
    primary: '142.1 76.2% 36.3%',
    ring: '142.1 76.2% 36.3%',
  },
}

export function applyThemeAccent(accent: ThemeAccent) {
  const root = document.documentElement
  const colors = themeAccents[accent]

  root.style.setProperty('--primary', colors.primary)
  root.style.setProperty('--ring', colors.ring)
}

export function getStoredAccent(): ThemeAccent {
  if (typeof window === 'undefined') return 'magenta'
  return (localStorage.getItem('theme-accent') as ThemeAccent) || 'magenta'
}

export function setStoredAccent(accent: ThemeAccent) {
  if (typeof window === 'undefined') return
  localStorage.setItem('theme-accent', accent)
}

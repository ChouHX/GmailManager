import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

export type ThemeMode = 'light' | 'dark' | 'system'

interface ThemeApi {
  mode: ThemeMode
  resolved: 'light' | 'dark'
  setMode: (mode: ThemeMode) => void
}

const ThemeContext = createContext<ThemeApi>({
  mode: 'system',
  resolved: 'light',
  setMode: () => undefined,
})

const STORAGE_KEY = 'gm-theme'

function readStored(): ThemeMode {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* ignore */
  }
  return 'system'
}

function systemDark(): boolean {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(readStored)
  const [dark, setDark] = useState(() =>
    mode === 'dark' ? true : mode === 'light' ? false : systemDark(),
  )

  useEffect(() => {
    const media = window.matchMedia?.('(prefers-color-scheme: dark)')
    const apply = () => {
      const next = mode === 'dark' ? true : mode === 'light' ? false : Boolean(media?.matches)
      setDark(next)
      document.documentElement.classList.toggle('dark', next)
      document.documentElement.style.colorScheme = next ? 'dark' : 'light'
    }
    apply()
    if (mode === 'system' && media) {
      media.addEventListener('change', apply)
      return () => media.removeEventListener('change', apply)
    }
  }, [mode])

  const setMode = useCallback((next: ThemeMode) => {
    setModeState(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      /* ignore */
    }
  }, [])

  const value = useMemo<ThemeApi>(
    () => ({ mode, resolved: dark ? 'dark' : 'light', setMode }),
    [mode, dark, setMode],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useThemeMode() {
  return useContext(ThemeContext)
}

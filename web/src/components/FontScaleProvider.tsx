import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

/**
 * Base UI font size in px.
 *
 * The UI is authored with fixed px type sizes, so the whole app is scaled with
 * `zoom` (see index.css) instead of changing the root font size. 12px is the
 * design baseline and maps to zoom 1.
 */
export const FONT_MIN = 11
export const FONT_MAX = 18
export const FONT_DEFAULT = 12

interface FontApi {
  /** Current base font size in px. */
  size: number
  setSize: (size: number) => void
  reset: () => void
}

const FontContext = createContext<FontApi>({
  size: FONT_DEFAULT,
  setSize: () => undefined,
  reset: () => undefined,
})

const STORAGE_KEY = 'gm-font-size'

export function clampFont(n: number): number {
  if (!Number.isFinite(n)) return FONT_DEFAULT
  return Math.min(FONT_MAX, Math.max(FONT_MIN, Math.round(n)))
}

function readStored(): number {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) return clampFont(Number(raw))
  } catch {
    /* storage blocked */
  }
  return FONT_DEFAULT
}

export function FontScaleProvider({ children }: { children: ReactNode }) {
  const [size, setSizeState] = useState<number>(readStored)

  useEffect(() => {
    const root = document.documentElement
    // One CSS variable drives the zoom; index.css derives the height fix-up.
    root.style.setProperty('--ui-scale', String(size / FONT_DEFAULT))
    root.dataset.fontSize = String(size)
  }, [size])

  const setSize = useCallback((next: number) => {
    const value = clampFont(next)
    setSizeState(value)
    try {
      localStorage.setItem(STORAGE_KEY, String(value))
    } catch {
      /* storage blocked */
    }
  }, [])

  const reset = useCallback(() => setSize(FONT_DEFAULT), [setSize])

  const value = useMemo(() => ({ size, setSize, reset }), [size, setSize, reset])

  return <FontContext.Provider value={value}>{children}</FontContext.Provider>
}

export function useFontScale() {
  return useContext(FontContext)
}

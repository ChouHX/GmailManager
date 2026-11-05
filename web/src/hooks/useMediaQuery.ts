import { useEffect, useState } from 'react'

/** True when the viewport is at or below the given breakpoint (default: < 768px). */
export function useMediaQuery(query = '(max-width: 767px)'): boolean {
  const [matches, setMatches] = useState(() =>
    typeof window === 'undefined' ? false : window.matchMedia(query).matches,
  )

  useEffect(() => {
    const media = window.matchMedia(query)
    const onChange = (e: MediaQueryListEvent) => setMatches(e.matches)
    setMatches(media.matches)
    media.addEventListener('change', onChange)
    return () => media.removeEventListener('change', onChange)
  }, [query])

  return matches
}

/** Convenience flag for phone-sized layouts. */
export function useIsMobile() {
  return useMediaQuery('(max-width: 767px)')
}

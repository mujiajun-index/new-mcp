import * as React from 'react'

const MOBILE_BREAKPOINT = 768

/**
 * Returns true when the viewport is narrower than 768px.
 *
 * Initial render returns `false` (state starts as `undefined`), so the first
 * paint always renders the desktop markup — this avoids a flash of hidden
 * content before the effect runs. The value corrects to the real viewport
 * width immediately after mount.
 */
export function useIsMobile() {
  const [isMobile, setIsMobile] = React.useState<boolean | undefined>(undefined)

  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`)
    const onChange = () => {
      setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    }
    mql.addEventListener('change', onChange)
    setIsMobile(window.innerWidth < MOBILE_BREAKPOINT)
    return () => mql.removeEventListener('change', onChange)
  }, [])

  return !!isMobile
}

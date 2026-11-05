// React 18 types predate the `inert` attribute (React 19 ships it), but browsers
// support it and KokonutUI relies on it to hide inactive menu pages from the
// accessibility tree. Declaring it here keeps those components unmodified.
import 'react'

declare module 'react' {
  interface HTMLAttributes<T> {
    inert?: '' | undefined
  }
}

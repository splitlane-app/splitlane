// Built-in icons (spec 9.8): the special tiles, programs without an icon of their own, and the glyphs of the board.
// Drawn on a 16×16 grid in currentColor, so the place that shows one gives it its colour and size.
import type { SVGProps } from 'react'

type IconProps = SVGProps<SVGSVGElement>

function Svg({ children, ...props }: IconProps): React.JSX.Element {
  return (
    <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" focusable="false" {...props}>
      {children}
    </svg>
  )
}

/** «Службы Windows»: a cog. */
export function ServicesIcon(props: IconProps): React.JSX.Element {
  return (
    <Svg {...props}>
      <path
        fill="currentColor"
        fillRule="evenodd"
        d="M6.9 1h2.2l.4 1.9c.4.1.8.3 1.1.5l1.6-1.1 1.6 1.6-1.1 1.6c.2.3.4.7.5 1.1l1.9.4v2.2l-1.9.4c-.1.4-.3.8-.5 1.1l1.1 1.6-1.6 1.6-1.6-1.1c-.3.2-.7.4-1.1.5L9.1 15H6.9l-.4-1.9c-.4-.1-.8-.3-1.1-.5l-1.6 1.1-1.6-1.6 1.1-1.6c-.2-.3-.4-.7-.5-1.1L1 9.1V6.9l1.9-.4c.1-.4.3-.8.5-1.1L2.3 3.8l1.6-1.6 1.6 1.1c.3-.2.7-.4 1.1-.5L6.9 1zM8 5.6a2.4 2.4 0 1 0 0 4.8 2.4 2.4 0 0 0 0-4.8z"
      />
    </Svg>
  )
}

/** «WSL и виртуальные машины»: a console window with a prompt. */
export function MachinesIcon(props: IconProps): React.JSX.Element {
  return (
    <Svg {...props}>
      <rect x="1.5" y="2.5" width="13" height="11" rx="2" fill="none" stroke="currentColor" strokeWidth="1.3" />
      <path d="M4.2 6.2 6.4 8 4.2 9.8" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M7.8 10.3h3.6" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
    </Svg>
  )
}

/** A program without an icon of its own: an application window. */
export function ProgramIcon(props: IconProps): React.JSX.Element {
  return (
    <Svg {...props}>
      <rect x="1.5" y="2.5" width="13" height="11" rx="2" fill="none" stroke="currentColor" strokeWidth="1.3" />
      <path d="M1.5 5.8h13" stroke="currentColor" strokeWidth="1.3" />
      <circle cx="3.6" cy="4.15" r=".6" fill="currentColor" />
      <circle cx="5.4" cy="4.15" r=".6" fill="currentColor" />
    </Svg>
  )
}

/** The pin of a pinned card (the mockups' glyph). */
export function PinIcon(props: IconProps): React.JSX.Element {
  return (
    <Svg {...props}>
      <path
        fill="currentColor"
        d="M10.2 1.3l4.5 4.5-1.1 1.1-1-.3-2.4 2.4.4 2.8-1.2 1.2-2.9-2.9-3.7 3.7-.6-.6 3.7-3.7-2.9-2.9 1.2-1.2 2.8.4 2.4-2.4-.3-1z"
      />
    </Svg>
  )
}

/** The magnifier of the search field. */
export function SearchIcon(props: IconProps): React.JSX.Element {
  return (
    <Svg {...props}>
      <circle cx="7" cy="7" r="4.3" fill="none" stroke="currentColor" strokeWidth="1.4" />
      <path d="m10.2 10.2 3.6 3.6" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </Svg>
  )
}

/** The settings button of the header: an outlined gear (the filled cog is the «Службы Windows» tile). */
export function SettingsIcon(props: IconProps): React.JSX.Element {
  return (
    <Svg {...props}>
      <circle cx="8" cy="8" r="5.7" fill="none" stroke="currentColor" strokeWidth="2.2" strokeDasharray="2.24 2.24" />
      <circle cx="8" cy="8" r="4.1" fill="none" stroke="currentColor" strokeWidth="1.4" />
      <circle cx="8" cy="8" r="1.5" fill="none" stroke="currentColor" strokeWidth="1.3" />
    </Svg>
  )
}

/** The cross that closes a notice. */
export function CloseIcon(props: IconProps): React.JSX.Element {
  return (
    <Svg {...props}>
      <path d="m4 4 8 8M12 4l-8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </Svg>
  )
}

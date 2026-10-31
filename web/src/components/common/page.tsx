import type { ReactNode } from 'react'
import { Link, NavLink, useLocation } from 'react-router-dom'

export interface PageTab {
  to: string
  label: string
  // For a tab that also covers paths other than its own.
  isActive?: (pathname: string) => boolean
}

interface PageProps {
  /** The page's name. A list page leaves it to the top bar and only names it for
   * assistive technology; a detail page shows it. */
  title: string
  /** Makes this a detail page, whose first row shows the object it is about. */
  detail?: {
    /** A link back to the list. */
    back: ReactNode
    /** The object's state, beside its name. */
    status?: ReactNode
    /** What the object is made of, after its state. */
    meta?: ReactNode
  }
  /** A list page's filter, on the left where section tabs would be. */
  leading?: ReactNode
  /** Page-level controls, right-aligned. */
  actions?: ReactNode
  /** Sections of the page, each with its own route. */
  tabs?: PageTab[]
  className?: string
  children: ReactNode
}

function PageTabs({ tabs }: { tabs: PageTab[] }) {
  const { pathname } = useLocation()
  return (
    <nav className="vergex-page-tabs" aria-label="Sections">
      {tabs.map((tab) =>
        tab.isActive ? (
          <Link
            key={tab.to}
            to={tab.to}
            aria-current={tab.isActive(pathname) ? 'page' : undefined}
          >
            {tab.label}
          </Link>
        ) : (
          <NavLink key={tab.to} to={tab.to} end>
            {tab.label}
          </NavLink>
        )
      )}
    </nav>
  )
}

// Every app page shares this frame: full-width content inside the page gutter,
// with its controls and section tabs on one row above it.
export function Page({
  title,
  detail,
  leading,
  actions,
  tabs,
  className = '',
  children,
}: PageProps) {
  const controls = actions && (
    <div className="vergex-page-actions">{actions}</div>
  )
  return (
    <div className={`vergex-page ${className}`}>
      {detail ? (
        <>
          <header className="vergex-page-identity">
            {detail.back}
            <h1 title={title}>{title}</h1>
            {detail.status}
            {detail.meta && (
              <div className="vergex-page-meta">{detail.meta}</div>
            )}
            {controls}
          </header>
          {tabs && <PageTabs tabs={tabs} />}
        </>
      ) : (
        <>
          <h1 className="sr-only">{title}</h1>
          {(tabs || leading || actions) && (
            <header className="vergex-page-bar">
              {tabs && <PageTabs tabs={tabs} />}
              {leading && <div className="vergex-page-leading">{leading}</div>}
              {controls}
            </header>
          )}
        </>
      )}
      {children}
    </div>
  )
}

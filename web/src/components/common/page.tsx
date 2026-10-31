import type { ReactNode } from 'react'
import { NavLink } from 'react-router-dom'

export interface PageTab {
  to: string
  label: string
}

interface PageProps {
  title: ReactNode
  description?: ReactNode
  /** Page-level controls, right-aligned beside the title. */
  actions?: ReactNode
  actionsClassName?: string
  /** Sections of the page, each with its own route. */
  tabs?: PageTab[]
  className?: string
  children: ReactNode
}

// Every app page shares this frame: full-width content inside the page
// gutter, one header style, and optional section tabs.
export function Page({
  title,
  description,
  actions,
  actionsClassName = '',
  tabs,
  className = '',
  children,
}: PageProps) {
  return (
    <div className={`vergex-page ${className}`}>
      <header className="vergex-page-header">
        <div className="vergex-page-title">
          <h1>{title}</h1>
          {description && <p>{description}</p>}
        </div>
        {actions && (
          <div className={`vergex-page-actions ${actionsClassName}`}>
            {actions}
          </div>
        )}
      </header>
      {tabs && (
        <nav className="vergex-page-tabs" aria-label="Sections">
          {tabs.map((tab) => (
            <NavLink key={tab.to} to={tab.to} end>
              {tab.label}
            </NavLink>
          ))}
        </nav>
      )}
      {children}
    </div>
  )
}

import { NavLink } from 'react-router-dom';

export function Sidebar() {
  return (
    <aside className="of-sidebar">
      <div className="of-sidebar__brand">
        <NavLink to="/" className="of-sidebar__logo" aria-label="Home">
          OF
        </NavLink>
      </div>
      <nav className="of-sidebar__section">
        <div className="of-sidebar__heading">Workspace</div>
        <NavLink
          to="/"
          end
          className="of-sidebar__link"
          style={({ isActive }) => ({ background: isActive ? 'var(--bg-sidebar-active)' : undefined })}
        >
          <span className="of-sidebar__label">Home</span>
        </NavLink>
        <NavLink
          to="/dashboards"
          className="of-sidebar__link"
          style={({ isActive }) => ({ background: isActive ? 'var(--bg-sidebar-active)' : undefined })}
        >
          <span className="of-sidebar__label">Dashboards</span>
        </NavLink>
        <NavLink
          to="/lineage"
          className="of-sidebar__link"
          style={({ isActive }) => ({ background: isActive ? 'var(--bg-sidebar-active)' : undefined })}
        >
          <span className="of-sidebar__label">Lineage</span>
        </NavLink>
        <NavLink
          to="/notebooks"
          className="of-sidebar__link"
          style={({ isActive }) => ({ background: isActive ? 'var(--bg-sidebar-active)' : undefined })}
        >
          <span className="of-sidebar__label">Notebooks</span>
        </NavLink>
        <NavLink
          to="/settings"
          className="of-sidebar__link"
          style={({ isActive }) => ({ background: isActive ? 'var(--bg-sidebar-active)' : undefined })}
        >
          <span className="of-sidebar__label">Settings</span>
        </NavLink>
      </nav>
    </aside>
  );
}

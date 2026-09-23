import { createContext, useContext } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, Navigate, Outlet } from '@tanstack/react-router';
import type { Role } from '@tap4furry/api-client/admin';
import { AdminRequestError, errorMessage, requireAdmin } from '../lib/admin-api';
import { canEditorial } from '../lib/capabilities';
import { keys } from '../lib/query-keys';
import styles from './AdminShell.module.scss';
const RolesContext = createContext<readonly Role[]>([]);
export const useRoles = () => useContext(RolesContext);
export function AdminShell() {
  const me = useQuery({
    queryKey: keys.me,
    queryFn: requireAdmin,
    retry: false,
  });
  if (me.error instanceof AdminRequestError && [401, 403].includes(me.error.status))
    return <Navigate to="/login" replace />;
  if (me.error) return <p role="alert">{errorMessage(me.error)}</p>;
  if (!me.data) return <p role="status">Loading Admin…</p>;
  return (
    <RolesContext.Provider value={me.data.roles}>
      <div className={`${styles.shell} min-h-screen`}>
        <header
          className={`${styles.header} flex flex-wrap items-center justify-between gap-4 px-6 py-5`}
        >
          <Link to="/resources" className={styles.brand}>
            Tap4Furry <span>Admin</span>
          </Link>
          <span>{me.data.roles.join(' / ')}</span>
        </header>
        <div className="mx-auto grid max-w-7xl gap-8 px-4 py-6 md:grid-cols-[11rem_1fr] md:px-6">
          <nav
            aria-label="Admin navigation"
            className="flex flex-wrap content-start gap-3 md:flex-col"
          >
            <Link to="/resources">Resources</Link>
            {canEditorial(me.data.roles) && <Link to="/contributions">Contributions</Link>}
            <Link to="/taxonomy/categories">Categories</Link>
            <Link to="/taxonomy/tags">Tags</Link>
            <Link to="/account">Account</Link>
          </nav>
          <main className="flex min-w-0 flex-col gap-6">
            {!canEditorial(me.data.roles) && (
              <aside className={styles.notice}>
                <strong>Read-only</strong>
                <p>Your current role does not include Editorial capability.</p>
              </aside>
            )}
            <Outlet />
          </main>
        </div>
      </div>
    </RolesContext.Provider>
  );
}

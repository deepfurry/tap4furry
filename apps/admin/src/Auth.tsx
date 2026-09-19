import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Navigate, useNavigate } from '@tanstack/react-router';
import { getCsrf, getMe, listSessions, login, logout, reauthenticate, revokeSession, revokeOtherSessions } from '@tap4furry/api-client/admin';
import styles from './Foundation.module.scss';

export async function requireAdmin() {
  const response = await getMe({ credentials: 'same-origin', cache: 'no-store' });
  if (response.status !== 200) throw new AdminRequestError(response.status, response.data.message, response.data.code);
  return response.data;
}
export class AdminRequestError extends Error {
  constructor(readonly status: number, message: string, readonly code?: string) { super(message); }
}
const errorMessage = (error: unknown) => error instanceof AdminRequestError ? error.message : 'Admin API is unavailable. Please try again.';
async function csrfHeaders() {
  const response = await getCsrf({ credentials: 'same-origin', cache: 'no-store' });
  if (response.status !== 200) throw new AdminRequestError(response.status, response.data.message, response.data.code);
  return { 'X-CSRF-Token': response.data.csrf_token };
}

export function Login() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const [password, setPassword] = useState('');
  const mutation = useMutation({ mutationFn: async (email: string) => {
    const suppliedPassword = password;
    setPassword('');
    const response = await login({ email, password: suppliedPassword }, { credentials: 'same-origin' });
    if (response.status !== 200) throw new AdminRequestError(response.status, response.data.message, response.data.code);
    client.clear();
    await navigate({ to: '/' });
  } });
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    mutation.mutate(String(data.get('email') ?? ''));
  }
  return <main className="mx-auto flex min-h-screen max-w-lg flex-col justify-center gap-6 p-6">
    <section className={`${styles.panel} flex flex-col gap-6 p-6 sm:p-10`}>
      <span className={styles.phase}>Tap4Furry Admin</span>
      <h1>Sign in</h1>
      <p>Use your verified email and password to access your team workspace.</p>
      <form className="flex flex-col gap-5" onSubmit={submit}>
        <label className="flex flex-col gap-2">Email<input name="email" type="email" autoComplete="username" required maxLength={254} className="px-3 py-2" /></label>
        <label className="flex flex-col gap-2">Password<input type="password" autoComplete="current-password" required minLength={15} maxLength={128} value={password} onChange={event => setPassword(event.target.value)} className="px-3 py-2" /></label>
        <button className={`${styles.button} px-4 py-2`} disabled={mutation.isPending}>{mutation.isPending ? 'Signing in…' : 'Sign in'}</button>
      </form>
      {mutation.isError && <p role="alert">{errorMessage(mutation.error)}</p>}
    </section>
  </main>;
}

export function Workspace() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const [password, setPassword] = useState('');
  const me = useQuery({ queryKey: ['admin', 'me'], queryFn: requireAdmin, retry: false });
  const sessions = useQuery({ queryKey: ['admin', 'sessions'], retry: false, queryFn: async ({ signal }) => {
    const response = await listSessions({ signal, credentials: 'same-origin', cache: 'no-store' });
    if (response.status !== 200) throw new AdminRequestError(response.status, response.data.message, response.data.code);
    return response.data.sessions;
  } });
  const mutation = useMutation({ mutationFn: async (action: { kind: 'logout' | 'reauth' | 'others' | 'revoke'; id?: string; current?: boolean }) => {
    const suppliedPassword = password;
    setPassword('');
    const headers = await csrfHeaders();
    const options = { headers, credentials: 'same-origin' as const };
    const response = action.kind === 'logout' ? await logout(options)
      : action.kind === 'reauth' ? await reauthenticate({ password: suppliedPassword }, options)
        : action.kind === 'others' ? await revokeOtherSessions(options)
          : await revokeSession(action.id!, options);
    if (response.status !== 204) throw new AdminRequestError(response.status, response.data.message, response.data.code);
    if (action.kind === 'logout' || action.current) { client.clear(); await navigate({ to: '/login' }); }
    else await client.invalidateQueries({ queryKey: ['admin'] });
  }, onError: async error => {
    if (error instanceof AdminRequestError && (error.code === 'ADMIN_UNAUTHENTICATED' || error.code === 'ADMIN_FORBIDDEN')) {
      client.clear(); await navigate({ to: '/login' });
    }
  } });
  const failure = me.error ?? sessions.error;
  if (failure instanceof AdminRequestError && (failure.status === 401 || failure.status === 403)) {
    return <Navigate to="/login" replace />;
  }
  return <main className="mx-auto flex min-h-screen max-w-3xl flex-col gap-6 p-6 sm:p-10">
    <header className="flex flex-wrap items-center justify-between gap-4"><div><span className={styles.phase}>Tap4Furry Admin</span><h1>Your workspace</h1></div><button className={`${styles.button} px-4 py-2`} disabled={mutation.isPending} onClick={() => mutation.mutate({ kind: 'logout' })}>Sign out</button></header>
    {failure && <p role="alert">{errorMessage(failure)}</p>}
    <section className={`${styles.panel} flex flex-col gap-3 p-6`} aria-labelledby="account-heading">
      <h2 id="account-heading">Your account</h2>
      {me.isPending ? <p role="status">Loading account…</p> : me.data && <><p className="break-all">{me.data.email}</p><p>Roles: {me.data.roles.join(', ')}</p><p>Authenticated: {new Date(me.data.authenticated_at).toLocaleString()}</p></>}
    </section>
    <section className={`${styles.panel} flex flex-col gap-5 p-6`} aria-labelledby="sessions-heading">
      <div className="flex flex-wrap items-center justify-between gap-3"><h2 id="sessions-heading">Admin sessions</h2><button className={`${styles.button} px-4 py-2`} disabled={mutation.isPending || !sessions.data} onClick={() => mutation.mutate({ kind: 'others' })}>Sign out other sessions</button></div>
      {sessions.isPending ? <p role="status">Loading sessions…</p> : <ul className="flex flex-col gap-4">{sessions.data?.map(session => <li key={session.id} className="flex flex-wrap items-center justify-between gap-4 pt-4"><div><p>{session.current ? 'This session' : 'Another session'}</p><p>Last active: {new Date(session.last_seen_at).toLocaleString()}</p><p>Expires: {new Date(session.absolute_expires_at).toLocaleString()}</p></div><button className={`${styles.button} px-3 py-2`} disabled={mutation.isPending} onClick={() => mutation.mutate({ kind: 'revoke', id: session.id, current: session.current })}>Revoke{session.current ? ' current' : ''}</button></li>)}</ul>}
    </section>
    <section className={`${styles.panel} flex flex-col gap-5 p-6`} aria-labelledby="reauth-heading"><h2 id="reauth-heading">Verify your identity again</h2><p>Confirm your password to refresh this session.</p>
      <form className="flex flex-col gap-4" onSubmit={event => { event.preventDefault(); mutation.mutate({ kind: 'reauth' }); }}><label className="flex flex-col gap-2">Current password<input type="password" autoComplete="current-password" required minLength={15} maxLength={128} value={password} onChange={event => setPassword(event.target.value)} className="px-3 py-2" /></label><button className={`${styles.button} px-4 py-2`} disabled={mutation.isPending}>Verify password</button></form>
    </section>
    {mutation.isError && <p role="alert">{errorMessage(mutation.error)}</p>}
    {mutation.isSuccess && <p role="status">Session updated.</p>}
  </main>;
}

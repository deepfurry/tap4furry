import { useEffect, useState, type SubmitEvent } from 'react';
import { changePassword, listSessions, getAuthMethods, requestEmailVerification, revokeSession, revokeOtherSessions, type Me, type Session, type AuthMethods } from '@tap4furry/api-client/public';
import AuthenticationMethods from './AuthenticationMethods';
import { authenticatedRequest, validPassword } from '../lib/security';
import styles from './Account.module.scss';

export default function AccountSecurity({ me, onMe }: { me: Me; onMe: (me: Me) => void }) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [methods, setMethods] = useState<AuthMethods | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    void listSessions({ signal: controller.signal, credentials: 'same-origin', cache: 'no-store' }).then(response => {
      if (response.status === 200) setSessions(response.data.sessions);
      else setMessage(response.data.message);
    }).catch(() => { if (!controller.signal.aborted) setMessage('Unable to load sessions. Reload to try again.'); });
    void getAuthMethods({ signal: controller.signal, credentials: 'same-origin', cache: 'no-store' }).then(response => {
      if (response.status === 200) setMethods(response.data);
      else setMessage(response.data.message);
    }).catch(() => { if (!controller.signal.aborted) setMessage('Unable to load sign-in methods. Reload to try again.'); });
    return () => controller.abort();
  }, [revision]);
  async function resend() {
    setBusy(true); setMessage('');
    try {
      const response = await requestEmailVerification(await authenticatedRequest());
      setMessage(response.status === 202 ? 'Check your inbox for a verification link. Please wait a minute before requesting another.' : response.data.message);
    } catch { setMessage('Unable to send verification email. Please try again.'); }
    finally { setBusy(false); }
  }
  async function passwordChange(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget, values = new FormData(form);
    const password = String(values.get('new_password') ?? '');
    if (!validPassword(password)) { setMessage('Use 15–128 characters for the new password.'); return; }
    if (password !== values.get('confirm_password')) { setMessage('The new passwords do not match.'); return; }
    setBusy(true); setMessage('');
    try {
      const response = await changePassword({ current_password: String(values.get('current_password') ?? ''), new_password: password }, await authenticatedRequest());
      if (response.status === 200) { form.reset(); onMe(response.data); setRevision(value => value + 1); setMessage('Password changed. All previous sessions have been signed out.'); }
      else setMessage(response.data.message);
    } catch { setMessage('Unable to change password. Please try again.'); }
    finally { setBusy(false); }
  }
  async function revoke(session?: Session) {
    setBusy(true); setMessage('');
    try {
      const options = await authenticatedRequest();
      const response = session ? await revokeSession(session.id, options) : await revokeOtherSessions(options);
      if (response.status === 204) {
        if (session?.current) { window.location.assign('/login'); return; }
        setRevision(value => value + 1); setMessage(session ? 'Session signed out.' : 'Other sessions signed out.');
      } else setMessage(response.data.message);
    } catch { setMessage('Unable to sign out sessions. Please try again.'); }
    finally { setBusy(false); }
  }
  return <section className="flex flex-col gap-6" aria-labelledby="security-heading">
    <h2 id="security-heading">Account security</h2>
    {!me.email_verified && <button type="button" className={styles.button} disabled={busy} onClick={() => { void resend(); }}>Send verification email</button>}
    {methods && <AuthenticationMethods methods={methods} changed={next => { if (next) onMe(next); setRevision(value => value + 1); }} />}
    {methods?.password && <form onSubmit={passwordChange} className="flex flex-col gap-4">
      <h3>Change password</h3>
      <label className="flex flex-col gap-2">Current password<input className={styles.input} type="password" name="current_password" autoComplete="current-password" required /></label>
      <label className="flex flex-col gap-2">New password<input className={styles.input} type="password" name="new_password" autoComplete="new-password" required aria-describedby="new-password-help" /></label>
      <p id="new-password-help" className={styles.hint}>15–128 characters. Changing your password signs out all previous sessions.</p>
      <label className="flex flex-col gap-2">Confirm new password<input className={styles.input} type="password" name="confirm_password" autoComplete="new-password" required /></label>
      <button type="submit" className={styles.button} disabled={busy}>Change password</button>
    </form>}
    <h3>Active sessions</h3>
    <ul className="flex flex-col gap-4">
      {sessions.map(session => <li key={session.id} className="flex flex-col gap-2">
        <p>{session.current ? 'This session' : 'Another session'} · {{ password: 'Password', password_reset: 'Password reset', google: 'Google', github: 'GitHub' }[session.auth_method]}</p>
        <p className={styles.hint}>Started {new Date(session.created_at).toLocaleString()}. Last active {new Date(session.last_seen_at).toLocaleString()}.</p>
        <button type="button" className={styles.button} disabled={busy} onClick={() => { void revoke(session); }}>{session.current ? 'Sign out this session' : 'Sign out session'}</button>
      </li>)}
    </ul>
    <button type="button" className={styles.button} disabled={busy || sessions.length < 2} onClick={() => { void revoke(); }}>Sign out other sessions</button>
    <p role="status" aria-live="polite">{message}</p>
  </section>;
}

import { useEffect, useState, type SubmitEvent } from 'react';
import { getMe, logout, updateProfile, type Me } from '@tap4furry/api-client/public';
import styles from './Account.module.scss';
import { authenticatedRequest } from '../lib/security';
import AccountSecurity from './AccountSecurity';

export default function Account() {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const [unauthenticated, setUnauthenticated] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    void getMe({ signal: controller.signal, credentials: 'same-origin', cache: 'no-store' }).then(response => {
      if (response.status === 200) setMe(response.data);
      else if (response.status === 401) setUnauthenticated(true);
      else setMessage(response.data.message);
    }).catch(() => { if (!controller.signal.aborted) setMessage('Unable to load your account. Please reload to try again.'); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, []);

  async function save(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    const text = (name: string) => { const value = String(values.get(name) ?? ''); return value === '' ? null : value; };
    const handle = text('handle'), displayName = text('display_name'), bio = text('bio');
    if ((displayName && [...displayName].length > 80) || (bio && [...bio].length > 500)) {
      setMessage('Use at most 80 characters for your display name and 500 for your bio.'); return;
    }
    setBusy(true); setMessage('');
    try {
      const response = await updateProfile({ handle, display_name: displayName, bio, search_engine_indexing: values.has('search_engine_indexing') }, await authenticatedRequest());
      if (response.status === 200) { setMe(response.data); setRevision(value => value + 1); setMessage('Profile saved.'); }
      else if (response.status === 401) { setMe(null); setUnauthenticated(true); }
      else setMessage(response.data.message);
    } catch { setMessage('Unable to save. Please try again.'); }
    finally { setBusy(false); }
  }
  async function signOut() {
    setBusy(true); setMessage('');
    try {
      const response = await logout(await authenticatedRequest());
      if (response.status === 204 || response.status === 401) { setMe(null); window.location.assign('/login'); }
      else setMessage(response.data.message);
    } catch { setMessage('Unable to log out. Please try again.'); }
    finally { setBusy(false); }
  }
  if (loading) return <p role="status">Loading your account…</p>;
  if (unauthenticated) return <p><a href="/login">Log in</a> or <a href="/register">create an account</a> to manage your profile.</p>;
  if (!me) return <p role="alert">{message}</p>;
  return <div className="flex flex-col gap-6">
    <div className="flex flex-col gap-2"><p>{me.email}</p><p className={styles.hint}>{me.email_verified ? 'Email verified' : 'Email not yet verified'}. Your email stays private.</p></div>
    <form key={revision} onSubmit={save} className="flex flex-col gap-6">
      <label className="flex flex-col gap-2">Handle
        <input className={styles.input} name="handle" defaultValue={me.profile.handle ?? ''} pattern="[a-z0-9][a-z0-9_\-]{2,31}" maxLength={32} aria-describedby="handle-help" autoCapitalize="none" spellCheck={false} />
        <span id="handle-help" className={styles.hint}>Optional. 3–32 lowercase letters, numbers, underscores or hyphens; start with a letter or number.</span>
      </label>
      <label className="flex flex-col gap-2">Display name
        <input className={styles.input} name="display_name" defaultValue={me.profile.display_name ?? ''} aria-describedby="name-help" />
        <span id="name-help" className={styles.hint}>Up to 80 characters.</span>
      </label>
      <label className="flex flex-col gap-2">Bio
        <textarea className={styles.input} name="bio" defaultValue={me.profile.bio ?? ''} rows={5} aria-describedby="bio-help" />
        <span id="bio-help" className={styles.hint}>Up to 500 characters. Your handle, display name and bio are public.</span>
      </label>
      <label className="flex items-start gap-3"><input name="search_engine_indexing" type="checkbox" defaultChecked={me.profile.search_engine_indexing} />Allow search engines to index my public profile</label>
      <button type="submit" className={styles.button} disabled={busy}>{busy ? 'Please wait…' : 'Save profile'}</button>
    </form>
    <p role="status" aria-live="polite">{message}</p>
    <AccountSecurity me={me} onMe={setMe} />
    <button type="button" onClick={() => { void signOut(); }} className={styles.button} disabled={busy}>Log out</button>
  </div>;
}

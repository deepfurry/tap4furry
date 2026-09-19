import { useState, type SubmitEvent } from 'react';
import { linkOAuthProvider, reauthenticateOAuthProvider, unlinkOAuthProvider, reauthenticate, type AuthMethods, type Me, type OAuthProvider } from '@tap4furry/api-client/public';
import { authenticatedRequest } from '../lib/security';
import styles from './Account.module.scss';

export default function AuthenticationMethods({ methods, changed }: { methods: AuthMethods; changed: (me?: Me) => void }) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  async function providerAction(provider: OAuthProvider, action: 'link' | 'reauthenticate' | 'unlink') {
    setBusy(true); setMessage('');
    try {
      const options = await authenticatedRequest();
      if (action === 'unlink') {
        const response = await unlinkOAuthProvider(provider, options);
        if (response.status === 200) { changed(response.data); setMessage('Provider unlinked. Its sessions have been signed out.'); }
        else setMessage(response.data.message);
      } else {
        const response = await (action === 'link' ? linkOAuthProvider : reauthenticateOAuthProvider)(provider, options);
        if (response.status === 200) {
          const url = new URL(response.data.authorization_url);
          const expected = provider === 'google' ? 'https://accounts.google.com/o/oauth2/v2/auth' : 'https://github.com/login/oauth/authorize';
          if (url.origin + url.pathname !== expected || url.username || url.password || url.hash) throw new Error('Invalid authorization destination');
          window.location.assign(url.href);
        } else setMessage(response.data.message);
      }
    } catch { setMessage('Unable to update sign-in methods. Please try again.'); }
    finally { setBusy(false); }
  }
  async function passwordReauth(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const password = String(new FormData(form).get('password') ?? '');
    setBusy(true); setMessage('');
    try {
      const response = await reauthenticate({ password }, await authenticatedRequest());
      if (response.status === 204) { form.reset(); changed(); setMessage('Password verified. You can now link or unlink a provider.'); }
      else setMessage(response.data.message);
    } catch { setMessage('Unable to verify your password. Please try again.'); }
    finally { setBusy(false); }
  }
  return <section className="flex flex-col gap-4" aria-labelledby="methods-heading">
    <h3 id="methods-heading">Sign-in methods</h3>
    <p>Password: {methods.password ? 'Configured' : 'Not configured. Use your linked providers to sign in.'}</p>
    <p className={styles.hint}>Linking and unlinking require verification within 15 minutes. Before removing your current sign-in provider, reauthenticate with another method. Keep at least one method.</p>
    <ul className="flex flex-col gap-4">
      {(['google', 'github'] as const).map(provider => {
        const linked = methods.providers.find(method => method.provider === provider);
        const name = provider === 'google' ? 'Google' : 'GitHub';
        return <li key={provider} className="flex flex-col gap-2">
          <p>{name}: {linked ? 'Linked' : 'Not linked'}</p>
          {linked && <p className={styles.hint}>{linked.email ?? 'No provider email available'} · Linked {new Date(linked.linked_at).toLocaleDateString()}. This information stays private.</p>}
          <div className="flex flex-wrap gap-3">
            <button type="button" className={`${styles.button} px-3 py-2`} disabled={busy} onClick={() => { void providerAction(provider, linked ? 'reauthenticate' : 'link'); }}>{linked ? `Reauthenticate with ${name}` : `Link ${name}`}</button>
            {linked && <button type="button" className={`${styles.button} px-3 py-2`} disabled={busy || (!methods.password && methods.providers.length === 1)} onClick={() => { void providerAction(provider, 'unlink'); }}>Unlink {name}</button>}
          </div>
        </li>;
      })}
    </ul>
    {methods.password && <form onSubmit={passwordReauth} className="flex flex-col gap-3">
      <label className="flex flex-col gap-2">Reauthenticate with password<input className={styles.input} type="password" name="password" autoComplete="current-password" required /></label>
      <button className={`${styles.button} px-3 py-2`} type="submit" disabled={busy}>Verify password</button>
    </form>}
    <p role="status" aria-live="polite">{message}</p>
  </section>;
}

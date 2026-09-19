import { useState, type SubmitEvent } from 'react';
import { login, register } from '@tap4furry/api-client/public';
import styles from './Account.module.scss';

export default function CredentialsForm({ mode }: { mode: 'login' | 'register' }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    const password = String(values.get('password') ?? '');
    if ([...password].length < 15 || [...password].length > 128) {
      setError('Use 15–128 characters for your password. Spaces and Unicode are welcome.');
      return;
    }
    setBusy(true); setError('');
    try {
      const response = await (mode === 'login' ? login : register)(
        { email: String(values.get('email') ?? ''), password }, { credentials: 'same-origin' });
      if (response.status === 200 || response.status === 201) {
        form.reset(); window.location.assign('/account');
      } else { setError(response.data.message); }
    } catch { setError('Unable to connect. Please try again.'); }
    finally { setBusy(false); }
  }
  return <form onSubmit={submit} className="flex flex-col gap-6">
    <label className="flex flex-col gap-2">Email
      <input className={styles.input} name="email" type="email" autoComplete="email" required maxLength={254} />
    </label>
    <label className="flex flex-col gap-2">Password
      <input className={styles.input} name="password" type="password" autoComplete={mode === 'register' ? 'new-password' : 'current-password'} required aria-describedby="password-help" />
      <span id="password-help" className={styles.hint}>15–128 characters. Spaces and Unicode are welcome.</span>
    </label>
    <p role="alert" className={styles.error}>{error}</p>
    <button className={styles.button} type="submit" disabled={busy}>{busy ? 'Please wait…' : mode === 'register' ? 'Create account' : 'Log in'}</button>
  </form>;
}

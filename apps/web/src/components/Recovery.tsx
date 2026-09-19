import { useEffect, useRef, useState, type SubmitEvent } from 'react';
import { requestPasswordReset, resetPassword, verifyEmail } from '@tap4furry/api-client/public';
import { validPassword } from '../lib/security';
import styles from './Account.module.scss';

export default function Recovery({ mode }: { mode: 'request' | 'reset' | 'verify' }) {
  const [token, setToken] = useState('');
  const captured = useRef(false);
  const [ready, setReady] = useState(mode === 'request');
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [message, setMessage] = useState('');
  useEffect(() => {
    if (mode === 'request' || captured.current) return;
    captured.current = true;
    const fragment = window.location.hash;
    // Remove the fragment before parsing, networking or rendering its contents.
    window.history.replaceState(null, '', window.location.pathname);
    const value = new URLSearchParams(fragment.slice(1)).get('token') ?? '';
    setToken(/^[A-Za-z0-9_-]{43}$/.test(value) ? value : '');
    setReady(true);
  }, [mode]);
  async function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget, values = new FormData(form);
    const password = String(values.get('password') ?? '');
    if (mode === 'reset' && !validPassword(password)) { setMessage('Use 15–128 characters for your password.'); return; }
    if (mode === 'reset' && password !== values.get('confirm_password')) { setMessage('The passwords do not match.'); return; }
    setBusy(true); setMessage('');
    try {
      if (mode === 'request') {
        const response = await requestPasswordReset({ email: String(values.get('email') ?? '') }, { credentials: 'same-origin' });
        setMessage(response.data.message);
        if (response.status === 202) { form.reset(); setDone(true); }
      } else if (mode === 'verify') {
        const response = await verifyEmail({ token }, { credentials: 'same-origin' });
        if (response.status === 204) { setToken(''); setDone(true); setMessage('Email verified.'); }
        else setMessage(response.data.message);
      } else {
        const response = await resetPassword({ token, new_password: password }, { credentials: 'same-origin' });
        if (response.status === 200) { setToken(''); form.reset(); window.location.assign('/account'); }
        else setMessage(response.data.message);
      }
    } catch { setMessage('Unable to complete the request. Please try again.'); }
    finally { setBusy(false); }
  }
  if (!ready) return <p role="status">Opening your link…</p>;
  if (!done && mode !== 'request' && !token) return <p role="alert">This link is missing or invalid. {mode === 'verify' ? <a href="/account">Request another verification email</a> : <a href="/forgot-password">Request another reset link</a>}.</p>;
  return <div className="flex flex-col gap-4">
    {!done && <form onSubmit={submit} className="flex flex-col gap-4">
      {mode === 'request' && <label className="flex flex-col gap-2">Email<input className={styles.input} type="email" name="email" autoComplete="email" maxLength={254} required /></label>}
      {mode === 'reset' && <>
        <label className="flex flex-col gap-2">New password<input className={styles.input} type="password" name="password" autoComplete="new-password" required aria-describedby="reset-password-help" /></label>
        <p id="reset-password-help" className={styles.hint}>15–128 characters. Your previous sessions will be signed out.</p>
        <label className="flex flex-col gap-2">Confirm new password<input className={styles.input} type="password" name="confirm_password" autoComplete="new-password" required /></label>
      </>}
      <button className={styles.button} type="submit" disabled={busy}>{busy ? 'Please wait…' : mode === 'request' ? 'Send reset link' : mode === 'verify' ? 'Verify email' : 'Reset password'}</button>
    </form>}
    <p role="status" aria-live="polite">{message}</p>
    <p><a href="/account">Go to your account</a> · <a href="/login">Log in</a></p>
  </div>;
}

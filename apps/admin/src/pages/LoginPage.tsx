import { useState, type FormEvent } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { login } from '@tap4furry/api-client/admin';
import { AdminRequestError, errorMessage } from '../lib/admin-api';
import styles from '../Foundation.module.scss';
export function LoginPage() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const [password, setPassword] = useState('');
  const mutation = useMutation({
    mutationFn: async (email: string) => {
      const suppliedPassword = password;
      setPassword('');
      const response = await login(
        { email, password: suppliedPassword },
        { credentials: 'same-origin' },
      );
      if (response.status !== 200)
        throw new AdminRequestError(response.status, response.data.message, response.data.code);
      client.clear();
      await navigate({ to: '/resources' });
    },
  });
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    mutation.mutate(String(data.get('email') ?? ''));
  }
  return (
    <main className="mx-auto flex min-h-screen max-w-lg flex-col justify-center gap-6 p-6">
      <section className={`${styles.panel} flex flex-col gap-6 p-6 sm:p-10`}>
        <span className={styles.phase}>Tap4Furry Admin</span>
        <h1>Sign in</h1>
        <p>Use your verified email and password to access your team workspace.</p>
        <form className="flex flex-col gap-5" onSubmit={submit}>
          <label className="flex flex-col gap-2">
            Email
            <input
              name="email"
              type="email"
              autoComplete="username"
              required
              maxLength={254}
              className="px-3 py-2"
            />
          </label>
          <label className="flex flex-col gap-2">
            Password
            <input
              type="password"
              autoComplete="current-password"
              required
              minLength={15}
              maxLength={128}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              className="px-3 py-2"
            />
          </label>
          <button className={`${styles.button} px-4 py-2`} disabled={mutation.isPending}>
            {mutation.isPending ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
        {mutation.isError && <p role="alert">{errorMessage(mutation.error)}</p>}
      </section>
    </main>
  );
}

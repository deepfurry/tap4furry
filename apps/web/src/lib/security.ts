import { getCsrf } from '@tap4furry/api-client/public';

// Fetch for each mutation: no persisted token or cache can outlive a rotation.
export async function authenticatedRequest(): Promise<RequestInit> {
  const response = await getCsrf({ credentials: 'same-origin', cache: 'no-store' });
  if (response.status !== 200) {
    if (response.status === 401) window.location.assign('/login');
    throw new Error('Session verification unavailable');
  }
  return { credentials: 'same-origin', headers: { 'X-CSRF-Token': response.data.csrf_token } };
}

export function validPassword(password: string) {
  return [...password].length >= 15 && [...password].length <= 128;
}

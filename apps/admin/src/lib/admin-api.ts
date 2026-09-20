import { getCsrf, getMe } from '@tap4furry/api-client/admin';

export class AdminRequestError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly code?: string,
  ) {
    super(message);
  }
}
export const errorMessage = (error: unknown) =>
  error instanceof AdminRequestError
    ? error.message
    : 'Admin API is unavailable. Please try again.';
export const readOptions = { credentials: 'same-origin', cache: 'no-store' } as const;
export function result<T>(response: {
  status: number;
  data: T | { code: string; message: string };
}): T {
  if (response.status >= 400) {
    const error = response.data as { code: string; message: string };
    throw new AdminRequestError(response.status, error.message, error.code);
  }
  return response.data as T;
}
export async function requireAdmin() {
  const response = await getMe(readOptions);
  if (response.status !== 200)
    throw new AdminRequestError(response.status, response.data.message, response.data.code);
  return response.data;
}
export async function writeOptions(): Promise<RequestInit> {
  const response = await getCsrf(readOptions);
  if (response.status !== 200)
    throw new AdminRequestError(response.status, response.data.message, response.data.code);
  return { ...readOptions, headers: { 'X-CSRF-Token': response.data.csrf_token } };
}

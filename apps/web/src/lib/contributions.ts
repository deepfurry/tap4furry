import { useEffect } from 'react';
export const privateRead = {
  credentials: 'same-origin',
  cache: 'no-store',
} as const;
export class ContributionError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}
export function checked<T>(response: {
  status: number;
  data: T | { code: string; message: string };
}): T {
  if (response.status >= 400) {
    if (response.status === 401) window.location.assign('/login');
    throw new ContributionError(
      response.status,
      (response.data as { message: string }).message,
    );
  }
  return response.data as T;
}
export const contributionError = (error: unknown) =>
  error instanceof ContributionError
    ? error.message
    : 'The service is unavailable. Your input is still here; please try again.';
// Astro performs document navigation: beforeunload covers links, back and refresh.
export function useContributionDirty(dirty: boolean) {
  useEffect(() => {
    if (!dirty) return;
    const leave = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    window.addEventListener('beforeunload', leave);
    return () => window.removeEventListener('beforeunload', leave);
  }, [dirty]);
}

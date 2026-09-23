import { useRef } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { AdminRequestError, errorMessage } from '../../lib/admin-api';
import { Button } from '../../components/admin/Primitives';
import { keys } from '../../lib/query-keys';
export function useRequestKey() {
  const value = useRef({ signature: '', id: '' });
  return (input: unknown) => {
    const signature = JSON.stringify(input);
    if (value.current.signature !== signature)
      value.current = { signature, id: crypto.randomUUID() };
    return value.current.id;
  };
}
export function useGovernanceMutation<T = void>(
  run: (input: T) => Promise<unknown>,
  saved?: () => void | Promise<void>,
) {
  const client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: run,
    onSuccess: async () => {
      await saved?.();
      await Promise.all(
        [
          ['reports'],
          ['governance-users'],
          ['source-health'],
          ['audit'],
          keys.resources,
          ['contributions'],
        ].map((queryKey) => client.invalidateQueries({ queryKey }, { throwOnError: true })),
      );
    },
  });
}
export function DecisionStatus({
  error,
  success,
  reload,
}: {
  error: unknown;
  success: boolean;
  reload: () => void;
}) {
  return (
    <>
      {error && (
        <div role="alert">
          <p>{errorMessage(error)}</p>
          {error instanceof AdminRequestError && error.status === 409 && (
            <>
              <p>Your input is preserved. Review the current snapshot before submitting again.</p>
              <Button type="button" onClick={reload}>
                Reload current snapshot
              </Button>
            </>
          )}
        </div>
      )}
      {success && <p role="status">Saved. The canonical snapshot has been refreshed.</p>}
    </>
  );
}
export function Pages({
  page,
  hasNext,
  onPage,
}: {
  page: number;
  hasNext: boolean;
  onPage: (page: number) => void;
}) {
  return (
    <nav className="flex gap-4" aria-label="Result pages">
      <Button disabled={page === 1} onClick={() => onPage(page - 1)}>
        Previous
      </Button>
      <span>Page {page}</span>
      <Button disabled={!hasNext} onClick={() => onPage(page + 1)}>
        Next
      </Button>
    </nav>
  );
}

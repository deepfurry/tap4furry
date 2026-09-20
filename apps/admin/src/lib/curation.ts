import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useBlocker } from '@tanstack/react-router';
import { keys } from './query-keys';

export function useCanonicalMutation<T>(
  operation: (input: T) => Promise<unknown>,
  saved?: () => void,
) {
  const client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: operation,
    onSuccess: async () => {
      saved?.();
      // Canonical side effects include normalized URLs, both relation revisions,
      // primary switches and publication time. Always refetch server snapshots.
      await Promise.all([
        client.invalidateQueries({ queryKey: keys.resources }),
        client.invalidateQueries({ queryKey: keys.taxonomy }),
      ]);
    },
  });
}
export function useDirtyGuard(dirty: boolean) {
  return useBlocker({ shouldBlockFn: () => dirty, enableBeforeUnload: dirty, withResolver: true });
}

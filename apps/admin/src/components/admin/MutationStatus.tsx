import { AdminRequestError, errorMessage } from '../../lib/admin-api';
import { Button } from './Primitives';
export function isVersionConflict(error: unknown) {
  return error instanceof AdminRequestError && error.code === 'RESOURCE_VERSION_CONFLICT';
}
export function MutationStatus({
  error,
  reload,
  success,
}: {
  error: unknown;
  reload?: () => void;
  success?: boolean;
}) {
  if (isVersionConflict(error))
    return (
      <aside role="alert" className="flex flex-col gap-3">
        <p>
          This Resource changed on the server. Your unsaved form has been preserved. Further saves
          are blocked.
        </p>
        <Button type="button" onClick={reload}>
          Reload latest version
        </Button>
      </aside>
    );
  if (error) return <p role="alert">{errorMessage(error)}</p>;
  return success ? <p role="status">Saved.</p> : null;
}

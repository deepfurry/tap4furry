import { useEffect, useRef } from 'react';
import type { useDirtyGuard } from '../../lib/curation';
import { Button } from './Primitives';
import styles from './Primitives.module.scss';
export function DirtyFormGuard({ blocker }: { blocker: ReturnType<typeof useDirtyGuard> }) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (blocker.status === 'blocked') dialog.current?.showModal();
    else dialog.current?.close();
  }, [blocker.status]);
  return (
    <dialog
      ref={dialog}
      className={`${styles.dialog} max-w-md p-6`}
      aria-label="Unsaved changes"
      onCancel={(event) => {
        event.preventDefault();
        blocker.reset?.();
      }}
    >
      <div className="flex flex-col gap-4">
        <h2>Unsaved changes</h2>
        <p>Leave this form and discard your unsaved changes?</p>
        <div className="flex gap-3">
          <Button onClick={() => blocker.reset?.()}>Keep editing</Button>
          <Button onClick={() => blocker.proceed?.()}>Discard and leave</Button>
        </div>
      </div>
    </dialog>
  );
}

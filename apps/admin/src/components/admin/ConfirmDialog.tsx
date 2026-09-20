import { useRef, useState } from 'react';
import { Button } from './Primitives';
import styles from './Primitives.module.scss';
export function ConfirmDialog({
  slug,
  disabled,
  onConfirm,
}: {
  slug: string;
  disabled?: boolean;
  onConfirm: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [typed, setTyped] = useState('');
  return (
    <>
      <Button
        type="button"
        disabled={disabled}
        onClick={() => {
          setTyped('');
          dialog.current?.showModal();
        }}
      >
        Soft-delete
      </Button>
      <dialog
        ref={dialog}
        className={`${styles.dialog} max-w-md p-6`}
        aria-labelledby="delete-heading"
      >
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            if (typed === slug) {
              dialog.current?.close();
              onConfirm();
            }
          }}
        >
          <h2 id="delete-heading">Confirm soft deletion</h2>
          <p>Restoration is not available from Admin yet.</p>
          <label className="flex flex-col gap-2">
            Type &quot;{slug}&quot; to confirm.
            <input
              autoComplete="off"
              value={typed}
              onChange={(event) => setTyped(event.target.value)}
            />
          </label>
          <div className="flex flex-wrap gap-3">
            <Button type="button" onClick={() => dialog.current?.close()}>
              Cancel
            </Button>
            <Button disabled={typed !== slug || disabled}>Confirm soft-delete</Button>
          </div>
        </form>
      </dialog>
    </>
  );
}

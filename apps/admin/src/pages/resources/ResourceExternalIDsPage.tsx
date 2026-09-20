import { useState } from 'react';
import { setResourceExternalIDs } from '@tap4furry/api-client/admin';
import { result, writeOptions } from '../../lib/admin-api';
import { externalIDsError } from '../../lib/external-ids';
import { useCanonicalMutation } from '../../lib/curation';
import { canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button } from '../../components/admin/Primitives';
import { MutationStatus, isVersionConflict } from '../../components/admin/MutationStatus';
import { useResource } from './ResourceLayout';
export function ResourceExternalIDsPage() {
  const { resource: r, reload } = useResource();
  const allowed = canEditorial(useRoles());
  const [items, setItems] = useState(
    r.external_ids.map((item, index) => ({ ...item, rowKey: index })),
  );
  const [nextKey, setNextKey] = useState(items.length);
  const mutation = useCanonicalMutation(async () =>
    result(
      await setResourceExternalIDs(
        r.id,
        { items: items.map(({ namespace, external_id }) => ({ namespace, external_id })) },
        { expected_version: r.version },
        await writeOptions(),
      ),
    ),
  );
  const validation = externalIDsError(items);
  return (
    <Panel title="External IDs">
      <p>
        IDs are globally unique within each namespace. Saving replaces this Resource’s entire set;
        identities are never transferred automatically.
      </p>
      <fieldset
        disabled={!allowed || mutation.isPending || isVersionConflict(mutation.error)}
        className="flex flex-col gap-4"
      >
        {items.map((item, index) => (
          <div key={item.rowKey} className="grid items-end gap-3 sm:grid-cols-[1fr_1fr_auto]">
            <Field label="Namespace">
              <input
                value={item.namespace}
                maxLength={64}
                onChange={(event) =>
                  setItems(
                    items.map((row, i) =>
                      i === index ? { ...row, namespace: event.target.value } : row,
                    ),
                  )
                }
              />
            </Field>
            <Field label="External ID">
              <input
                value={item.external_id}
                maxLength={512}
                onChange={(event) =>
                  setItems(
                    items.map((row, i) =>
                      i === index ? { ...row, external_id: event.target.value } : row,
                    ),
                  )
                }
              />
            </Field>
            <Button onClick={() => setItems(items.filter((_, i) => i !== index))}>
              Remove row
            </Button>
          </div>
        ))}
        <div className="flex flex-wrap gap-3">
          <Button
            onClick={() => {
              setItems([...items, { namespace: '', external_id: '', rowKey: nextKey }]);
              setNextKey(nextKey + 1);
            }}
          >
            Add row
          </Button>
          <Button disabled={!!validation} onClick={() => mutation.mutate()}>
            Save set
          </Button>
        </div>
      </fieldset>
      {validation && <p role="status">{validation}</p>}
      <MutationStatus
        error={mutation.error}
        success={mutation.isSuccess}
        reload={() => {
          void reload();
        }}
      />
    </Panel>
  );
}

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { listTags, setResourceTags } from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions, errorMessage } from '../../lib/admin-api';
import { keys } from '../../lib/query-keys';
import { useCanonicalMutation } from '../../lib/curation';
import { canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Button, Badge } from '../../components/admin/Primitives';
import { MutationStatus, isVersionConflict } from '../../components/admin/MutationStatus';
import { useResource } from './ResourceLayout';
export function ResourceTagsPage() {
  const { resource: r, reload } = useResource();
  const allowed = canEditorial(useRoles());
  const [selected, setSelected] = useState(r.tags.map((tag) => tag.id));
  const query = useQuery({
    queryKey: keys.tags,
    queryFn: async () => result(await listTags(readOptions)),
    retry: false,
  });
  const mutation = useCanonicalMutation(async () =>
    result(
      await setResourceTags(
        r.id,
        { tag_ids: selected },
        { expected_version: r.version },
        await writeOptions(),
      ),
    ),
  );
  const items = [
    ...r.tags,
    ...(query.data?.items.filter((tag) => !r.tags.some((current) => current.id === tag.id)) ?? []),
  ].filter((tag) => tag.state === 'active' || r.tags.some((current) => current.id === tag.id));
  return (
    <Panel title="Resource tags">
      <p>
        Existing retired tags may remain. Once a retired binding is saved as removed, it cannot be
        added again.
      </p>
      {query.error && <p role="alert">{errorMessage(query.error)}</p>}
      <fieldset
        disabled={!allowed || mutation.isPending || isVersionConflict(mutation.error)}
        className="flex flex-col gap-4"
      >
        {items.map((tag) => (
          <label key={tag.id} className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={selected.includes(tag.id)}
              onChange={(event) =>
                setSelected(
                  event.target.checked
                    ? [...selected, tag.id]
                    : selected.filter((id) => id !== tag.id),
                )
              }
            />
            {tag.name} {tag.state === 'retired' && <Badge>Retired</Badge>}
          </label>
        ))}
        <Button onClick={() => mutation.mutate()}>Save tag set</Button>
      </fieldset>
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

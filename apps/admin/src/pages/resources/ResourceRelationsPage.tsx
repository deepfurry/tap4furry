import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  addRelation,
  deleteRelation,
  listResources,
  type RelationType,
  type ResourceListItem,
} from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions, errorMessage } from '../../lib/admin-api';
import { useCanonicalMutation } from '../../lib/curation';
import { canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button, EmptyState } from '../../components/admin/Primitives';
import { MutationStatus, isVersionConflict } from '../../components/admin/MutationStatus';
import { useResource } from './ResourceLayout';
export function ResourceRelationsPage() {
  const { resource: r, reload } = useResource();
  const allowed = canEditorial(useRoles());
  const [slug, setSlug] = useState('');
  const [target, setTarget] = useState<ResourceListItem | null>(null);
  const [type, setType] = useState<RelationType>('related_to');
  const lookup = useMutation({
    retry: false,
    mutationFn: async () => {
      const response = result(
        await listResources({ slug: slug.trim(), page: 1, page_size: 1 }, readOptions),
      );
      setTarget(response.items[0] ?? null);
      return response;
    },
  });
  const mutation = useCanonicalMutation(async (relationId?: string) =>
    relationId
      ? result(
          await deleteRelation(
            r.id,
            relationId,
            { expected_version: r.version },
            await writeOptions(),
          ),
        )
      : result(
          await addRelation(
            r.id,
            { target_resource_id: target!.id, relation_type: type },
            { expected_version: r.version },
            await writeOptions(),
          ),
        ),
  );
  const disabled = !allowed || mutation.isPending || isVersionConflict(mutation.error);
  return (
    <>
      <Panel title="Relations">
        {r.relations.length === 0 && <EmptyState>No relations yet.</EmptyState>}
        <ul className="flex flex-col gap-4">
          {r.relations.map((item) => (
            <li key={item.id} className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <span aria-label={item.direction}>
                  {item.direction === 'incoming' ? '←' : item.direction === 'outgoing' ? '→' : '↔'}
                </span>{' '}
                {item.relation_type}{' '}
                <Link to="/resources/$resourceId" params={{ resourceId: item.other.id }}>
                  {item.other.name}
                </Link>{' '}
                <small>({item.other.publication_state})</small>
              </div>
              <Button
                disabled={disabled}
                onClick={() => {
                  if (window.confirm('Remove this relation from both Resources?'))
                    mutation.mutate(item.id);
                }}
              >
                Remove relation
              </Button>
            </li>
          ))}
        </ul>
      </Panel>
      <Panel title="Add relation">
        <form
          className="grid items-end gap-4 sm:grid-cols-3"
          onSubmit={(event) => {
            event.preventDefault();
            lookup.mutate();
          }}
        >
          <Field label="Relation type">
            <select
              value={type}
              disabled={disabled}
              onChange={(event) => setType(event.target.value as RelationType)}
            >
              {['related_to', 'part_of', 'successor_of', 'derived_from'].map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          </Field>
          <Field label="Target exact slug">
            <input
              required
              maxLength={80}
              value={slug}
              disabled={disabled}
              onChange={(event) => {
                setSlug(event.target.value);
                setTarget(null);
              }}
            />
          </Field>
          <Button disabled={disabled || lookup.isPending}>Lookup exact slug</Button>
        </form>
        {lookup.error && <p role="alert">{errorMessage(lookup.error)}</p>}
        {lookup.isSuccess && !target && <p>No Resource with that exact slug.</p>}
        {target && (
          <div className="flex flex-wrap items-center gap-4">
            <p>
              Target: <strong>{target.name}</strong> / {target.slug} ({target.publication_state})
            </p>
            <Button
              disabled={disabled || target.id === r.id}
              onClick={() => mutation.mutate(undefined)}
            >
              Confirm & add relation
            </Button>
          </div>
        )}
      </Panel>
      <MutationStatus
        error={mutation.error}
        reload={() => {
          void reload();
        }}
      />
    </>
  );
}

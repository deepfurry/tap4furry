import { useState } from 'react';
import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import {
  createSource,
  patchSource,
  setSourceRights,
  type CreateSource,
  type RightsStatus,
  type Source,
} from '@tap4furry/api-client/admin';
import { result, writeOptions } from '../../lib/admin-api';
import { useCanonicalMutation, useDirtyGuard } from '../../lib/curation';
import { canAdministrate, canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button } from '../../components/admin/Primitives';
import { MutationStatus, isVersionConflict } from '../../components/admin/MutationStatus';
import { useResource } from './ResourceLayout';
export function ResourceSourcesPage() {
  const { resource: r, reload } = useResource();
  const roles = useRoles();
  const [dirty, setDirty] = useState(false);
  const blocker = useDirtyGuard(dirty);
  const mutation = useCanonicalMutation(
    async (action: {
      source?: string;
      body?: CreateSource;
      rights?: RightsStatus;
      reason?: string;
    }) => {
      const params = { expected_version: r.version };
      const options = await writeOptions();
      if (action.rights)
        return result(
          await setSourceRights(
            r.id,
            action.source!,
            { rights_status: action.rights, reason: action.reason! },
            params,
            options,
          ),
        );
      return action.source
        ? result(await patchSource(r.id, action.source, action.body!, params, options))
        : result(await createSource(r.id, action.body!, params, options));
    },
    () => setDirty(false),
  );
  const disabled = mutation.isPending || isVersionConflict(mutation.error);
  return (
    <div onChange={() => setDirty(true)}>
      <DirtyFormGuard blocker={blocker} />
      <p>
        Sources are retained as knowledge. Set availability to Removed to hide a Source from Public.
      </p>
      {r.sources.map((source) => (
        <Panel key={source.id} title={source.label || source.url}>
          <SourceForm
            source={source}
            disabled={disabled || !canEditorial(roles)}
            save={(body) => mutation.mutate({ source: source.id, body })}
          />
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              const data = new FormData(event.currentTarget);
              mutation.mutate({
                source: source.id,
                rights: String(data.get('rights')) as RightsStatus,
                reason: String(data.get('reason')),
              });
            }}
          >
            <Field label="Rights (Administration)">
              <select
                name="rights"
                defaultValue={source.rights_status}
                disabled={!canAdministrate(roles) || disabled}
              >
                {[
                  'unknown',
                  'creator_provided',
                  'confirmed',
                  'disputed',
                  'rights_review',
                  'removed_by_request',
                ].map((v) => (
                  <option key={v}>{v}</option>
                ))}
              </select>
            </Field>
            <Field label="Governance reason">
              <input
                name="reason"
                required
                maxLength={1000}
                disabled={!canAdministrate(roles) || disabled}
              />
            </Field>
            <Button disabled={!canAdministrate(roles) || disabled}>Update rights</Button>
          </form>
        </Panel>
      ))}
      <Panel title="Add source">
        <SourceForm
          disabled={disabled || !canEditorial(roles)}
          save={(body) => mutation.mutate({ body })}
        />
      </Panel>
      <MutationStatus
        error={mutation.error}
        success={mutation.isSuccess}
        reload={() => {
          void reload();
        }}
      />
    </div>
  );
}
function SourceForm({
  source,
  disabled,
  save,
}: {
  source?: Source;
  disabled: boolean;
  save: (body: CreateSource) => void;
}) {
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        save({
          url: String(data.get('url')),
          label: String(data.get('label')) || null,
          source_type: String(data.get('type')) as CreateSource['source_type'],
          availability_state: String(
            data.get('availability'),
          ) as CreateSource['availability_state'],
          is_primary: data.get('primary') === 'on',
        });
      }}
    >
      <fieldset disabled={disabled} className="grid gap-4 sm:grid-cols-2">
        <Field label="URL">
          <input type="url" name="url" defaultValue={source?.url} required maxLength={2048} />
        </Field>
        <Field label="Label">
          <input name="label" defaultValue={source?.label ?? ''} maxLength={80} />
        </Field>
        <Field label="Type">
          <select name="type" defaultValue={source?.source_type ?? 'unknown'}>
            {['official', 'store', 'archive', 'mirror', 'community', 'external', 'unknown'].map(
              (v) => (
                <option key={v}>{v}</option>
              ),
            )}
          </select>
        </Field>
        <Field label="Availability">
          <select name="availability" defaultValue={source?.availability_state ?? 'active'}>
            {['active', 'unavailable', 'broken', 'removed', 'restricted'].map((v) => (
              <option key={v}>{v}</option>
            ))}
          </select>
        </Field>
        <label className="flex items-center gap-3">
          <input type="checkbox" name="primary" defaultChecked={source?.is_primary ?? false} />
          Primary Source
        </label>
        <Button>{source ? 'Save source' : 'Add source'}</Button>
      </fieldset>
    </form>
  );
}

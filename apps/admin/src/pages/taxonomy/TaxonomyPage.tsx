import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import { useState } from 'react';
import { flushSync } from 'react-dom';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from '@tanstack/react-router';
import {
  getCategory,
  getTag,
  patchCategory,
  patchTag,
  deleteCategory,
  deleteTag,
  putCategoryLocalization,
  putTagLocalization,
  deleteCategoryLocalization,
  deleteTagLocalization,
  type TaxonomyDetail,
} from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions, errorMessage } from '../../lib/admin-api';
import { keys } from '../../lib/query-keys';
import { useCanonicalMutation, useDirtyGuard } from '../../lib/curation';
import { canAdministrate, canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button, Badge } from '../../components/admin/Primitives';
import { ConfirmDialog } from '../../components/admin/ConfirmDialog';
import { MutationStatus } from '../../components/admin/MutationStatus';
import type { TaxonomyKind } from './TaxonomyListPage';
export function TaxonomyPage({ kind }: { kind: TaxonomyKind }) {
  const params = useParams({ strict: false });
  const id = kind === 'categories' ? params.categoryId! : params.tagId!;
  const query = useQuery({
    queryKey: keys.entity(kind, id),
    queryFn: async () =>
      result<TaxonomyDetail>(await (kind === 'categories' ? getCategory : getTag)(id, readOptions)),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  if (query.error) return <p role="alert">{errorMessage(query.error)}</p>;
  if (!query.data) return <p role="status">Loading taxonomy…</p>;
  return (
    <TaxonomyEditor
      key={`${query.data.id}:${query.data.updated_at}`}
      kind={kind}
      entity={query.data}
    />
  );
}
function TaxonomyEditor({ kind, entity: e }: { kind: TaxonomyKind; entity: TaxonomyDetail }) {
  const roles = useRoles();
  const navigate = useNavigate();
  const [locale, setLocale] = useState(e.default_locale);
  const [adding, setAdding] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [reason, setReason] = useState('');
  const blocker = useDirtyGuard(dirty || !!reason);
  const selected = e.localizations.find((item) => item.locale === locale);
  const mutation = useCanonicalMutation(
    async (action: {
      kind: 'save' | 'default' | 'delete-localization' | 'state' | 'delete';
      locale?: string;
      name?: string;
      description?: string;
    }) => {
      const options = await writeOptions();
      if (action.kind === 'save')
        return result(
          await (kind === 'categories' ? putCategoryLocalization : putTagLocalization)(
            e.id,
            action.locale!,
            { name: action.name!, description: action.description || null },
            options,
          ),
        );
      if (action.kind === 'default')
        return result(
          await (kind === 'categories' ? patchCategory : patchTag)(
            e.id,
            { default_locale: action.locale },
            options,
          ),
        );
      if (action.kind === 'state')
        return result(
          await (kind === 'categories' ? patchCategory : patchTag)(
            e.id,
            { state: e.state === 'active' ? 'retired' : 'active', reason: reason.trim() },
            options,
          ),
        );
      if (action.kind === 'delete-localization')
        return result(
          await (kind === 'categories' ? deleteCategoryLocalization : deleteTagLocalization)(
            e.id,
            action.locale!,
            options,
          ),
        );
      result(
        await (kind === 'categories' ? deleteCategory : deleteTag)(
          e.id,
          { reason: reason.trim() },
          options,
        ),
      );
      flushSync(() => {
        setDirty(false);
        setReason('');
      });
      await navigate({ to: kind === 'categories' ? '/taxonomy/categories' : '/taxonomy/tags' });
    },
    () => setDirty(false),
  );
  function choose(next: string, add = false) {
    if (dirty && !window.confirm('Discard your unsaved changes?')) return;
    setDirty(false);
    setLocale(next);
    setAdding(add);
  }
  return (
    <>
      <DirtyFormGuard blocker={blocker} />
      <header className="flex flex-col gap-3">
        <h1>{e.slug}</h1>
        <p>Slug is immutable.</p>
        <div>
          <Badge>{e.state}</Badge> Default: {e.default_locale}
        </div>
      </header>
      <Panel title="Localizations">
        <div className="flex flex-wrap gap-3">
          {e.localizations.map((item) => (
            <Button key={item.locale} onClick={() => choose(item.locale)}>
              {item.locale}
              {item.locale === e.default_locale ? ' · Default' : ''}
            </Button>
          ))}
          <Button disabled={!canEditorial(roles)} onClick={() => choose('', true)}>
            Add localization
          </Button>
        </div>
        <form
          key={adding ? 'new' : locale}
          onChange={() => setDirty(true)}
          onSubmit={(event) => {
            event.preventDefault();
            const data = new FormData(event.currentTarget);
            mutation.mutate({
              kind: 'save',
              locale: adding ? String(data.get('locale')) : locale,
              name: String(data.get('name')),
              description: String(data.get('description')),
            });
          }}
        >
          <fieldset
            disabled={!canEditorial(roles) || mutation.isPending}
            className="flex flex-col gap-4"
          >
            {adding && (
              <Field label="Locale">
                <input name="locale" required maxLength={64} />
              </Field>
            )}
            <Field label="Name">
              <input name="name" defaultValue={selected?.name ?? ''} required maxLength={80} />
            </Field>
            <Field label="Description">
              <textarea
                name="description"
                defaultValue={selected?.description ?? ''}
                maxLength={500}
              />
            </Field>
            <Button>Save localization</Button>
          </fieldset>
        </form>
        {!adding && locale !== e.default_locale && (
          <div className="flex gap-3">
            <Button
              disabled={!canEditorial(roles) || dirty || mutation.isPending}
              onClick={() => mutation.mutate({ kind: 'default', locale })}
            >
              Make default
            </Button>
            <Button
              disabled={!canEditorial(roles) || dirty || mutation.isPending}
              onClick={() => {
                if (window.confirm('Delete this localization?'))
                  mutation.mutate({ kind: 'delete-localization', locale });
              }}
            >
              Delete localization
            </Button>
          </div>
        )}
      </Panel>
      <Panel title="Governance">
        <Field label="Governance reason">
          <textarea
            maxLength={1000}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            disabled={!canAdministrate(roles)}
          />
        </Field>
        <p>
          Administration capability required. Retired taxonomy keeps existing bindings and cannot
          accept new bindings.
        </p>
        <Button
          disabled={!canAdministrate(roles) || dirty || mutation.isPending || !reason.trim()}
          onClick={() => mutation.mutate({ kind: 'state' })}
        >
          {e.state === 'active' ? 'Retire' : 'Reactivate'}
        </Button>
      </Panel>
      {canAdministrate(roles) && (
        <Panel title="Danger Zone">
          <p>Soft deletion is blocked while any non-deleted Resource uses this taxonomy.</p>
          <ConfirmDialog
            slug={e.slug}
            disabled={dirty || mutation.isPending || !reason.trim()}
            onConfirm={() => mutation.mutate({ kind: 'delete' })}
          />
        </Panel>
      )}
      <MutationStatus error={mutation.error} success={mutation.isSuccess} />
    </>
  );
}

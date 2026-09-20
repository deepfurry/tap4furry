import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import { useState } from 'react';
import {
  deleteResourceLocalization,
  patchResource,
  putResourceLocalization,
} from '@tap4furry/api-client/admin';
import { result, writeOptions } from '../../lib/admin-api';
import { useCanonicalMutation, useDirtyGuard } from '../../lib/curation';
import { canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button, Badge } from '../../components/admin/Primitives';
import { MutationStatus, isVersionConflict } from '../../components/admin/MutationStatus';
import { useResource } from './ResourceLayout';
export function ResourceLocalizationsPage() {
  const { resource: r, reload } = useResource();
  const allowed = canEditorial(useRoles());
  const [locale, setLocale] = useState(r.default_locale);
  const [adding, setAdding] = useState(false);
  const [dirty, setDirty] = useState(false);
  const blocker = useDirtyGuard(dirty);
  const selected = r.localizations.find((item) => item.locale === locale);
  const mutation = useCanonicalMutation(
    async (action: {
      kind: 'save' | 'delete' | 'default';
      locale: string;
      name?: string;
      summary?: string;
      description?: string;
    }) => {
      const options = await writeOptions();
      const params = { expected_version: r.version };
      if (action.kind === 'delete')
        return result(await deleteResourceLocalization(r.id, action.locale, params, options));
      if (action.kind === 'default')
        return result(
          await patchResource(r.id, { default_locale: action.locale }, params, options),
        );
      return result(
        await putResourceLocalization(
          r.id,
          action.locale,
          {
            name: action.name!,
            summary: action.summary || null,
            description: action.description || null,
          },
          params,
          options,
        ),
      );
    },
    () => setDirty(false),
  );
  const blocked = isVersionConflict(mutation.error);
  function choose(next: string, add = false) {
    if (dirty && !window.confirm('Discard your unsaved changes?')) return;
    setDirty(false);
    setAdding(add);
    setLocale(next);
  }
  return (
    <>
      <DirtyFormGuard blocker={blocker} />
      <div className="grid gap-5 md:grid-cols-[10rem_1fr]">
        <aside className="flex flex-col items-start gap-3">
          {r.localizations.map((item) => (
            <Button
              key={item.locale}
              type="button"
              disabled={blocked}
              onClick={() => choose(item.locale)}
            >
              {item.locale} {item.locale === r.default_locale && <Badge>Default</Badge>}
            </Button>
          ))}
          <Button disabled={!allowed || blocked} onClick={() => choose('', true)}>
            Add localization
          </Button>
        </aside>
        <Panel title={adding ? 'Add localization' : `Edit ${locale}`}>
          <form
            key={adding ? 'new' : locale}
            className="flex flex-col gap-5"
            onChange={() => setDirty(true)}
            onSubmit={(event) => {
              event.preventDefault();
              const data = new FormData(event.currentTarget);
              mutation.mutate({
                kind: 'save',
                locale: adding ? String(data.get('locale')) : locale,
                name: String(data.get('name')),
                summary: String(data.get('summary')),
                description: String(data.get('description')),
              });
            }}
          >
            <fieldset
              disabled={!allowed || blocked || mutation.isPending}
              className="flex flex-col gap-5"
            >
              {adding && (
                <Field label="Locale">
                  <input name="locale" required maxLength={64} placeholder="zh-Hans" />
                </Field>
              )}
              <Field label="Name">
                <input name="name" required maxLength={160} defaultValue={selected?.name ?? ''} />
              </Field>
              <Field label="Summary">
                <textarea name="summary" maxLength={500} defaultValue={selected?.summary ?? ''} />
              </Field>
              <Field
                label="Description"
                hint="Markdown supported. Raw HTML and embedded images are not supported. Up to 50,000 Unicode characters."
              >
                <textarea name="description" rows={14} defaultValue={selected?.description ?? ''} />
              </Field>
              <Button>Save localization</Button>
            </fieldset>
          </form>
          {!adding && locale !== r.default_locale && (
            <div className="flex flex-wrap gap-3">
              <Button
                disabled={!allowed || blocked || dirty || mutation.isPending}
                onClick={() => mutation.mutate({ kind: 'default', locale })}
              >
                Make default
              </Button>
              <Button
                disabled={!allowed || blocked || dirty || mutation.isPending}
                onClick={() => {
                  if (window.confirm(`Delete the ${locale} localization?`))
                    mutation.mutate({ kind: 'delete', locale });
                }}
              >
                Delete localization
              </Button>
            </div>
          )}
        </Panel>
      </div>
      <MutationStatus
        error={mutation.error}
        success={mutation.isSuccess}
        reload={() => {
          setDirty(false);
          void reload();
        }}
      />
    </>
  );
}

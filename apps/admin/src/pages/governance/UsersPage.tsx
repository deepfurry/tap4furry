import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from '@tanstack/react-router';
import {
  getUserGovernance,
  updateUserTrust,
  createRestriction,
  revokeRestriction,
  type UserGovernance,
  type TrustUpdate,
  type RestrictionCreate,
} from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions, errorMessage, requireAdmin } from '../../lib/admin-api';
import { canAdministrate } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { useDirtyGuard } from '../../lib/curation';
import { keys } from '../../lib/query-keys';
import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import { Panel, Field, Button } from '../../components/admin/Primitives';
import { DecisionStatus, useGovernanceMutation, useRequestKey } from './shared';
export function UsersPage() {
  const { userId = '' } = useParams({ strict: false });
  const allowed = canAdministrate(useRoles());
  const navigate = useNavigate();
  const [lookup, setLookup] = useState('');
  const q = useQuery({
    queryKey: ['governance-users', userId],
    queryFn: async () => result(await getUserGovernance(userId, readOptions)),
    enabled: allowed && !!userId,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  if (!allowed) return <p>Administration capability is required.</p>;
  return (
    <>
      <h1>User business governance</h1>
      <Panel title="Exact account lookup">
        <p>
          Use the User ID from an authorized contribution or report. This does not search email or
          alter authentication roles.
        </p>
        <form
          className="flex gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            void navigate({ to: '/governance/users/$userId', params: { userId: lookup } });
          }}
        >
          <Field label="User ID">
            <input
              required
              pattern="[0-9a-fA-F-]{36}"
              value={lookup}
              onChange={(e) => setLookup(e.target.value)}
            />
          </Field>
          <Button>Open account</Button>
        </form>
      </Panel>
      {q.error && <p role="alert">{errorMessage(q.error)}</p>}
      {q.data ? (
        <UserEditor
          key={userId}
          data={q.data}
          reload={async () => {
            await q.refetch();
          }}
        />
      ) : (
        userId && !q.error && <p role="status">Loading governance state…</p>
      )}
    </>
  );
}
function UserEditor({ data: d, reload }: { data: UserGovernance; reload: () => Promise<void> }) {
  const me = useQuery({ queryKey: keys.me, queryFn: requireAdmin, retry: false });
  const [action, setAction] = useState('trust'),
    [trust, setTrust] = useState<TrustUpdate['trust_level']>(d.trust_level),
    [scope, setScope] = useState<RestrictionCreate['scope']>('contribution_submit'),
    [duration, setDuration] = useState<RestrictionCreate['duration']>('7d'),
    [code, setCode] = useState<RestrictionCreate['reason_code']>('spam');
  const [message, setMessage] = useState(''),
    [reason, setReason] = useState(''),
    [note, setNote] = useState(''),
    [restriction, setRestriction] = useState(''),
    [replaces, setReplaces] = useState('');
  const [dirty, setDirty] = useState(false),
    [confirmed, setConfirmed] = useState(false);
  const blocker = useDirtyGuard(dirty);
  const request = useRequestKey();
  const active = d.restrictions.filter(
    (r) => !r.revoked_at && (!r.expires_at || new Date(r.expires_at).getTime() > Date.now()),
  );
  const mutation = useGovernanceMutation(
    async () => {
      const common = { expected_revision: d.revision };
      const options = await writeOptions();
      if (action === 'trust') {
        const payload = {
          ...common,
          trust_level: trust,
          reason: reason.trim(),
          ...(note.trim() && { internal_note: note.trim() }),
        };
        return result(
          await updateUserTrust(
            d.user_id,
            { ...payload, request_id: request([d.user_id, payload]) },
            options,
          ),
        );
      }
      if (action === 'revoke') {
        const payload = { ...common, reason: reason.trim() };
        return result(
          await revokeRestriction(
            d.user_id,
            restriction,
            { ...payload, request_id: request([d.user_id, restriction, payload]) },
            options,
          ),
        );
      }
      const payload = {
        ...common,
        scope,
        duration,
        reason_code: code,
        user_message: message.trim(),
        ...(note.trim() && { internal_note: note.trim() }),
        ...(replaces && { replaces_restriction_id: replaces }),
      };
      return result(
        await createRestriction(
          d.user_id,
          { ...payload, request_id: request([d.user_id, payload]) },
          options,
        ),
      );
    },
    () => {
      setDirty(false);
      setConfirmed(false);
      setReplaces('');
      setRestriction('');
    },
  );
  const self = me.data?.id === d.user_id;
  return (
    <>
      <DirtyFormGuard blocker={blocker} />
      <Panel title="Current business state">
        <p>
          User {d.user_id} · Configuration revision {d.revision}
        </p>
        <p>
          Internal trust: <strong>{d.trust_level}</strong>. This changes contribution limits, never
          Admin roles or review requirements.
        </p>
        {d.restrictions.length ? (
          <ul className="grid gap-4">
            {d.restrictions.map((r) => (
              <li key={r.id}>
                <strong>{r.scope}</strong>
                <p>{r.user_message}</p>
                <p>
                  {r.revoked_at
                    ? `Revoked ${new Date(r.revoked_at).toLocaleString()}`
                    : r.expires_at
                      ? `Expires ${new Date(r.expires_at).toLocaleString()}`
                      : 'Until revoked'}
                </p>
                {r.internal_note && <p>Internal: {r.internal_note}</p>}
                {r.revoke_reason && <p>Revocation reason: {r.revoke_reason}</p>}
              </li>
            ))}
          </ul>
        ) : (
          <p>No restrictions recorded.</p>
        )}
      </Panel>
      <Panel title="Change business policy">
        <p>
          Reporting, login, account security, personal history and withdrawal remain available.
          Disabling profile indexing is always allowed.
        </p>
        {self && <p>You cannot change your own trust or restrictions.</p>}
        <form
          className="grid gap-4"
          onChange={(e) => {
            setDirty(true);
            if (!(e.target instanceof HTMLInputElement && e.target.name === 'confirmation'))
              setConfirmed(false);
          }}
          onSubmit={(e) => {
            e.preventDefault();
            if (confirmed) mutation.mutate();
          }}
        >
          <fieldset disabled={self || mutation.isPending} className="grid gap-4">
            <Field label="Action">
              <select value={action} onChange={(e) => setAction(e.target.value)}>
                <option value="trust">Adjust trust</option>
                <option value="restrict">Add or replace a restriction</option>
                <option value="revoke">Revoke a restriction</option>
              </select>
            </Field>
            {action === 'trust' && (
              <Field label="Internal trust">
                <select
                  value={trust}
                  onChange={(e) => setTrust(e.target.value as TrustUpdate['trust_level'])}
                >
                  <option value="new">New: 10/day, 5 pending, 60 seconds</option>
                  <option value="established">Established: 30/day, 10 pending, 30 seconds</option>
                  <option value="trusted">Trusted: 60/day, 20 pending, 15 seconds</option>
                </select>
              </Field>
            )}
            {action === 'restrict' && (
              <>
                <Field label="Restricted scope">
                  <select
                    value={scope}
                    onChange={(e) => {
                      setScope(e.target.value as RestrictionCreate['scope']);
                      setReplaces('');
                    }}
                  >
                    <option value="contribution_submit">Contribution submission</option>
                    <option value="public_profile_write">Public profile changes</option>
                    <option value="all_write">Both business scopes</option>
                  </select>
                </Field>
                <Field label="Duration">
                  <select
                    value={duration}
                    onChange={(e) => setDuration(e.target.value as RestrictionCreate['duration'])}
                  >
                    <option value="24h">24 hours</option>
                    <option value="7d">7 days</option>
                    <option value="30d">30 days</option>
                    <option value="indefinite">Until revoked</option>
                  </select>
                </Field>
                <Field label="Reason code">
                  <select
                    value={code}
                    onChange={(e) => setCode(e.target.value as RestrictionCreate['reason_code'])}
                  >
                    {['spam', 'abuse', 'repeated_policy_violation', 'other'].map((v) => (
                      <option key={v}>{v}</option>
                    ))}
                  </select>
                </Field>
                <Field label="Replace an existing active restriction">
                  <select value={replaces} onChange={(e) => setReplaces(e.target.value)}>
                    <option value="">Create new</option>
                    {active
                      .filter((r) => r.scope === scope)
                      .map((r) => (
                        <option key={r.id} value={r.id}>
                          {r.scope} · {new Date(r.starts_at).toLocaleString()}
                        </option>
                      ))}
                  </select>
                </Field>
                <Field label="Message the user will see">
                  <textarea
                    required
                    maxLength={1000}
                    value={message}
                    onChange={(e) => setMessage(e.target.value)}
                  />
                </Field>
                <p className="whitespace-pre-wrap">
                  User message preview: {message || 'Enter a clear explanation.'}
                </p>
              </>
            )}
            {action === 'revoke' && (
              <Field label="Active restriction">
                <select
                  required
                  value={restriction}
                  onChange={(e) => setRestriction(e.target.value)}
                >
                  <option value="">Select explicitly</option>
                  {active.map((r) => (
                    <option value={r.id} key={r.id}>
                      {r.scope} · {new Date(r.starts_at).toLocaleString()}
                    </option>
                  ))}
                </select>
              </Field>
            )}
            {action !== 'restrict' && (
              <Field label="Reason">
                <textarea
                  required
                  maxLength={1000}
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                />
              </Field>
            )}
            {action !== 'revoke' && (
              <Field label="Internal note">
                <textarea maxLength={2000} value={note} onChange={(e) => setNote(e.target.value)} />
              </Field>
            )}
            <label>
              <input
                name="confirmation"
                type="checkbox"
                checked={confirmed}
                onChange={(e) => setConfirmed(e.target.checked)}
              />{' '}
              I reviewed the account, scope and reason.
            </label>
            <Button disabled={!confirmed}>Apply policy change</Button>
          </fieldset>
        </form>
        <DecisionStatus
          error={mutation.error}
          success={mutation.isSuccess}
          reload={() => {
            setConfirmed(false);
            void reload();
          }}
        />
      </Panel>
    </>
  );
}

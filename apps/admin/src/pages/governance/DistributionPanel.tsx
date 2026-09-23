import { useState } from 'react';
import {
  updateResourceDistribution,
  type ResourceDetail,
  type DistributionUpdate,
} from '@tap4furry/api-client/admin';
import { result, writeOptions } from '../../lib/admin-api';
import { useDirtyGuard } from '../../lib/curation';
import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import { Panel, Field, Button } from '../../components/admin/Primitives';
import { DecisionStatus, useGovernanceMutation, useRequestKey } from './shared';
export function DistributionPanel({
  resource: r,
  reload,
}: {
  resource: ResourceDetail;
  reload: () => Promise<unknown>;
}) {
  const [policy, setPolicy] = useState<DistributionUpdate['policy']>(r.distribution_policy),
    [reason, setReason] = useState(''),
    [dirty, setDirty] = useState(false),
    [confirmed, setConfirmed] = useState(false);
  const request = useRequestKey(),
    blocker = useDirtyGuard(dirty);
  const mutation = useGovernanceMutation(
    async () => {
      const payload = { expected_version: r.version, policy, reason: reason.trim() };
      return result(
        await updateResourceDistribution(
          r.id,
          { ...payload, request_id: request([r.id, payload]) },
          await writeOptions(),
        ),
      );
    },
    () => {
      setDirty(false);
      setConfirmed(false);
    },
  );
  return (
    <>
      <DirtyFormGuard blocker={blocker} />
      <Panel title="Recommendation policy">
        <p>
          Current: {r.distribution_policy}. Exclusion affects active recommendation eligibility, not
          ordinary public browsing. This does not certify safety or rights.
        </p>
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
          <fieldset className="grid gap-4" disabled={mutation.isPending}>
            <Field label="Policy">
              <select
                value={policy}
                onChange={(e) => setPolicy(e.target.value as DistributionUpdate['policy'])}
              >
                <option value="normal">Normal eligibility rules</option>
                <option value="excluded">Exclude from active recommendation</option>
              </select>
            </Field>
            <Field label="Governance reason">
              <textarea
                required
                maxLength={1000}
                value={reason}
                onChange={(e) => setReason(e.target.value)}
              />
            </Field>
            <label>
              <input
                name="confirmation"
                type="checkbox"
                checked={confirmed}
                onChange={(e) => setConfirmed(e.target.checked)}
              />{' '}
              Apply this policy to Resource version {r.version}.
            </label>
            <Button disabled={!confirmed}>Save recommendation policy</Button>
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

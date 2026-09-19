import { useState } from 'react';
import { getReady } from '@tap4furry/api-client/public';
import styles from './HealthCheck.module.scss';

export default function HealthCheck() {
  const [status, setStatus] = useState('Not checked');
  const [pending, setPending] = useState(false);
  async function check() {
    setPending(true);
    try {
      const response = await getReady();
      setStatus(`Public API: ${response.data.status}`);
    } catch { setStatus('Public API unavailable'); }
    finally { setPending(false); }
  }
  return <div className="flex flex-wrap items-center gap-4">
    <button className={`${styles.button} px-4 py-2`} disabled={pending} onClick={() => { void check(); }}>
      {pending ? 'Checking…' : 'Check API readiness'}
    </button>
    <span className={styles.status} role="status">{status}</span>
  </div>;
}

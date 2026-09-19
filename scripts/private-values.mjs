// Collect private material only in memory. Callers report paths, never values.
export function privateEnvValues(env) {
  const values = new Set();
  for (const [key, value] of Object.entries(env)) {
    if (/(PASSWORD|SECRET|TOKEN|DSN|DATABASE_URL|REDIS_URL|OAUTH_CLIENT_ID|(?:^|_)API_KEY$|RESEND_TEST_RECIPIENT|TEST_EMAIL)/i.test(key) && value.length >= 8) values.add(value);
    try {
      const url = new URL(value);
      if (url.password.length >= 8) { values.add(url.password); values.add(decodeURIComponent(url.password)); }
      if (url.hostname && !['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)) values.add(url.hostname);
    } catch { /* Non-URL configuration has no connection hostname. */ }
  }
  return values;
}

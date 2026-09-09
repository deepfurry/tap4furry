# Architecture contract

- One Go module, three runtime processes: Public API, Admin API, Worker.
- Public/Admin remain separate transport/security boundaries; no inter-process HTTP
  calls to share business behavior. Composition belongs in `cmd/*`.
- Application owns transactions; Domain stays ordinary Go without transport
  DTOs, Fiber, Redis or River. No ceremonial service/repository wrappers around sqlc.
- River imports and types stay under `server/internal/jobs`.
- PostgreSQL is canonical; Redis is ephemeral and all created keys use `gfp:`.
- OpenAPI is spec-first. Public adds local authentication, `/me` and profiles in
  P0-1A and session/verification/recovery APIs in P0-1B. P0-1C adds explicit OAuth;
  P0-1D adds independent password-only Admin authentication and static roles.
  Both expose `/health/live` and `/health/ready`.
  Liveness never probes dependencies. Readiness requires PostgreSQL; Redis failure
  reports degraded state while PostgreSQL remains ready.
- Go 1.27+, Node 24, pnpm workspace; Astro/React 19 public SSR, React 19/Vite admin.
  Tailwind v4 handles layout; SCSS handles appearance.
- Only `auth` and `identity` product packages in P0-1A/B/C/D. No future scaffolds, unused
  dependencies, ORM/AutoMigrate, MongoDB, NATS, vectors or observability stack.
- Business IDs use Go standard-library `uuid.NewV7`. easyhash must explicitly use
  Argon2id and an Argon2id upgrade policy; password hashes never enter transport.
- Public cookies carry opaque random tokens; only SHA-256 lookup hashes persist.
  Exact Origin protects unsafe auth/profile requests; authenticated unsafe requests
  also require a session-bound HMAC CSRF value. Never expose raw session tokens to JS.
- Auth owns the challenge-mail consumer interface and database transactions. Mail
  delivery happens after commit, with no raw challenge token in Redis/River/logs.
  P0-1B provides private local capture only, rejected in production.
- Auth owns static role/capability policy and the narrow throttle interface;
  `redisstore` implements HMAC subject/global counters with bounded TTLs.
  Public local auth fails open on Redis outage; Admin login/reauth fail closed.
- Admin uses only `kind=admin` password sessions and re-reads roles on each request.
  `adminctl` alone changes roles via the owner connection; runtime cannot write roles.
  Normal revocations stay within one session kind. Password reset/change are the
  explicit exception: all kinds revoked atomically, one Public replacement created.

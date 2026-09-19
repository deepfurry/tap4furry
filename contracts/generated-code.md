# Generated code contract

| Authoritative inputs | Committed outputs | Generator |
| --- | --- | --- |
| `contracts/openapi/public.yaml` | `server/internal/transport/public/generated`, `packages/api-client/src/generated/public` | oapi-codegen / Orval |
| `contracts/openapi/admin.yaml` | `server/internal/transport/admin/generated`, `packages/api-client/src/generated/admin` | oapi-codegen / Orval |
| `server/db/migrations`, `server/db/queries` | `server/internal/database/sqlc` | sqlc |

Never manually edit generated outputs. Go DTOs remain transport-only. TypeScript
consumers import `@tap4furry/api-client/public` or `@tap4furry/api-client/admin` facades.

Pin tools in the Go module and pnpm lockfile. `pnpm generate` invokes Go generation
and Orval; orchestration scripts do not implement generators. A second generation
must produce identical bytes and the same files. CI regenerates and rejects both
tracked differences and unexpected untracked generated files.

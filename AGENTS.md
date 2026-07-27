# Hexagonal Go Backend Guide

This repository is the reference implementation for a Go backend using Clean
Architecture (Hexagonal Architecture). Preserve the dependency rule: source
code may depend only inwards.

## Required reading and checks

1. Read this file before changing code.
2. Inspect `git status --short`; do not overwrite unrelated work.
3. Run `gofmt` on changed Go files and `go test ./...` before handoff.
4. Keep public HTTP contracts stable unless the task explicitly changes them.

## Target layout

```text
cmd/server/                         # composition root: config, wiring, process lifecycle
internal/
  domain/
    entity/                         # business entities, invariants, pure domain logic
    port/                           # repository/cache/event ports required by use cases
  application/
    service/                        # one service/use-case group per business capability
    port/                           # application-owned external capabilities (e.g. token, password)
  adapter/
    http/                           # driving adapter: handlers, middleware, routing, DTO mapping
    repository/mongo/               # driven persistence adapter
    cache/                          # driven cache adapter
    ws/                             # driven real-time/event adapter
    security/                       # driven security adapters (JWT, bcrypt, etc.)
  infrastructure/
    config/                         # environment/config loading
    db/                             # Mongo/driver connection setup
    cache/                          # Redis/client connection setup
    seed/                           # operational bootstrap data
```

`cmd/server/bootstrap.go` is the only composition root. It creates concrete
adapters, injects them into application services, and gives the driving HTTP
adapter its dependencies. It must contain wiring only, not business rules.

## Dependency rule

Allowed direction:

```text
HTTP / worker / CLI adapter -> application -> domain
Mongo / Redis / JWT / WebSocket adapter -> application port or domain port
infrastructure -> external libraries
cmd/server -> all layers, solely for wiring
```

Forbidden imports:

- `domain` must not import Gin, MongoDB, Redis, JWT, bcrypt, configuration, or adapters.
- `application` must not import `adapter` or `infrastructure`; inject an interface instead.
- HTTP handlers must not call repositories directly or implement business decisions.
- Repositories must not return HTTP DTOs or reference Gin.

## How to add a feature

1. Put business data/invariants in `internal/domain/entity`.
2. Define or extend a narrow port where the *consumer* needs it:
   persistence/event/cache interfaces normally belong in `domain/port`; external
   capabilities used by a use case (password hashing, token issuing, clock,
   email) belong in `application/port`.
3. Implement orchestration in an application service. Accept input/output
   structs; keep transport and database types at the edges.
4. Implement the port in an adapter, such as `adapter/repository/mongo` or
   `adapter/security`.
5. Map HTTP JSON and context data in `adapter/http/handler`, then register the
   route in `adapter/http/router`.
6. Wire the new concrete adapter in `cmd/server/bootstrap.go`.
7. Add focused unit tests for services using fake ports, and adapter integration
   tests only where a real driver is needed.

## Conventions

- Name interfaces by capability (`TokenIssuer`, `UserRepository`), not vague
  implementation names (`JWTServiceInterface`, `UserRepoInterface`).
- Keep ports small and use-case oriented. Split a growing repository interface.
- Use `context.Context` for I/O-bound port methods.
- Convert transport/database errors to application errors at the appropriate
  boundary; never expose driver-specific errors as API behavior.
- Do not create catch-all `utils`, `models`, `helpers`, or a second parallel
  architecture. Place code in the layer that owns its concern.
- Domain entities currently use Mongo ObjectIDs as the established persistence
  identifier. For a new greenfield module, prefer a domain ID value type so the
  domain has no database-driver dependency; migrate existing entities only as a
  deliberate compatibility change.

## Tests

- Application-service tests should use in-memory fakes that implement ports.
- Adapter tests can use mocked drivers or a disposable integration environment.
- Do not require MongoDB or Redis to run ordinary unit tests.

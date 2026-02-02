# Isolated database tests with samurai

This example demonstrates writing fully isolated PostgreSQL tests using [samurai](https://github.com/zerosixty/samurai) and [pgtestdb](https://github.com/peterldowns/pgtestdb). Each leaf test gets its own database — no shared state, full parallel execution.

## How it works

Samurai re-runs the builder from scratch for each leaf path. Database setup lives in a parent `Test` callback, so every leaf automatically gets a fresh, isolated database:

```
Path 1: with fresh database → Add → returns sequential IDs     (own DB)
Path 2: with fresh database → Get → returns the created todo   (own DB)
Path 3: with fresh database → Get → returns error for non-existent ID  (own DB)
Path 4: with fresh database → Complete → marks todo as done    (own DB)
Path 5: with fresh database → All → returns empty list         (own DB)
Path 6: with fresh database → All → returns all added todos    (own DB)
```

All 6 paths run in parallel. Each calls `newPool()` which provisions a fresh PostgreSQL database with migrations already applied.

[pgtestdb](https://github.com/peterldowns/pgtestdb) makes this fast by using PostgreSQL template databases: migrations run once, then each test gets a near-instant clone (~20ms). But samurai itself has no dependency on pgtestdb — the same pattern works with [testcontainers-go](https://github.com/testcontainers/testcontainers-go), custom scripts, or any other approach that gives each test its own database.

## The pattern

A helper provisions a fresh database and returns a connection pool:

```go
func newPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
    t.Helper()
    conf := pgtestdb.Custom(t, dbConf, migrator)
    pool, err := pgxpool.New(ctx, conf.URL())
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(pool.Close)
    return pool
}
```

`pgtestdb.Custom()` creates an isolated database with Atlas migrations applied and returns connection config. We use `Custom()` instead of `New()` because we need a native `pgxpool.Pool`, not `*sql.DB`.

The samurai test tree calls `newPool` in the parent setup — each leaf path re-executes this setup independently:

```go
func TestTodoRepo(t *testing.T) {
    samurai.Run(t, func(s *samurai.Scope) {
        var repo *todo.Repo

        s.Test("with fresh database", func(ctx context.Context, w samurai.W) {
            pool := newPool(ctx, w.Testing())
            repo = todo.NewRepo(pool)
        }, func(s *samurai.Scope) {

            s.Test("Add", func(ctx context.Context, w samurai.W) {
                id, err := repo.Add(ctx, "buy milk")
                assert.NoError(w.Testing(), err)
                assert.Positive(w.Testing(), id)
            }, func(s *samurai.Scope) {
                s.Test("returns sequential IDs", func(ctx context.Context, w samurai.W) {
                    id1, _ := repo.Add(ctx, "first")
                    id2, _ := repo.Add(ctx, "second")
                    assert.Equal(w.Testing(), id1+1, id2)
                })
            })

            s.Test("Get", func(ctx context.Context, w samurai.W) {
                _, err := repo.Add(ctx, "learn samurai")
                assert.NoError(w.Testing(), err)
            }, func(s *samurai.Scope) {
                s.Test("returns the created todo", func(ctx context.Context, w samurai.W) {
                    id, _ := repo.Add(ctx, "test get")
                    got, err := repo.Get(ctx, id)
                    assert.NoError(w.Testing(), err)
                    assert.Equal(w.Testing(), "test get", got.Title)
                    assert.False(w.Testing(), got.Done)
                })

                s.Test("returns error for non-existent ID", func(ctx context.Context, w samurai.W) {
                    _, err := repo.Get(ctx, 99999)
                    assert.Error(w.Testing(), err)
                })
            })

            // ... more test groups (Complete, All)
        })
    })
}
```

## Stack

| Component | Role |
|---|---|
| [samurai](https://github.com/zerosixty/samurai) | Test framework — path isolation, parallel execution |
| [pgtestdb](https://github.com/peterldowns/pgtestdb) | Database provisioning — template-based fast cloning |
| [Atlas](https://atlasgo.io) | Schema migrations |
| [pgx](https://github.com/jackc/pgx) | PostgreSQL driver (native `pgxpool.Pool`) |
| [testify](https://github.com/stretchr/testify) | Assertions |

## Prerequisites

- Go 1.24+
- Docker
- [Atlas CLI](https://atlasgo.io) (`curl -sSf https://atlasgo.sh | sh`)

## Run

```bash
docker compose up -d          # start postgres on port 5444
go test -v -count=1 ./...     # run tests
docker compose down            # cleanup
```

Output:

```
=== RUN   TestTodoRepo
=== RUN   TestTodoRepo/with_fresh_database
=== RUN   TestTodoRepo/with_fresh_database/Add
=== RUN   TestTodoRepo/with_fresh_database/Add/returns_sequential_IDs
=== RUN   TestTodoRepo/with_fresh_database/Get
=== RUN   TestTodoRepo/with_fresh_database/Get/returns_the_created_todo
=== RUN   TestTodoRepo/with_fresh_database/Get/returns_error_for_non-existent_ID
=== RUN   TestTodoRepo/with_fresh_database/Complete
=== RUN   TestTodoRepo/with_fresh_database/Complete/marks_todo_as_done
=== RUN   TestTodoRepo/with_fresh_database/All
=== RUN   TestTodoRepo/with_fresh_database/All/returns_empty_list_on_fresh_DB
=== RUN   TestTodoRepo/with_fresh_database/All/returns_all_added_todos
--- PASS: TestTodoRepo (0.84s)
```

## Files

| File | Purpose |
|---|---|
| `todo.go` | Simple `Repo` with Add/Get/Complete/All via `pgxpool` |
| `todo_test.go` | Samurai tests demonstrating isolated DB testing |
| `migrations/` | Atlas migration creating the `todos` table |
| `docker-compose.yml` | PostgreSQL 17 on port 5444 (tmpfs, fsync=off) |

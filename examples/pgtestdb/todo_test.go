package todo_test

import (
	"context"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // register "pgx" driver for pgtestdb
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/atlasmigrator"
	"github.com/stretchr/testify/assert"

	"github.com/zerosixty/samurai"
	todo "github.com/zerosixty/samurai/examples/pgtestdb"
)

// dbConf is the connection config for the local docker-compose postgres.
var dbConf = pgtestdb.Config{
	DriverName: "pgx",
	Host:       "localhost",
	Port:       "5444",
	User:       "postgres",
	Password:   "password",
	Options:    "sslmode=disable",
}

// migrator points to the atlas migrations directory.
var migrator = atlasmigrator.NewDirMigrator("migrations")

// newPool creates a fresh isolated database and returns a pgxpool.Pool connected to it.
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

// TestTodoRepo demonstrates isolated database tests using samurai + pgtestdb.
// Each leaf test gets its own postgres database — no shared state, full isolation.
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

			s.Test("Complete", func(ctx context.Context, w samurai.W) {
				_, err := repo.Add(ctx, "finish docs")
				assert.NoError(w.Testing(), err)
			}, func(s *samurai.Scope) {

				s.Test("marks todo as done", func(ctx context.Context, w samurai.W) {
					id, _ := repo.Add(ctx, "mark done")
					err := repo.Complete(ctx, id)
					assert.NoError(w.Testing(), err)

					got, _ := repo.Get(ctx, id)
					assert.True(w.Testing(), got.Done)
				})
			})

			s.Test("All", func(_ context.Context, _ samurai.W) {
				// no-op parent setup
			}, func(s *samurai.Scope) {

				s.Test("returns empty list on fresh DB", func(ctx context.Context, w samurai.W) {
					todos, err := repo.All(ctx)
					assert.NoError(w.Testing(), err)
					assert.Empty(w.Testing(), todos)
				})

				s.Test("returns all added todos", func(ctx context.Context, w samurai.W) {
					_, _ = repo.Add(ctx, "one")
					_, _ = repo.Add(ctx, "two")

					todos, err := repo.All(ctx)
					assert.NoError(w.Testing(), err)
					assert.Len(w.Testing(), todos, 2)
					assert.Equal(w.Testing(), "one", todos[0].Title)
					assert.Equal(w.Testing(), "two", todos[1].Title)
				})
			})
		})
	})
}

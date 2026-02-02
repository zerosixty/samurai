// Package todo demonstrates samurai testing with pgtestdb.
package todo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Todo represents a single todo item.
type Todo struct {
	ID    int64
	Title string
	Done  bool
}

// Repo provides CRUD operations for todos.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo creates a new Repo backed by the given connection pool.
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Add creates a new todo and returns its ID.
func (r *Repo) Add(ctx context.Context, title string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO todos (title) VALUES ($1) RETURNING id`, title,
	).Scan(&id)
	return id, err
}

// Get returns a single todo by ID.
func (r *Repo) Get(ctx context.Context, id int64) (*Todo, error) {
	t := &Todo{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, title, done FROM todos WHERE id = $1`, id,
	).Scan(&t.ID, &t.Title, &t.Done)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// Complete marks a todo as done.
func (r *Repo) Complete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE todos SET done = true WHERE id = $1`, id,
	)
	return err
}

// All returns all todos.
func (r *Repo) All(ctx context.Context) ([]Todo, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, title, done FROM todos ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []Todo
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.Title, &t.Done); err != nil {
			return nil, err
		}
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

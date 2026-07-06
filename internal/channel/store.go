// Package channel stores and retrieves chat channels
package channel

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/loqui-chat/loqui-backend/internal/snowflake"
)

type Channel struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

var ErrNotFound = errors.New("channel: not found")

type Store struct {
	pool *pgxpool.Pool
	gen  *snowflake.Generator
}

func NewStore(pool *pgxpool.Pool, gen *snowflake.Generator) *Store {
	return &Store{pool: pool, gen: gen}
}

// Create inserts a new channel
func (s *Store) Create(ctx context.Context, name string) (*Channel, error) {
	id, err := s.gen.Next()
	if err != nil {
		return nil, err
	}
	c := &Channel{ID: id, Name: name}
	err = s.pool.QueryRow(
		ctx, `
		insert into channels (id, name)
		value ($1, $2)
		returning created_at`,
		c.ID, c.Name,
	).Scan(&c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// GetByID looks up a channel by id
func (s *Store) GetByID(ctx context.Context, id int64) (*Channel, error) {
	c := &Channel{}
	err := s.pool.QueryRow(
		ctx, `
		select id, name, created_at from channels where id = $1`, id,
	).Scan(&c.ID, &c.Name, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// List returns all channels ordered by id (creation order)
func (s *Store) List(ctx context.Context) ([]Channel, error) {
	rows, err := s.pool.Query(ctx, `
		select id, name, created_at from channels order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

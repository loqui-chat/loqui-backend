// Package message stores and paginates channel messages
package message

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/loqui-chat/loqui-backend/internal/snowflake"
)

type Author struct {
	ID            int64
	Username      string
	Discriminator string
}

type Message struct {
	ID        int64
	ChannelID int64
	Content   string
	EditedAt  *time.Time
	CreatedAt time.Time
	Author    Author
}

type Store struct {
	pool *pgxpool.Pool
	gen  *snowflake.Generator
}

func NewStore(pool *pgxpool.Pool, gen *snowflake.Generator) *Store {
	return &Store{pool: pool, gen: gen}
}

// Create inserts a message and returns it with its author embedded
func (s *Store) Create(ctx context.Context, channelID, authorID int64, content string) (*Message, error) {
	id, err := s.gen.Next()
	if err != nil {
		return nil, err
	}
	m := &Message{}
	err = s.pool.QueryRow(
		ctx, `
		with inserted as (
				insert into messages (id, channel_id, author_id, content)
				values ($1, $2, $3, $4)
				returning id, channel_id, author_id, content, edited_at, created_at
		)
		select i.id, i.channel_id, i.content, i.edited_at, i.created_at,
					u.id, u.username, u.discriminator
		from inserted i
		join users u on u.id = i.author_id`,
		id, channelID, authorID, content,
	).Scan(
		&m.ID, &m.ChannelID, &m.Content, &m.EditedAt, &m.CreatedAt,
		&m.Author.ID, &m.Author.Username, &m.Author.Discriminator,
	)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// List returns newest-first messages (cursors are optional, limit 1..100)
func (s *Store) List(ctx context.Context, channelID, before, after int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// forward pagination reads ascending. reverse so newest stays first
	asc := after > 0 && before == 0
	order := "desc"
	if asc {
		order = "asc"
	}

	rows, err := s.pool.Query(
		ctx, `
		select m.id, m.channel_id, m.content, m.edited_at, m.created_at,
					u.id, u.username, u.discriminator
		from messages m
		join users u on u.id = m.author_id
		where m.channel_id = $1 and m.deleted_at is null
			and ($2 = 0 or m.id < $2)
			and ($3 = 0 or m.id < $3)
		order by m.id `+order+`
		limit $4`,
		channelID, before, after, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(
			&m.ID, &m.ChannelID, &m.Content, &m.EditedAt, &m.CreatedAt,
			&m.Author.ID, &m.Author.Username, &m.Author.Discriminator,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if asc {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, nil
}

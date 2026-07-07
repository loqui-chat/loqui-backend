-- +migrate up

create table messages (
    id         bigint      primary key,
    channel_id bigint      not null references channels(id),
    author_id  bigint      not null references users(id),
    content    text        not null,
    edited_at  timestamptz,
    deleted_at timestamptz,
    created_at timestamptz not null default now(),

    -- length backstop
    -- control-char rules are enforced in app layer
    constraint messages_content_len check (char_length(content) between 1 and 2000)
);

-- pagination reads by channel (newest first)
-- soft-deleted rows are skipped
create index messages_channel_id_idx
    on messages (channel_id, id)
    where deleted_at is null;

-- +migrate down

drop table messages;

-- +migrate up

create table refresh_tokens (
    id           bigint      primary key,   -- snowflake, also tokens public id
    user_id      bigint      not null references users(id) on delete cascade,
    family_id    bigint      not null,      -- session: shared by every token in a rotation chain
    token_hash   text        not null,      -- sha256 of opaque secret half
    user_agent   text,                      -- nullable (active devices list)
    created_at   timestamptz not null default now(),
    last_used_at timestamptz not null default now(),
    expires_at   timestamptz not null,
    rotated_at   timestamptz,               -- set when this token is exchanged for a successor
    revoked_at   timestamptz                -- set when this tokens family is revoked
);

-- reuse detection and revocation both act on a whole family
create index refresh_tokens_family_idx on refresh_tokens (family_id);

-- listing/bulk revoking one users sessions later
create index refresh_tokens_user_idx on refresh_tokens (user_id);

-- +migrate down

drop table refresh_tokens;

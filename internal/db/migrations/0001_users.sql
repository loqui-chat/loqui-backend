-- +migrate up

create table users (
    id            bigint primary key,
    username      text        not null,
    discriminator text        not null,
    email         text,
    password_hash text        not null,
    created_at    timestamptz not null default now(),

    -- letters digits underscore dot hyphen
    constraint users_username_format
        check (username ~ '^[A-Za-z0-9_.-]{2,32}$'),

    -- exactly 4 base62 chars
    constraint users_discriminator_format
        check (discriminator ~ '^[A-Za-z0-9]{4}$')
);

-- identity = (username, discriminator): username case insensitive,
-- discriminator case sensitive. so user#0000 and example#0000 coexist
create unique index users_identity_key
    on users (lower(username), discriminator);

-- email optional, unique case insensitively when set
create unique index users_email_key
    on users (lower(email))
    where email is not null;

-- +migrate down

drop table users;

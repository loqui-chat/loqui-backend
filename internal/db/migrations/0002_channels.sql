-- +migrate up

create table channels (
    id         bigint      primary key,
    name       text        not null,
    created_at timestamptz not null default now(),

    -- free-form, 1-100 chars, not control characters
    constraint channels_name_valid
        check (char_length(name) between 1 and 100 and name !~ '[[:cntrl:]]')
);

-- +migrate down

drop table channels;

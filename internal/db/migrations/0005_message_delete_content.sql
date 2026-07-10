-- +migrate up

-- deleted messages have their content cleared, so length floor only
-- applies while a message is live
alter table messages drop constraint messages_content_len;
alter table messages add constraint messages_content_len
    check (deleted_at is not null or char_length(content) between 1 and 2000);

-- +migrate down

-- note: fails if any delered rows already hold empty content
alter table messages drop constraint messages_content_len;
alter table messages add constraint messages_content_len
    check (char_length(content) between 1 and 2000);
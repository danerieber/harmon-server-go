create table if not exists persona (
    id bigserial primary key,
    created_at timestamp not null default now(),

    username text,
    username_color text,
    icon text
);

create table if not exists account (
    id bigserial primary key,
    created_at timestamp not null default now(),

    username text not null,
    password_hash text not null
);

create table if not exists place (
    id bigserial primary key,
    created_at timestamp not null default now(),

    owner_account_id bigint references account(id) not null,

    is_public boolean not null default false,
    name text not null,
    allow_members_to_invite boolean not null default false
);

create table if not exists place_invite (
    id uuid primary key default gen_random_uuid(),
    created_at timestamp not null default now(),

    place_id bigint references place(id) not null,

    created_by_account_id bigint references account(id) not null,
    expires_at timestamp not null default now() + interval '7 days'
);

create table if not exists place_account (
    id bigserial primary key,
    created_at timestamp not null default now(),

    place_id bigint references place(id) not null,
    account_id bigint references account(id) not null,

    persona_id bigint references persona(id)
);

create table if not exists place_account_ban (
    id bigserial primary key,
    created_at timestamp not null default now(),

    place_id bigint references place(id) not null,
    account_id bigint references account(id) not null,

    created_by_account_id bigint references account(id) not null,
);

create table if not exists chat_message (
    id bigserial primary key,

    created_at timestamp not null default now(),

    place_id bigint references place(id) not null,
    account_id bigint references account(id) not null,

    content text not null,
    reply_to_message_id bigint references chat_message(id),
    is_deleted boolean not null default false,
    edits jsonb,
    reactions jsonb,
    attachments jsonb
);

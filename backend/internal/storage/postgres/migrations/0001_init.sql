CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username      text        NOT NULL UNIQUE
                  CHECK (username = lower(username) AND char_length(username) BETWEEN 3 AND 32),
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz
);

-- One row per pair of users: user_a < user_b makes (alice, bob) and (bob, alice) the same dialog.
CREATE TABLE dialogs (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_a     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_b     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (user_a < user_b),
    UNIQUE (user_a, user_b)
);
CREATE INDEX dialogs_user_b_idx ON dialogs (user_b); -- user_a is covered by the unique index

CREATE TABLE messages (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dialog_id  uuid        NOT NULL REFERENCES dialogs (id) ON DELETE CASCADE,
    sender_id  uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    client_id  uuid        NOT NULL,
    body       text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4000),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (sender_id, client_id) -- idempotent sends
);
CREATE INDEX messages_dialog_id_id_idx ON messages (dialog_id, id DESC);

-- Read receipts as a "read up to" pointer per participant instead of a flag
-- per message: one row per user per dialog, one UPDATE per read.
CREATE TABLE dialog_reads (
    dialog_id            uuid        NOT NULL REFERENCES dialogs (id) ON DELETE CASCADE,
    user_id              uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    last_read_message_id bigint      NOT NULL,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (dialog_id, user_id)
);

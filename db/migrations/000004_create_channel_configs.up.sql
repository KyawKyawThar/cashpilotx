CREATE TYPE channel_type AS ENUM ('whatsapp', 'line', 'messenger', 'wechat', 'telegram', 'viper');

CREATE TABLE channel_configs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel          channel_type NOT NULL,

    config_encrypted BYTEA NOT NULL,
    config_nonce     BYTEA NOT NULL,
    config_tag       BYTEA NOT NULL,
    key_id           TEXT NOT NULL,

    config_version   INT NOT NULL DEFAULT 1,

    is_active        BOOL NOT NULL DEFAULT TRUE,

    status           TEXT NOT NULL DEFAULT 'disconnected'
                     CHECK (status IN ('connected','disconnected','error','revoked')),

    last_connected_at TIMESTAMPTZ,
    last_error        TEXT,

    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at       TIMESTAMPTZ
);
CREATE UNIQUE INDEX uniq_channel_active
ON channel_configs(user_id, channel)
WHERE deleted_at IS NULL;

CREATE TRIGGER trg_channel_configs_updated_at
BEFORE UPDATE ON channel_configs
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
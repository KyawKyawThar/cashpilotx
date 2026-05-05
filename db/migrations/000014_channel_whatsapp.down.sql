-- PostgreSQL does not support removing individual ENUM values.
-- Recreate message_channel without 'whatsapp'.
ALTER TYPE message_channel RENAME TO message_channel_old;

CREATE TYPE message_channel AS ENUM ('line', 'messenger', 'wechat', 'telegram', 'external');

ALTER TABLE messages
    ALTER COLUMN channel TYPE message_channel
    USING (CASE channel::text
        WHEN 'whatsapp' THEN 'external'::message_channel
        ELSE channel::text::message_channel
    END);

ALTER TABLE reminders
    ALTER COLUMN channel TYPE message_channel
    USING (CASE channel::text
        WHEN 'whatsapp' THEN 'external'::message_channel
        ELSE channel::text::message_channel
    END);

DROP TYPE message_channel_old;

-- Recreate contact_platform without 'WHATSAPP'.
ALTER TYPE contact_platform RENAME TO contact_platform_old;

CREATE TYPE contact_platform AS ENUM ('LINE', 'WECHAT', 'VIBER', 'MESSENGER', 'TELEGRAM');

ALTER TABLE customer_contacts
    ALTER COLUMN platform TYPE contact_platform
    USING (CASE platform::text
        WHEN 'WHATSAPP' THEN 'MESSENGER'::contact_platform
        ELSE platform::text::contact_platform
    END);

DROP TYPE contact_platform_old;

-- Rename old ENUM so we can replace it atomically.
ALTER TYPE message_tone RENAME TO message_tone_old;

-- New labels match the v4 UI: polite → firm → urgent → final.
CREATE TYPE message_tone AS ENUM ('polite', 'firm', 'urgent', 'final');

-- Migrate messages.tone (default was 'friendly').
ALTER TABLE messages
    ALTER COLUMN tone DROP DEFAULT,
    ALTER COLUMN tone TYPE message_tone
        USING (CASE tone::text
            WHEN 'friendly' THEN 'polite'::message_tone
            WHEN 'neutral'  THEN 'polite'::message_tone
            WHEN 'firm'     THEN 'firm'::message_tone
            WHEN 'strict'   THEN 'urgent'::message_tone
            ELSE 'polite'::message_tone
        END),
    ALTER COLUMN tone SET DEFAULT 'polite';

-- Migrate reminders.tone (default was 'friendly').
ALTER TABLE reminders
    ALTER COLUMN tone DROP DEFAULT,
    ALTER COLUMN tone TYPE message_tone
        USING (CASE tone::text
            WHEN 'friendly' THEN 'polite'::message_tone
            WHEN 'neutral'  THEN 'polite'::message_tone
            WHEN 'firm'     THEN 'firm'::message_tone
            WHEN 'strict'   THEN 'urgent'::message_tone
            ELSE 'polite'::message_tone
        END),
    ALTER COLUMN tone SET DEFAULT 'polite';

DROP TYPE message_tone_old;

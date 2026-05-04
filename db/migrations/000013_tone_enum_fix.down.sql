ALTER TYPE message_tone RENAME TO message_tone_new;

CREATE TYPE message_tone AS ENUM ('friendly', 'neutral', 'firm', 'strict');

ALTER TABLE messages
    ALTER COLUMN tone DROP DEFAULT,
    ALTER COLUMN tone TYPE message_tone
        USING (CASE tone::text
            WHEN 'polite'  THEN 'friendly'::message_tone
            WHEN 'firm'    THEN 'firm'::message_tone
            WHEN 'urgent'  THEN 'strict'::message_tone
            WHEN 'final'   THEN 'strict'::message_tone
            ELSE 'friendly'::message_tone
        END),
    ALTER COLUMN tone SET DEFAULT 'friendly';

ALTER TABLE reminders
    ALTER COLUMN tone DROP DEFAULT,
    ALTER COLUMN tone TYPE message_tone
        USING (CASE tone::text
            WHEN 'polite'  THEN 'friendly'::message_tone
            WHEN 'firm'    THEN 'firm'::message_tone
            WHEN 'urgent'  THEN 'strict'::message_tone
            WHEN 'final'   THEN 'strict'::message_tone
            ELSE 'friendly'::message_tone
        END),
    ALTER COLUMN tone SET DEFAULT 'friendly';

DROP TYPE message_tone_new;

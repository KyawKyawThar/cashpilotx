-- Add 'whatsapp' to message_channel so messages and reminders can target
-- the WhatsApp channel shown in the v4 Send modal.
ALTER TYPE message_channel ADD VALUE IF NOT EXISTS 'whatsapp' AFTER 'telegram';

-- Add 'WHATSAPP' to contact_platform so customer_contacts can store
-- WhatsApp platform IDs alongside LINE, MESSENGER, TELEGRAM.
ALTER TYPE contact_platform ADD VALUE IF NOT EXISTS 'WHATSAPP' AFTER 'TELEGRAM';

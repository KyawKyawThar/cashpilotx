-- Reset locale to 'th' for all users (reverting the backfill).
UPDATE users SET locale = 'th', updated_at = now();

-- Make locale self-healing: a BEFORE INSERT/UPDATE trigger computes it
-- directly from home_currency so application code can never produce NULL locale.
-- Also backfills any existing NULL rows.

UPDATE users
SET locale = CASE home_currency
               WHEN 'MMK' THEN 'my'
               WHEN 'CNY' THEN 'zh'
               ELSE 'th'
             END,
    updated_at = now()
WHERE locale IS NULL;

CREATE OR REPLACE FUNCTION sync_locale_from_currency()
RETURNS TRIGGER AS $$
BEGIN
    NEW.locale := CASE NEW.home_currency
                    WHEN 'MMK' THEN 'my'
                    WHEN 'CNY' THEN 'zh'
                    ELSE 'th'
                  END;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_sync_locale
BEFORE INSERT OR UPDATE OF home_currency ON users
FOR EACH ROW EXECUTE FUNCTION sync_locale_from_currency();

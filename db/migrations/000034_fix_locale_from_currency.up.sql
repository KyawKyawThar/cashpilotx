-- Backfill locale for all existing users where it was left at the default 'th'
-- regardless of their actual home_currency.
UPDATE users
SET locale = CASE home_currency
               WHEN 'MMK' THEN 'my'
               WHEN 'CNY' THEN 'zh'
               ELSE 'th'
             END,
    updated_at = now()
WHERE locale IS NULL
   OR locale != CASE home_currency
                  WHEN 'MMK' THEN 'my'
                  WHEN 'CNY' THEN 'zh'
                  ELSE 'th'
                END;

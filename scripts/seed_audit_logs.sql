-- seed_audit_logs.sql
-- Inserts sample audit log entries linked to the first registered user.
-- Run with: psql $DATABASE_URL -f scripts/seed_audit_logs.sql
--
-- Safe to re-run: uses INSERT only (the table has delete-prevention triggers).

DO $$
DECLARE
  v_user  UUID;
  v_cust  UUID;
  v_inv   UUID;
  v_pay   UUID;
BEGIN
  -- Resolve first user
  SELECT id INTO v_user FROM users ORDER BY created_at LIMIT 1;
  IF v_user IS NULL THEN
    RAISE NOTICE 'No users found — run seed.sh first, then re-run this script.';
    RETURN;
  END IF;

  -- Resolve optional FK references (nullable — fine if missing)
  SELECT id INTO v_cust FROM customers WHERE user_id = v_user ORDER BY created_at LIMIT 1;
  SELECT id INTO v_inv  FROM invoices   WHERE user_id = v_user ORDER BY created_at LIMIT 1;
  SELECT id INTO v_pay  FROM payments   WHERE user_id = v_user ORDER BY created_at LIMIT 1;

  RAISE NOTICE 'Seeding audit logs for user %', v_user;

  INSERT INTO operator_audit_log
    (user_id, performed_by, action_type, entity_type, action_label, detail,
     customer_id, invoice_id, payment_id, metadata, created_at)
  VALUES
    -- payment_recorded
    (v_user, v_user,
     'payment_recorded', 'payment',
     'Payment Recorded',
     'Recorded a full payment of ฿15,000 via bank transfer for invoice #INV-001.',
     v_cust, v_inv, v_pay,
     '{"method":"transfer","amount":15000,"currency":"THB","is_full_payment":true}'::jsonb,
     now() - interval '5 minutes'),

    -- message_marked_sent
    (v_user, v_user,
     'message_marked_sent', 'message',
     'Message Marked Sent',
     'WhatsApp reminder message marked as sent manually to Ma Hla Hla.',
     v_cust, v_inv, NULL,
     '{"channel":"whatsapp","tone":"friendly","attempt":2}'::jsonb,
     now() - interval '38 minutes'),

    -- collection_state_changed
    (v_user, v_user,
     'collection_state_changed', 'invoice',
     'Collection State → No Response',
     'Invoice moved from "reminded" to "no_response" after 3 unanswered follow-ups.',
     v_cust, v_inv, NULL,
     '{"from":"reminded","to":"no_response","note":"No reply after 72h"}'::jsonb,
     now() - interval '2 hours'),

    -- message_copied
    (v_user, v_user,
     'message_copied', 'message',
     'Message Copied',
     'Copied draft collection message text for Ko Zaw Lin — to paste into LINE.',
     v_cust, v_inv, NULL,
     '{"channel":"line","tone":"firm","attempt":1}'::jsonb,
     now() - interval '3 hours'),

    -- reminder_scheduled
    (v_user, v_user,
     'reminder_scheduled', 'reminder',
     'Reminder Scheduled',
     'Scheduled a follow-up reminder in 3 days for invoice INV-004 (Nang Phyu Phyu).',
     v_cust, v_inv, NULL,
     '{"channel":"sms","scheduled_for":"2026-05-17T09:00:00Z","tone":"polite"}'::jsonb,
     now() - interval '5 hours'),

    -- invoice_created
    (v_user, v_user,
     'invoice_created', 'invoice',
     'Invoice Created',
     'New invoice INV-007 created for Sai Kham for retail supply — April batch. Amount: K320,000.',
     v_cust, v_inv, NULL,
     '{"amount":320000,"currency":"MMK","due_days":30}'::jsonb,
     now() - interval '1 day'),

    -- payment_recorded (partial)
    (v_user, v_user,
     'payment_recorded', 'payment',
     'Partial Payment Recorded',
     'Recorded a partial payment of K180,000 cash from Sai Kham against INV-007.',
     v_cust, v_inv, v_pay,
     '{"method":"cash","amount":180000,"currency":"MMK","is_full_payment":false}'::jsonb,
     now() - interval '1 day' - interval '2 hours'),

    -- collection_state_changed (negotiating)
    (v_user, v_user,
     'collection_state_changed', 'invoice',
     'Collection State → Negotiating',
     'INV-007 moved to negotiating after customer replied with a proposed payment schedule.',
     v_cust, v_inv, NULL,
     '{"from":"reminded","to":"negotiating","note":"Customer proposes paying in 2 instalments"}'::jsonb,
     now() - interval '2 days'),

    -- message_marked_sent
    (v_user, v_user,
     'message_marked_sent', 'message',
     'Message Marked Sent',
     'SMS reminder for INV-003 marked as sent. Tone: urgent.',
     v_cust, v_inv, NULL,
     '{"channel":"sms","tone":"urgent","attempt":3}'::jsonb,
     now() - interval '2 days' - interval '4 hours'),

    -- invoice_created (second)
    (v_user, v_user,
     'invoice_created', 'invoice',
     'Invoice Created',
     'New invoice INV-008 created for Ma Hla Hla — service fee May. Amount: ฿8,500.',
     v_cust, v_inv, NULL,
     '{"amount":8500,"currency":"THB","due_days":14}'::jsonb,
     now() - interval '3 days'),

    -- reminder_scheduled (second)
    (v_user, v_user,
     'reminder_scheduled', 'reminder',
     'Reminder Scheduled',
     'Automated reminder scheduled for INV-008 due in 7 days. Channel: email.',
     v_cust, v_inv, NULL,
     '{"channel":"email","scheduled_for":"2026-05-21T08:30:00Z","tone":"polite"}'::jsonb,
     now() - interval '3 days' - interval '1 hour'),

    -- message_copied (second)
    (v_user, v_user,
     'message_copied', 'message',
     'Message Copied',
     'Copied overdue notice text for Ko Zaw Lin — INV-005 (45 days overdue).',
     v_cust, v_inv, NULL,
     '{"channel":"whatsapp","tone":"firm","attempt":4}'::jsonb,
     now() - interval '4 days'),

    -- action_reverted (to demonstrate the reverted state)
    (v_user, v_user,
     'action_reverted', 'system',
     'Entry Reverted',
     'A previously recorded payment was flagged as entered in error and marked reverted.',
     NULL, NULL, NULL,
     '{"reason":"entered_in_error"}'::jsonb,
     now() - interval '4 days' - interval '30 minutes'),

    -- collection_state_changed (paid)
    (v_user, v_user,
     'collection_state_changed', 'invoice',
     'Collection State → Paid',
     'INV-006 marked as paid in full after final bank transfer confirmed.',
     v_cust, v_inv, NULL,
     '{"from":"negotiating","to":"paid","note":"Transfer confirmed in bank statement"}'::jsonb,
     now() - interval '5 days'),

    -- payment_recorded (5 days ago)
    (v_user, v_user,
     'payment_recorded', 'payment',
     'Payment Recorded',
     'Full payment of ฿22,000 recorded via PromptPay for INV-006. Invoice now closed.',
     v_cust, v_inv, v_pay,
     '{"method":"promptpay","amount":22000,"currency":"THB","is_full_payment":true}'::jsonb,
     now() - interval '5 days' - interval '3 hours')
  ;

  -- Mark the "action_reverted" row as is_reverted to demonstrate the UI state
  UPDATE operator_audit_log
  SET
    is_reverted = TRUE,
    reverted_by = v_user,
    reverted_at = now() - interval '4 days'
  WHERE
    user_id     = v_user
    AND action_type = 'payment_recorded'
    AND detail LIKE '%entered in error%'
  ;
  -- (trigger allows revert field updates only)

  RAISE NOTICE 'Done — inserted 15 sample audit log entries for user %', v_user;
END
$$;

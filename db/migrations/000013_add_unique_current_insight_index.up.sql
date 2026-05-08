CREATE UNIQUE INDEX customer_insights_unique_current_idx ON customer_insights (user_id, customer_id, insight_type) WHERE is_current;

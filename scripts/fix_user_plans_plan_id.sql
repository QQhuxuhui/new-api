-- Fix user_plans.plan_id column to allow NULL values
-- This is required to support the OnDelete:SET NULL foreign key constraint

-- For PostgreSQL
ALTER TABLE user_plans ALTER COLUMN plan_id DROP NOT NULL;

-- For MySQL/MariaDB, use this instead:
-- ALTER TABLE user_plans MODIFY COLUMN plan_id INT NULL;

-- Verify the change
-- PostgreSQL: \d user_plans
-- MySQL: DESCRIBE user_plans;

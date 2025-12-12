-- Fix user_plans.plan_id column to allow NULL values (MySQL/MariaDB)
-- This is required to support the OnDelete:SET NULL foreign key constraint

ALTER TABLE user_plans MODIFY COLUMN plan_id INT NULL;

-- Verify the change
-- DESCRIBE user_plans;

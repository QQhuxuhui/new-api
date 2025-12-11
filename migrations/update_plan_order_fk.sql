-- Migration: Update PlanOrder foreign key constraint
-- Date: 2025-12-11
-- Purpose: Allow plan deletion by changing constraint to SET NULL

-- Step 1: Drop existing foreign key constraint
ALTER TABLE plan_orders DROP CONSTRAINT IF EXISTS fk_plan_orders_plan;

-- Step 2: Recreate with ON DELETE SET NULL
ALTER TABLE plan_orders
ADD CONSTRAINT fk_plan_orders_plan
FOREIGN KEY (plan_id)
REFERENCES plans(id)
ON DELETE SET NULL
ON UPDATE CASCADE;

-- Verification query (optional):
-- SELECT constraint_name, delete_rule, update_rule
-- FROM information_schema.referential_constraints
-- WHERE constraint_name = 'fk_plan_orders_plan';

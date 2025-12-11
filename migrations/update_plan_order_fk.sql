-- Migration: Update PlanOrder foreign key constraint to allow plan deletion
-- Date: 2025-12-11 (Final Version - Solution B)
-- Purpose: Allow deletion of plans with only completed orders

-- Business Logic:
-- - Plans can be deleted when all orders are completed (delivered/expired/cancelled)
-- - Plans CANNOT be deleted when there are pending or paid orders (enforced by application logic)
-- - When a plan is deleted, completed orders' plan_id is set to NULL
-- - Completed orders use snapshot fields (plan_name, plan_display_name) for display

-- Step 1: Drop existing foreign key constraint if it exists
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
-- Expected: delete_rule = 'SET NULL', update_rule = 'CASCADE'

-- Migration: Add plan name snapshots to PlanOrder and allow plan_id to be NULL
-- Date: 2025-12-11 (Final Version - Solution B)
-- Purpose: Store plan name snapshots and allow plan deletion with completed orders

-- Step 1: Modify plan_id column to allow NULL
-- This allows completed orders to remain when plans are deleted
ALTER TABLE plan_orders ALTER COLUMN plan_id DROP NOT NULL;

-- Step 2: Add snapshot columns
ALTER TABLE plan_orders
ADD COLUMN IF NOT EXISTS plan_name VARCHAR(255),
ADD COLUMN IF NOT EXISTS plan_display_name VARCHAR(255);

-- Step 3: Backfill existing orders with plan names from plans table
-- This preserves historical data for all existing orders
UPDATE plan_orders po
SET
    plan_name = p.name,
    plan_display_name = p.display_name
FROM plans p
WHERE po.plan_id = p.id
  AND (po.plan_name IS NULL OR po.plan_name = '')
  AND (po.plan_display_name IS NULL OR po.plan_display_name = '');

-- Step 4: Verification query (optional)
-- Check how many orders were updated
-- SELECT
--     COUNT(*) as total_orders,
--     COUNT(plan_name) as orders_with_name,
--     COUNT(plan_display_name) as orders_with_display_name,
--     COUNT(plan_id) as orders_with_plan_id
-- FROM plan_orders;

-- Step 5: Show orders that couldn't be backfilled (plan already deleted)
-- SELECT
--     id, order_no, plan_id, status, created_at
-- FROM plan_orders
-- WHERE (plan_name IS NULL OR plan_name = '')
--   AND (plan_display_name IS NULL OR plan_display_name = '')
-- ORDER BY created_at DESC;

-- Business Logic Notes:
-- - plan_id is now nullable to support plan deletion
-- - When a plan is deleted, completed orders' plan_id becomes NULL
-- - Snapshot fields (plan_name, plan_display_name) preserve display information
-- - Pending and paid orders cannot have their plans deleted (enforced by application)

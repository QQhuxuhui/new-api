-- Migration: Complete Solution B - Allow plan deletion with completed orders
-- Date: 2025-12-11
-- Purpose: Single-file migration combining all Solution B changes
--
-- This is a combined version of:
--   1. add_plan_order_snapshots.sql
--   2. update_plan_order_fk.sql
--
-- Use this if you prefer to execute everything in one transaction

BEGIN;

-- ============================================================================
-- PART 1: Modify plan_id column and add snapshot fields
-- ============================================================================

-- Step 1.1: Modify plan_id column to allow NULL
-- This allows completed orders to remain when plans are deleted
ALTER TABLE plan_orders ALTER COLUMN plan_id DROP NOT NULL;

-- Step 1.2: Add snapshot columns
ALTER TABLE plan_orders
ADD COLUMN IF NOT EXISTS plan_name VARCHAR(255),
ADD COLUMN IF NOT EXISTS plan_display_name VARCHAR(255);

-- Step 1.3: Backfill existing orders with plan names from plans table
-- This preserves historical data for all existing orders
UPDATE plan_orders po
SET
    plan_name = p.name,
    plan_display_name = p.display_name
FROM plans p
WHERE po.plan_id = p.id
  AND (po.plan_name IS NULL OR po.plan_name = '')
  AND (po.plan_display_name IS NULL OR po.plan_display_name = '');

-- ============================================================================
-- PART 2: Update foreign key constraint
-- ============================================================================

-- Step 2.1: Drop existing foreign key constraint if it exists
ALTER TABLE plan_orders DROP CONSTRAINT IF EXISTS fk_plan_orders_plan;

-- Step 2.2: Recreate with ON DELETE SET NULL
ALTER TABLE plan_orders
ADD CONSTRAINT fk_plan_orders_plan
FOREIGN KEY (plan_id)
REFERENCES plans(id)
ON DELETE SET NULL
ON UPDATE CASCADE;

COMMIT;

-- ============================================================================
-- Verification Queries (optional - uncomment to run)
-- ============================================================================

-- Check column nullable status
-- SELECT
--     column_name,
--     is_nullable,
--     data_type
-- FROM information_schema.columns
-- WHERE table_name = 'plan_orders'
--   AND column_name = 'plan_id';
-- Expected: is_nullable = 'YES'

-- Check foreign key constraint
-- SELECT constraint_name, delete_rule, update_rule
-- FROM information_schema.referential_constraints
-- WHERE constraint_name = 'fk_plan_orders_plan';
-- Expected: delete_rule = 'SET NULL', update_rule = 'CASCADE'

-- Check snapshot data backfill
-- SELECT
--     COUNT(*) as total_orders,
--     COUNT(plan_name) as orders_with_name,
--     COUNT(plan_display_name) as orders_with_display_name,
--     COUNT(plan_id) as orders_with_plan_id
-- FROM plan_orders;

-- ============================================================================
-- Business Logic Notes
-- ============================================================================
-- - plan_id is now nullable to support plan deletion
-- - When a plan is deleted, completed orders' plan_id becomes NULL
-- - Snapshot fields (plan_name, plan_display_name) preserve display information
-- - Pending and paid orders cannot have their plans deleted (enforced by application)

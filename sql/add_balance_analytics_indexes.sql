-- Migration: Add indexes for balance analytics queries
-- Date: 2025-11-27
-- Related to: enhance-analytics-with-usd-charts OpenSpec change

-- Add index on users.quota for balance ranking and distribution queries
-- This improves performance of ORDER BY quota DESC and WHERE quota BETWEEN queries
CREATE INDEX IF NOT EXISTS idx_users_quota ON users(quota);

-- Add index on users.status for active user filtering
-- All balance queries filter by status = 1 (enabled users only)
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);

-- Create composite index for optimal balance query performance
-- Covers: WHERE status = 1 ORDER BY quota DESC
CREATE INDEX IF NOT EXISTS idx_users_status_quota ON users(status, quota DESC);

-- Verify indexes were created
-- PostgreSQL:
-- SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'users' AND indexname LIKE 'idx_users_%';

-- MySQL:
-- SHOW INDEXES FROM users WHERE Key_name LIKE 'idx_users_%';

-- SQLite:
-- SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = 'users' AND name LIKE 'idx_users_%';

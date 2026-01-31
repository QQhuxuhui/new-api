-- 诊断脚本：检查用户套餐为什么不显示切换按钮
-- 请将 YOUR_USER_ID 替换为你的实际用户ID

-- 1. 查看你的所有套餐及关键字段
SELECT
    id,
    user_id,
    plan_id,
    plan_name,
    plan_display_name,
    is_current,
    allow_user_switch,  -- 这个字段决定是否显示切换按钮
    locked,
    queue_position,
    status,
    quota,
    expires_at
FROM user_plans
WHERE user_id = YOUR_USER_ID
ORDER BY is_current DESC, plan_priority DESC;

-- 2. 检查切换按钮不显示的具体原因
SELECT
    id,
    plan_display_name,
    CASE
        WHEN is_current = 1 THEN '❌ 是当前套餐（不显示切换按钮）'
        WHEN allow_user_switch = 0 THEN '❌ 不允许用户切换（allow_user_switch=0）'
        WHEN locked = 1 THEN '❌ 套餐已锁定'
        WHEN queue_position > 0 THEN '❌ 在队列中（不能手动切换）'
        WHEN status != 1 THEN '❌ 套餐状态异常（status != 1）'
        ELSE '✅ 应该显示切换按钮'
    END AS 切换按钮状态,
    is_current AS 是否当前,
    allow_user_switch AS 允许切换,
    locked AS 是否锁定,
    queue_position AS 队列位置,
    status AS 状态
FROM user_plans
WHERE user_id = YOUR_USER_ID
ORDER BY is_current DESC;

-- 3. 查看套餐模板的默认设置
SELECT
    p.id,
    p.name,
    p.display_name,
    p.default_allow_switch,  -- 套餐模板的默认切换权限
    COUNT(up.id) as 用户数量
FROM plans p
LEFT JOIN user_plans up ON up.plan_id = p.id
GROUP BY p.id
ORDER BY p.id;

-- 4. 如果需要修复：批量开启切换权限（谨慎执行！）
-- UPDATE user_plans
-- SET allow_user_switch = 1
-- WHERE user_id = YOUR_USER_ID
--   AND is_current = 0
--   AND locked = 0
--   AND queue_position = 0;

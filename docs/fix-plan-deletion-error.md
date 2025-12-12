# 修复套餐删除错误

## 问题描述

删除套餐时报错：
```
错误：ERROR: null value in column "plan_id" of relation "user_plans" violates not-null constraint (SQLSTATE 23502)
```

## 问题原因

`user_plans` 表的 `plan_id` 列在数据库中仍然设置为 NOT NULL，但代码已经将其定义为可空（`*int`），并设置了外键约束 `OnDelete:SET NULL`。当删除套餐时，系统尝试将关联的 user_plans 记录的 plan_id 设置为 NULL，但数据库约束不允许。

GORM 的 AutoMigrate 不会自动将已存在的 NOT NULL 列改为 NULLABLE，需要手动迁移。

## 解决方案

### 步骤 1: 备份数据库（重要！）

在执行任何数据库修改前，务必备份数据库：

```bash
# PostgreSQL
pg_dump -U username -d database_name > backup_$(date +%Y%m%d_%H%M%S).sql

# MySQL
mysqldump -u username -p database_name > backup_$(date +%Y%m%d_%H%M%S).sql
```

### 步骤 2: 执行数据库迁移

#### PostgreSQL

```bash
# 方法1：使用 psql 命令行
psql -U username -d database_name -f scripts/fix_user_plans_plan_id.sql

# 方法2：直接执行 SQL
psql -U username -d database_name -c "ALTER TABLE user_plans ALTER COLUMN plan_id DROP NOT NULL;"
```

#### MySQL/MariaDB

```bash
# 方法1：使用 mysql 命令行
mysql -u username -p database_name < scripts/fix_user_plans_plan_id_mysql.sql

# 方法2：直接执行 SQL
mysql -u username -p database_name -e "ALTER TABLE user_plans MODIFY COLUMN plan_id INT NULL;"
```

### 步骤 3: 验证修改

#### PostgreSQL
```bash
psql -U username -d database_name -c "\d user_plans" | grep plan_id
```

应该看到 `plan_id` 列没有 `not null` 约束。

#### MySQL
```bash
mysql -u username -p database_name -e "DESCRIBE user_plans;" | grep plan_id
```

应该看到 `plan_id` 列的 `Null` 列为 `YES`。

### 步骤 4: 测试删除套餐

修改完成后，尝试再次删除套餐，应该可以正常删除了。

## 注意事项

1. **删除保护**：即使修复了这个问题，系统仍然有删除保护机制：
   - 有活跃用户实例且未完全快照化的套餐无法删除
   - 有未完成订单（pending/paid）的套餐无法删除

2. **数据一致性**：当套餐被删除后：
   - 已完成快照的 UserPlan 记录会保留，`plan_id` 设置为 NULL
   - UserPlan 使用快照字段（`plan_name`, `plan_display_name` 等）独立运行
   - 已完成的订单也会保留，`plan_id` 设置为 NULL，使用订单快照字段显示

3. **快照迁移**：如果有用户实例显示 "未完全快照化" 的错误，需要先运行快照迁移：
   ```bash
   # 在应用启动时会自动执行，或手动触发
   # 查看 model/main.go 中的 migrateUserPlanSnapshots() 函数
   ```

## 技术细节

相关代码位置：
- `model/user_plan.go:16` - PlanId 定义为可空指针
- `model/user_plan.go:81` - 外键约束定义 `OnDelete:SET NULL`
- `model/plan.go:207-255` - 删除套餐的逻辑和保护机制

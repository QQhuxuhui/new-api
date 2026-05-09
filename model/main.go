package model

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var commonGroupCol string
var commonKeyCol string
var commonTrueVal string
var commonFalseVal string

var logKeyCol string
var logGroupCol string

func initCol() {
	// init common column names
	if common.UsingPostgreSQL {
		commonGroupCol = `"group"`
		commonKeyCol = `"key"`
		commonTrueVal = "true"
		commonFalseVal = "false"
	} else {
		commonGroupCol = "`group`"
		commonKeyCol = "`key`"
		commonTrueVal = "1"
		commonFalseVal = "0"
	}
	if os.Getenv("LOG_SQL_DSN") != "" {
		switch common.LogSqlType {
		case common.DatabaseTypePostgreSQL:
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		default:
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	} else {
		// LOG_SQL_DSN 为空时，日志数据库与主数据库相同
		if common.UsingPostgreSQL {
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		} else {
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	}
	// log sql type and database type
	//common.SysLog("Using Log SQL Type: " + common.LogSqlType)
}

var DB *gorm.DB

var LOG_DB *gorm.DB

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		DB.Create(&rootUser)
	}
	return nil
}

func CheckSetup() {
	setup := GetSetup()
	if setup == nil {
		// No setup record exists, check if we have a root user
		if RootUserExists() {
			common.SysLog("system is not initialized, but root user exists")
			// Create setup record
			newSetup := Setup{
				Version:       common.Version,
				InitializedAt: time.Now().Unix(),
			}
			err := DB.Create(&newSetup).Error
			if err != nil {
				common.SysLog("failed to create setup record: " + err.Error())
			}
			constant.Setup = true
		} else {
			common.SysLog("system is not initialized and no root user exists")
			constant.Setup = false
		}
	} else {
		// Setup record exists, system is initialized
		common.SysLog("system is already initialized at: " + time.Unix(setup.InitializedAt, 0).String())
		constant.Setup = true
	}
}

func chooseDB(envName string, isLog bool) (*gorm.DB, error) {
	defer func() {
		initCol()
	}()
	dsn := os.Getenv(envName)
	if dsn != "" {
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			// Use PostgreSQL
			common.SysLog("using PostgreSQL as database")
			if !isLog {
				common.UsingPostgreSQL = true
			} else {
				common.LogSqlType = common.DatabaseTypePostgreSQL
			}
			return gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // disables implicit prepared statement usage
			}), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
		}
		if strings.HasPrefix(dsn, "local") {
			common.SysLog("SQL_DSN not set, using SQLite as database")
			if !isLog {
				common.UsingSQLite = true
			} else {
				common.LogSqlType = common.DatabaseTypeSQLite
			}
			return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
		}
		// Use MySQL
		common.SysLog("using MySQL as database")
		// check parseTime
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		if !isLog {
			common.UsingMySQL = true
		} else {
			common.LogSqlType = common.DatabaseTypeMySQL
		}
		return gorm.Open(mysql.Open(dsn), &gorm.Config{
			PrepareStmt: true, // precompile SQL
		})
	}
	// Use SQLite
	common.SysLog("SQL_DSN not set, using SQLite as database")
	common.UsingSQLite = true
	return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
		PrepareStmt: true, // precompile SQL
	})
}

func InitDB() (err error) {
	db, err := chooseDB("SQL_DSN", false)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		DB = db
		// MySQL charset/collation startup check: ensure Chinese-capable charset
		if common.UsingMySQL {
			if err := checkMySQLChineseSupport(DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		if common.UsingMySQL {
			//_, _ = sqlDB.Exec("ALTER TABLE channels MODIFY model_mapping TEXT;") // TODO: delete this line when most users have upgraded
		}
		common.SysLog("database migration started")
		err = migrateDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func InitLogDB() (err error) {
	if os.Getenv("LOG_SQL_DSN") == "" {
		LOG_DB = DB
		// Sync LogSqlType with main database type when using shared connection
		if common.UsingPostgreSQL {
			common.LogSqlType = common.DatabaseTypePostgreSQL
		} else if common.UsingMySQL {
			common.LogSqlType = common.DatabaseTypeMySQL
		} else if common.UsingSQLite {
			common.LogSqlType = common.DatabaseTypeSQLite
		}
		return
	}
	db, err := chooseDB("LOG_SQL_DSN", true)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		LOG_DB = db
		// If log DB is MySQL, also ensure Chinese-capable charset
		if common.LogSqlType == common.DatabaseTypeMySQL {
			if err := checkMySQLChineseSupport(LOG_DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := LOG_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		common.SysLog("database migration started")
		err = migrateLOGDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func migrateDB() error {
	err := DB.AutoMigrate(
		&Channel{},
		&Token{},
		&User{},
		&PasskeyCredential{},
		&Option{},
		&Redemption{},
		&Ability{},
		&Log{},
		&Midjourney{},
		&TopUp{},
		&QuotaData{},
		&Task{},
		&Model{},
		&Vendor{},
		&PrefillGroup{},
		&Setup{},
		&TwoFA{},
		&TwoFABackupCode{},
		&Plan{},
		&UserPlan{},
		&UserDailyPool{},
		&AdminPlanLog{},
		&UserAssetSnapshot{},
		&UserNotification{},
		&PlanOrder{},
		&TopupOrder{},
		&ChannelDisableRule{},
		&InviterRewardPayout{},
	)
	if err != nil {
		return err
	}
	// Seed default plans after migration
	if err := SeedDefaultPlans(); err != nil {
		common.SysLog("failed to seed default plans: " + err.Error())
	}
	// Migrate UserPlan snapshots after schema migration
	if err := migrateUserPlanSnapshots(); err != nil {
		common.SysLog("failed to migrate user plan snapshots: " + err.Error())
		// Don't fail startup, migration can be retried
	}
	// Backfill locked_by='admin' for historically locked rows (idempotent)
	if err := migrateUserPlanLockedBy(); err != nil {
		common.SysLog("failed to backfill user plan locked_by: " + err.Error())
		// Don't fail startup, migration can be retried
	}
	// Clear stale client-restriction / sticky-session / masquerade-hash flags left
	// over from the removed identity-masquerade stack. Idempotent via options row.
	if err := clearMasqueradeLegacyFlags(); err != nil {
		common.SysLog("failed to clear masquerade legacy flags: " + err.Error())
		// Don't fail startup, can be retried
	}
	return nil
}

// clearMasqueradeLegacyFlags zeroes DB columns that backed the removed
// identity-masquerade features (client restriction, sticky session,
// masquerade_hash). The schema fields are intentionally kept to avoid
// migration churn, but leaving the values populated would silently change
// the semantics of existing rows (access-control bypass, session drift).
// Guarded by an options row so it only runs once.
func clearMasqueradeLegacyFlags() error {
	const optionKey = "MasqueradeLegacyFlagsCleared"

	var existing Option
	if err := DB.Where(commonKeyCol+" = ?", optionKey).First(&existing).Error; err == nil {
		// Already ran.
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	common.SysLog("clearing stale masquerade config flags (one-time)")

	if err := DB.Model(&Token{}).
		Where("sticky_session = ? OR client_restriction_enabled = ? OR (allowed_clients IS NOT NULL AND allowed_clients != ?)", true, true, "").
		Updates(map[string]interface{}{
			"sticky_session":             false,
			"sticky_session_ttl":         3600,
			"client_restriction_enabled": false,
			"allowed_clients":            "",
		}).Error; err != nil {
		return err
	}

	if err := DB.Model(&Channel{}).
		Where("enable_client_restriction = ? OR (allowed_clients IS NOT NULL AND allowed_clients != ?) OR (masquerade_hash IS NOT NULL AND masquerade_hash != ?)", true, "", "").
		Updates(map[string]interface{}{
			"enable_client_restriction": false,
			"allowed_clients":           "",
			"masquerade_hash":           "",
		}).Error; err != nil {
		return err
	}

	if err := DB.Create(&Option{Key: optionKey, Value: "true"}).Error; err != nil {
		return err
	}
	common.SysLog("masquerade legacy flags cleared")
	return nil
}

// migrateUserPlanSnapshots populates snapshot fields in existing UserPlan records
// This migration is idempotent and can be run multiple times safely
func migrateUserPlanSnapshots() error {
	common.SysLog("starting user plan snapshot migration...")

	var totalMigrated int
	batchSize := 100

	// Process in batches to avoid memory issues with large datasets
	// Check for ANY missing snapshot field to catch both:
	// 1. Records never migrated (plan_name empty)
	// 2. Records from Phase 1 missing Phase 2 routing fields (plan_type empty)
	// Additionally: migrate plan_validity_days snapshot ONLY when the associated Plan has validity_days > 0.
	// (plan_validity_days = 0 is a valid value meaning "permanent" and should not trigger repeated migrations.)
	result := DB.Preload("Plan").
		Joins("LEFT JOIN plans ON plans.id = user_plans.plan_id").
		Where("user_plans.plan_name = ? OR user_plans.plan_name IS NULL OR user_plans.plan_type = ? OR user_plans.plan_type IS NULL OR user_plans.plan_channel_groups = ? OR user_plans.plan_channel_groups IS NULL OR (user_plans.plan_validity_days = 0 AND user_plans.plan_id IS NOT NULL AND plans.validity_days > 0)",
			"", "", "").
		FindInBatches(&[]UserPlan{}, batchSize, func(tx *gorm.DB, batch int) error {
			var userPlans []UserPlan
			tx.Find(&userPlans)

			for i := range userPlans {
				up := &userPlans[i]
				if up.Plan != nil {
					// Copy display & sorting snapshot fields from Plan template
					up.PlanName = up.Plan.Name
					up.PlanDisplayName = up.Plan.DisplayName
					up.PlanCategory = up.Plan.Category
					up.PlanPriority = up.Plan.Priority

					// Copy routing & access control snapshot fields
					up.PlanType = up.Plan.Type
					up.PlanChannelGroup = up.Plan.ChannelGroup
					up.PlanChannelGroups = up.Plan.ChannelGroups // Already JSON string
					up.PlanRateLimitRules = up.Plan.RateLimitRules
					up.PlanDailyQuotaLimit = up.Plan.DailyQuotaLimit
					if up.PlanValidityDays == 0 && up.Plan.ValidityDays > 0 {
						up.PlanValidityDays = up.Plan.ValidityDays
					}

					// Update all snapshot fields
					if err := DB.Model(up).Select(
						"plan_name",
						"plan_display_name",
						"plan_category",
						"plan_priority",
						"plan_type",
						"plan_channel_group",
						"plan_channel_groups",
						"plan_rate_limit_rules",
						"plan_daily_quota_limit",
						"plan_validity_days",
					).Updates(up).Error; err != nil {
						common.SysLog("failed to migrate user plan " + fmt.Sprint(up.Id) + ": " + err.Error())
						continue
					}
					totalMigrated++
				}
			}

			if batch > 1 {
				common.SysLog(fmt.Sprintf("migrated batch %d (%d records so far)", batch, totalMigrated))
			}

			return nil
		})

	if result.Error != nil {
		return result.Error
	}

	if totalMigrated > 0 {
		common.SysLog(fmt.Sprintf("user plan snapshot migration completed: %d records updated", totalMigrated))
	} else {
		common.SysLog("user plan snapshot migration: no records to migrate")
	}

	return nil
}

// migrateUserPlanLockedBy backfills locked_by='admin' for rows that were locked
// before the LockedBy field existed. Idempotent: only touches rows still missing the value.
func migrateUserPlanLockedBy() error {
	result := DB.Model(&UserPlan{}).
		Where("locked = ? AND (locked_by IS NULL OR locked_by = ?)", 1, "").
		Update("locked_by", "admin")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("user plan locked_by backfill: %d rows updated to 'admin'", result.RowsAffected))
	}
	return nil
}

func migrateDBFast() error {

	var wg sync.WaitGroup

	migrations := []struct {
		model interface{}
		name  string
	}{
		{&Channel{}, "Channel"},
		{&Token{}, "Token"},
		{&User{}, "User"},
		{&PasskeyCredential{}, "PasskeyCredential"},
		{&Option{}, "Option"},
		{&Redemption{}, "Redemption"},
		{&Ability{}, "Ability"},
		{&Log{}, "Log"},
		{&Midjourney{}, "Midjourney"},
		{&TopUp{}, "TopUp"},
		{&QuotaData{}, "QuotaData"},
		{&Task{}, "Task"},
		{&Model{}, "Model"},
		{&Vendor{}, "Vendor"},
		{&PrefillGroup{}, "PrefillGroup"},
		{&Setup{}, "Setup"},
		{&TwoFA{}, "TwoFA"},
		{&TwoFABackupCode{}, "TwoFABackupCode"},
		{&Plan{}, "Plan"},
		{&UserPlan{}, "UserPlan"},
		{&UserDailyPool{}, "UserDailyPool"},
		{&AdminPlanLog{}, "AdminPlanLog"},
		{&UserAssetSnapshot{}, "UserAssetSnapshot"},
		{&UserNotification{}, "UserNotification"},
		{&PlanOrder{}, "PlanOrder"},
		{&ChannelDisableRule{}, "ChannelDisableRule"},
		{&InviterRewardPayout{}, "InviterRewardPayout"},
	}
	// 动态计算migration数量，确保errChan缓冲区足够大
	errChan := make(chan error, len(migrations))

	for _, m := range migrations {
		wg.Add(1)
		go func(model interface{}, name string) {
			defer wg.Done()
			if err := DB.AutoMigrate(model); err != nil {
				errChan <- fmt.Errorf("failed to migrate %s: %v", name, err)
			}
		}(m.model, m.name)
	}

	// Wait for all migrations to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	common.SysLog("database migrated")
	return nil
}

func migrateLOGDB() error {
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}); err != nil {
		return err
	}
	return nil
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

func CloseDB() error {
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return err
		}
	}
	return closeDB(DB)
}

// checkMySQLChineseSupport ensures the MySQL connection and current schema
// default charset/collation can store Chinese characters. It allows common
// Chinese-capable charsets (utf8mb4, utf8, gbk, big5, gb18030) and panics otherwise.
func checkMySQLChineseSupport(db *gorm.DB) error {
	// 仅检测：当前库默认字符集/排序规则 + 各表的排序规则（隐含字符集）

	// Read current schema defaults
	var schemaCharset, schemaCollation string
	err := db.Raw("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()").Row().Scan(&schemaCharset, &schemaCollation)
	if err != nil {
		return fmt.Errorf("读取当前库默认字符集/排序规则失败 / Failed to read schema default charset/collation: %v", err)
	}

	toLower := func(s string) string { return strings.ToLower(s) }
	// Allowed charsets that can store Chinese text
	allowedCharsets := map[string]string{
		"utf8mb4": "utf8mb4_",
		"utf8":    "utf8_",
		"gbk":     "gbk_",
		"big5":    "big5_",
		"gb18030": "gb18030_",
	}
	isChineseCapable := func(cs, cl string) bool {
		csLower := toLower(cs)
		clLower := toLower(cl)
		if prefix, ok := allowedCharsets[csLower]; ok {
			if clLower == "" {
				return true
			}
			return strings.HasPrefix(clLower, prefix)
		}
		// 如果仅提供了排序规则，尝试按排序规则前缀判断
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(clLower, prefix) {
				return true
			}
		}
		return false
	}

	// 1) 当前库默认值必须支持中文
	if !isChineseCapable(schemaCharset, schemaCollation) {
		return fmt.Errorf("当前库默认字符集/排序规则不支持中文：schema(%s/%s)。请将库设置为 utf8mb4/utf8/gbk/big5/gb18030 / Schema default charset/collation is not Chinese-capable: schema(%s/%s). Please set to utf8mb4/utf8/gbk/big5/gb18030",
			schemaCharset, schemaCollation, schemaCharset, schemaCollation)
	}

	// 2) 所有物理表的排序规则（隐含字符集）必须支持中文
	type tableInfo struct {
		Name      string
		Collation *string
	}
	var tables []tableInfo
	if err := db.Raw("SELECT TABLE_NAME, TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'").Scan(&tables).Error; err != nil {
		return fmt.Errorf("读取表排序规则失败 / Failed to read table collations: %v", err)
	}

	var badTables []string
	for _, t := range tables {
		// NULL 或空表示继承库默认设置，已在上面校验库默认，视为通过
		if t.Collation == nil || *t.Collation == "" {
			continue
		}
		cl := *t.Collation
		// 仅凭排序规则判断是否中文可用
		ok := false
		lower := strings.ToLower(cl)
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(lower, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			badTables = append(badTables, fmt.Sprintf("%s(%s)", t.Name, cl))
		}
	}

	if len(badTables) > 0 {
		// 限制输出数量以避免日志过长
		maxShow := 20
		shown := badTables
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}
		return fmt.Errorf(
			"存在不支持中文的表，请修复其排序规则/字符集。示例（最多展示 %d 项）：%v / Found tables not Chinese-capable. Please fix their collation/charset. Examples (showing up to %d): %v",
			maxShow, shown, maxShow, shown,
		)
	}
	return nil
}

var (
	lastPingTime time.Time
	pingMutex    sync.Mutex
)

func PingDB() error {
	pingMutex.Lock()
	defer pingMutex.Unlock()

	if time.Since(lastPingTime) < time.Second*10 {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting sql.DB from GORM: %v", err)
		return err
	}

	err = sqlDB.Ping()
	if err != nil {
		log.Printf("Error pinging DB: %v", err)
		return err
	}

	lastPingTime = time.Now()
	common.SysLog("Database pinged successfully")
	return nil
}

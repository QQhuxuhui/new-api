package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// StartAffCronTasks 启动一级分销返佣相关后台任务:
//   - 每日清理 30 天之前的 user_login_ip_logs
//   - 每日归档已结算 1 年以上的 audit log
//
// 返现不再自动结算:每笔 pending 都由管理员在"返现审核"页人工通过后入账(见 aff_settle.go)。
//
// 注意:这些任务只能在 master 节点运行(IsMasterNode=true),否则多实例并发会导致
// 归档任务重复移动同一行数据。
func StartAffCronTasks() {
	if !common.IsMasterNode {
		common.SysLog("affiliate background tasks: skipped on non-master node")
		return
	}
	common.SysLog("starting affiliate background tasks")

	// 每日清理过期登录 IP 日志
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		runOnce := func() {
			deleted, err := model.CleanupOldUserLoginIpLogs(30)
			if err != nil {
				common.SysLog("cleanup user_login_ip_logs failed: " + err.Error())
				return
			}
			if deleted > 0 {
				common.SysLog("cleanup user_login_ip_logs deleted " + itoa(int(deleted)) + " rows")
			}
		}
		runOnce()
		for range ticker.C {
			runOnce()
		}
	}()

	// 每日归档:已结算 1 年以上的 audit log 移到归档表
	go func() {
		// 错峰:cleanup 在 0 时运行,归档延后 1 小时跑
		time.Sleep(1 * time.Hour)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		runOnce := func() {
			archived, err := model.ArchiveOldSettledLogs(365)
			if err != nil {
				common.SysLog("archive old settled aff_audit_logs failed: " + err.Error())
				return
			}
			if archived > 0 {
				common.SysLog("archive old settled aff_audit_logs: archived " + itoa(int(archived)) + " rows")
			}
		}
		runOnce()
		for range ticker.C {
			runOnce()
		}
	}()

	common.SysLog("affiliate background tasks started")
}

// itoa 轻量本地 int → string,避免 import strconv 仅为一处使用。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

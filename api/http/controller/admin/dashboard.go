package admin

import (
	"time"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/model"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/global"
)

type Dashboard struct{}

func (ct *Dashboard) Stats(c *gin.Context) {
	var totalUsers int64
	var totalPeers int64
	var totalGroups int64
	var totalLoginLogs int64
	var onlinePeers int64
	var recentLogins int64

	global.DB.Model(&model.User{}).Count(&totalUsers)
	global.DB.Model(&model.Peer{}).Count(&totalPeers)
	global.DB.Model(&model.Group{}).Count(&totalGroups)
	global.DB.Model(&model.LoginLog{}).Count(&totalLoginLogs)

	global.DB.Model(&model.Peer{}).Where("last_online_time > 0").Count(&onlinePeers)

	since := time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	global.DB.Model(&model.LoginLog{}).Where("created_at > ?", since).Count(&recentLogins)

	response.Success(c, gin.H{
		"total_users":    totalUsers,
		"total_peers":    totalPeers,
		"total_groups":   totalGroups,
		"online_peers":   onlinePeers,
		"total_logins":   totalLoginLogs,
		"recent_logins":  recentLogins,
	})
}

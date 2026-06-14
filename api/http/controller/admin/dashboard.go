package admin

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/model"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/global"
	"rustdesk-server/api/service"
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

func parseRelayValue(raw string) float64 {
	v := strings.TrimSpace(raw)
	v = strings.TrimSuffix(v, "Mb/s")
	v = strings.TrimSuffix(v, "mb/s")
	v = strings.TrimSpace(v)
	n, _ := strconv.ParseFloat(v, 64)
	return n
}

func (ct *Dashboard) Health(c *gin.Context) {
	idPort := global.Config.Admin.IdServerPort - 1
	relayPort := global.Config.Admin.RelayServerPort
	svc := service.AllService.ServerCmdService

	_, idErr := svc.SendCmd(idPort, "h", "")
	idOnline := idErr == nil

	_, relayErr := svc.SendCmd(relayPort, "h", "")
	relayOnline := relayErr == nil

	type usageRow struct {
		IP      string  `json:"ip"`
		Time    int64   `json:"time"`
		Total   float64 `json:"total"`
		Highest float64 `json:"highest"`
		Avg     float64 `json:"avg"`
		Speed   float64 `json:"speed"`
	}

	var usage []usageRow
	activeConns := 0

	if usageRaw, err := svc.SendCmd(relayPort, "u", ""); err == nil && usageRaw != "" {
		lines := strings.Split(strings.TrimSpace(usageRaw), "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) < 6 {
				continue
			}
			ip := strings.TrimRight(parts[0], ":")
			timeVal, _ := strconv.ParseInt(strings.TrimRight(parts[1], "s"), 10, 64)
			totalVal, _ := strconv.ParseFloat(strings.TrimRight(parts[2], "MB"), 64)
			highestVal, _ := strconv.ParseFloat(strings.TrimRight(parts[3], "kb/s"), 64)
			avgVal, _ := strconv.ParseFloat(strings.TrimRight(parts[4], "kb/s"), 64)
			speedVal, _ := strconv.ParseFloat(strings.TrimRight(parts[5], "kb/s"), 64)
			usage = append(usage, usageRow{
				IP: ip, Time: timeVal, Total: totalVal,
				Highest: highestVal, Avg: avgVal, Speed: speedVal,
			})
		}
		activeConns = len(usage)
		sort.Slice(usage, func(i, j int) bool {
			return usage[i].Total > usage[j].Total
		})
		if len(usage) > 5 {
			usage = usage[:5]
		}
	}

	totalBW := 0.0
	singleBW := 0.0
	limitSpeed := 0.0
	if raw, err := svc.SendCmd(relayPort, "total-bandwidth", ""); err == nil {
		totalBW = parseRelayValue(raw)
	}
	if raw, err := svc.SendCmd(relayPort, "single-bandwidth", ""); err == nil {
		singleBW = parseRelayValue(raw)
	}
	if raw, err := svc.SendCmd(relayPort, "limit-speed", ""); err == nil {
		limitSpeed = parseRelayValue(raw)
	}

	response.Success(c, gin.H{
		"id_server": gin.H{
			"online": idOnline,
		},
		"relay_server": gin.H{
			"online": relayOnline,
		},
		"usage":              usage,
		"active_connections": activeConns,
		"total_bandwidth":    totalBW,
		"single_bandwidth":   singleBW,
		"limit_speed":        limitSpeed,
	})
}

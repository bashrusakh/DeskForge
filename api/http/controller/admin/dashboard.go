package admin

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

type Dashboard struct{}

const onlinePeerWindow = 5 * time.Minute

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

	onlineSince := time.Now().Add(-onlinePeerWindow).Unix()
	global.DB.Model(&model.Peer{}).Where("last_online_time > ?", onlineSince).Count(&onlinePeers)

	recentSince := time.Now().UTC().Add(-24 * time.Hour)
	global.DB.Model(&model.LoginLog{}).Where("created_at > ?", recentSince).Count(&recentLogins)

	response.Success(c, gin.H{
		"total_users":   totalUsers,
		"total_peers":   totalPeers,
		"total_groups":  totalGroups,
		"online_peers":  onlinePeers,
		"total_logins":  totalLoginLogs,
		"recent_logins": recentLogins,
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

type usageRow struct {
	IP      string  `json:"ip"`
	Time    int64   `json:"time"`
	Total   float64 `json:"total"`
	Highest float64 `json:"highest"`
	Avg     float64 `json:"avg"`
	Speed   float64 `json:"speed"`
}

func (ct *Dashboard) Health(c *gin.Context) {
	idPort := global.Config.Admin.IdServerPort - 1
	relayPort := global.Config.Admin.RelayServerPort
	svc := service.AllService.ServerCmdService

	var (
		wg                                          sync.WaitGroup
		idOnline, relayOnline                       bool
		usageRaw, totalBWRaw, singleBWRaw, limitRaw string
	)

	run := func(port int, cmd string, ok *bool, out *string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := svc.SendCmd(port, cmd, "")
			if ok != nil {
				*ok = err == nil
			}
			if out != nil && err == nil {
				*out = res
			}
		}()
	}

	run(idPort, "h", &idOnline, nil)
	run(relayPort, "h", &relayOnline, nil)
	run(relayPort, "u", nil, &usageRaw)
	run(relayPort, "total-bandwidth", nil, &totalBWRaw)
	run(relayPort, "single-bandwidth", nil, &singleBWRaw)
	run(relayPort, "limit-speed", nil, &limitRaw)

	wg.Wait()

	usage := []usageRow{}
	activeConns := 0
	currentTotalKbps := 0.0
	currentPeakKbps := 0.0
	if usageRaw != "" {
		for _, line := range strings.Split(strings.TrimSpace(usageRaw), "\n") {
			parts := strings.Fields(line)
			if len(parts) < 6 {
				continue
			}
			ip := strings.TrimRight(parts[0], ":")
			timeVal, _ := strconv.ParseInt(strings.TrimSuffix(parts[1], "s"), 10, 64)
			totalVal, _ := strconv.ParseFloat(strings.TrimSuffix(parts[2], "MB"), 64)
			highestVal, _ := strconv.ParseFloat(strings.TrimSuffix(parts[3], "kb/s"), 64)
			avgVal, _ := strconv.ParseFloat(strings.TrimSuffix(parts[4], "kb/s"), 64)
			speedVal, _ := strconv.ParseFloat(strings.TrimSuffix(parts[5], "kb/s"), 64)
			usage = append(usage, usageRow{
				IP: ip, Time: timeVal, Total: totalVal,
				Highest: highestVal, Avg: avgVal, Speed: speedVal,
			})
			currentTotalKbps += speedVal
			if speedVal > currentPeakKbps {
				currentPeakKbps = speedVal
			}
		}
		activeConns = len(usage)
		sort.Slice(usage, func(i, j int) bool {
			return usage[i].Total > usage[j].Total
		})
		if len(usage) > 5 {
			usage = usage[:5]
		}
	}

	response.Success(c, gin.H{
		"id_server": gin.H{
			"online": idOnline,
			"port":   idPort,
		},
		"relay_server": gin.H{
			"online": relayOnline,
			"port":   relayPort,
		},
		"usage":              usage,
		"active_connections": activeConns,
		"current_total_kbps": currentTotalKbps,
		"current_peak_kbps":  currentPeakKbps,
		"total_bandwidth":    parseRelayValue(totalBWRaw),
		"single_bandwidth":   parseRelayValue(singleBWRaw),
		"limit_speed":        parseRelayValue(limitRaw),
	})
}

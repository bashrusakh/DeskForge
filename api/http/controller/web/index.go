package web

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
)

type Index struct {
}

func (i *Index) Index(c *gin.Context) {
	c.Redirect(302, "/admin/")
}

// resolveApiServer returns the configured api-server, or — when it is empty or a
// loopback address — derives it from the incoming request so the web client talks
// to the host the browser actually used instead of the server's own loopback
// (otherwise the web client opens localhost:port). An explicitly configured
// non-loopback api-server is always respected. Honors X-Forwarded-Proto/Host.
func resolveApiServer(c *gin.Context, configured string) string {
	s := strings.TrimSpace(configured)
	low := strings.ToLower(s)
	if s == "" || strings.Contains(low, "127.0.0.1") || strings.Contains(low, "localhost") || strings.Contains(low, "[::1]") {
		scheme := "http"
		if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
			scheme = strings.TrimSpace(strings.Split(proto, ",")[0])
		} else if c.Request.TLS != nil {
			scheme = "https"
		}
		host := c.Request.Host
		if fwd := c.GetHeader("X-Forwarded-Host"); fwd != "" {
			host = strings.TrimSpace(strings.Split(fwd, ",")[0])
		}
		if host != "" {
			return scheme + "://" + host
		}
	}
	return s
}

func (i *Index) ConfigJs(c *gin.Context) {
	apiServer := resolveApiServer(c, global.Config.Rustdesk.ApiServer)
	idServer := global.Config.Rustdesk.IdServer
	magicQueryonline := global.Config.Rustdesk.WebclientMagicQueryonline
	tmp := fmt.Sprintf(`localStorage.setItem('api-server', '%v');
localStorage.setItem('rendezvous-server', '%v');
const ws2_prefix = 'wc-';
localStorage.setItem(ws2_prefix+'api-server', '%v');

window.webclient_magic_queryonline = %d;
window.ws_host = '%v';
`, apiServer, idServer, apiServer, magicQueryonline, global.Config.Rustdesk.WsHost)
	//	tmp := `
	//localStorage.setItem('api-server', "` + apiServer + `")
	//const ws2_prefix = 'wc-'
	//localStorage.setItem(ws2_prefix+'api-server', "` + apiServer + `")
	//
	//window.webclient_magic_queryonline = ` + magicQueryonline + ``

	c.Header("Content-Type", "application/javascript")
	c.String(200, tmp)
}

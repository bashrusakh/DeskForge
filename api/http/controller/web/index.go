package web

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
)

type Index struct {
}

func (i *Index) Index(c *gin.Context) {
	c.Redirect(302, "/admin/")
}

func (i *Index) ConfigJs(c *gin.Context) {
	apiServer := global.Config.Rustdesk.ApiServer
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

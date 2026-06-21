package router

import (
	"github.com/gin-gonic/gin"
	_ "rustdesk-server/api/docs/admin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/controller/admin"
	"rustdesk-server/api/http/controller/admin/my"
	"rustdesk-server/api/http/middleware"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func Init(g *gin.Engine) {

	//swagger
	//g.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	if global.Config.App.ShowSwagger == 1 {
		g.GET("/admin/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.InstanceName("admin")))
	}

	adg := g.Group("/api/admin")
	LoginBind(adg)
	adg.POST("/user/register", (&admin.User{}).Register)

	ConfigBind(adg)

	// NoCache ПЕРЕД auth — иначе BackendUserAuth/AdminPrivilege могут прервать цепочку
	// через c.Abort() и заголовки не дойдут до клиента.
	adg.Use(middleware.NoCache(), middleware.BackendUserAuth())
	FileBind(adg)
	DashboardBind(adg)
	UserBind(adg)
	GroupBind(adg)
	TagBind(adg)
	AddressBookBind(adg)
	PeerBind(adg)
	OauthBind(adg)
	LoginLogBind(adg)
	AuditBind(adg)
	AddressBookCollectionBind(adg)
	AddressBookCollectionRuleBind(adg)
	UserTokenBind(adg)

	//deprecated by ConfigBind
	//rs := &admin.Rustdesk{}
	//adg.GET("/server-config", rs.ServerConfig)
	//adg.GET("/app-config", rs.AppConfig)
	//deprecated end

	ShareRecordBind(adg)
	MyBind(adg)

	RustdeskCmdBind(adg)
	DeviceGroupBind(adg)
	CustomBuildBind(adg)
	CustomPresetBind(adg)
	GithubBuildConfigBind(adg)

	// Public (no-auth) эндпоинты custom_build — вынесены из adg, чтобы не наследовать
	// BackendUserAuth. Доступ по download_key (capability URL).
	cbCont := &admin.CustomBuild{}
	g.GET("/api/admin/custom_build/public/detailByKey/:key", cbCont.DetailByKey)
	g.GET("/api/admin/custom_build/public/download/:key", cbCont.DownloadByKey)

	//g.StaticFS("/upload", http.Dir(global.Config.Gin.ResourcesPath+"/upload"))
}

func RustdeskCmdBind(adg *gin.RouterGroup) {
	cont := &admin.Rustdesk{}
	rg := adg.Group("/rustdesk").Use(middleware.AdminPrivilege())
	// AU-S-001: журналируем мутирующие server-команды (кто/что/когда/результат).
	audit := middleware.ServerCmdAudit()
	rg.POST("/sendCmd", audit, cont.SendCmd)
	rg.GET("/cmdList", cont.CmdList)
	rg.POST("/cmdDelete", audit, cont.CmdDelete)
	rg.POST("/cmdCreate", audit, cont.CmdCreate)
	rg.POST("/cmdUpdate", audit, cont.CmdUpdate)
	rg.GET("/cmdAuditList", cont.CmdAuditList)
}
func LoginBind(rg *gin.RouterGroup) {
	cont := &admin.Login{}
	rg.POST("/login", cont.Login)
	rg.GET("/captcha", cont.Captcha)
	rg.POST("/logout", cont.Logout)
	rg.GET("/login-options", cont.LoginOptions)
	rg.POST("/oidc/auth", cont.OidcAuth)
	rg.GET("/oidc/auth-query", cont.OidcAuthQuery)
}

func FileBind(rg *gin.RouterGroup) {
	aR := rg.Group("/file")
	{
		cont := &admin.File{}
		aR.GET("/oss_token", cont.OssToken)
		aR.POST("/notify", cont.Notify)
		aR.POST("/upload", cont.Upload)
	}
}

func UserBind(rg *gin.RouterGroup) {
	aR := rg.Group("/user")
	{
		cont := &admin.User{}
		aR.GET("/current", cont.Current)
		aR.POST("/changeCurPwd", cont.ChangeCurPwd)
		aR.POST("/myOauth", cont.MyOauth)
		//aR.GET("/myPeer", cont.MyPeer)
	}
	aRP := rg.Group("/user").Use(middleware.AdminPrivilege())
	{
		cont := &admin.User{}
		aRP.GET("/list", cont.List)
		aRP.GET("/detail/:id", cont.Detail)
		aRP.POST("/create", cont.Create)
		aRP.POST("/update", cont.Update)
		aRP.POST("/delete", cont.Delete)
		aRP.POST("/changePwd", cont.UpdatePassword)
		aRP.POST("/groupUsers", cont.GroupUsers)
	}
}

func GroupBind(rg *gin.RouterGroup) {
	aR := rg.Group("/group").Use(middleware.AdminPrivilege())
	{
		cont := &admin.Group{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}
}

func DeviceGroupBind(rg *gin.RouterGroup) {
	aR := rg.Group("/device_group").Use(middleware.AdminPrivilege())
	{
		cont := &admin.DeviceGroup{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}
}

func TagBind(rg *gin.RouterGroup) {
	aR := rg.Group("/tag").Use(middleware.AdminPrivilege())
	{
		cont := &admin.Tag{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}
}

func AddressBookBind(rg *gin.RouterGroup) {
	aR := rg.Group("/address_book")
	{
		cont := &admin.AddressBook{}
		aR.POST("/shareByWebClient", cont.ShareByWebClient)

		arp := aR.Use(middleware.AdminPrivilege())
		arp.GET("/list", cont.List)
		//arp.GET("/detail/:id", cont.Detail)
		arp.POST("/create", cont.Create)
		arp.POST("/update", cont.Update)
		arp.POST("/delete", cont.Delete)
		arp.POST("/batchCreate", cont.BatchCreate)
		arp.POST("/batchCreateFromPeers", cont.BatchCreateFromPeers)

	}
}
func PeerBind(rg *gin.RouterGroup) {
	aR := rg.Group("/peer")
	aR.POST("/simpleData", (&admin.Peer{}).SimpleData)
	aR.Use(middleware.AdminPrivilege())
	{
		cont := &admin.Peer{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
		aR.POST("/batchDelete", cont.BatchDelete)
	}
}

func OauthBind(rg *gin.RouterGroup) {
	aR := rg.Group("/oauth")
	{
		cont := &admin.Oauth{}
		aR.POST("/confirm", cont.Confirm)
		aR.POST("/bind", cont.ToBind)
		aR.POST("/bindConfirm", cont.BindConfirm)
		aR.POST("/unbind", cont.Unbind)
		aR.GET("/info", cont.Info)
	}
	arp := aR.Use(middleware.AdminPrivilege())
	{
		cont := &admin.Oauth{}
		arp.GET("/list", cont.List)
		arp.GET("/detail/:id", cont.Detail)
		arp.POST("/create", cont.Create)
		arp.POST("/update", cont.Update)
		arp.POST("/delete", cont.Delete)

	}

}
func LoginLogBind(rg *gin.RouterGroup) {
	cont := &admin.LoginLog{}
	aR := rg.Group("/login_log").Use(middleware.AdminPrivilege())
	aR.GET("/list", cont.List)
	aR.POST("/delete", cont.Delete)
	aR.POST("/batchDelete", cont.BatchDelete)
}
func AuditBind(rg *gin.RouterGroup) {
	cont := &admin.Audit{}
	aR := rg.Group("/audit_conn").Use(middleware.AdminPrivilege())
	aR.GET("/list", cont.ConnList)
	aR.POST("/delete", cont.ConnDelete)
	aR.POST("/batchDelete", cont.BatchConnDelete)
	afR := rg.Group("/audit_file").Use(middleware.AdminPrivilege())
	afR.GET("/list", cont.FileList)
	afR.POST("/delete", cont.FileDelete)
	afR.POST("/batchDelete", cont.BatchFileDelete)
}
func AddressBookCollectionBind(rg *gin.RouterGroup) {
	aR := rg.Group("/address_book_collection").Use(middleware.AdminPrivilege())
	{
		cont := &admin.AddressBookCollection{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}

}
func AddressBookCollectionRuleBind(rg *gin.RouterGroup) {
	aR := rg.Group("/address_book_collection_rule").Use(middleware.AdminPrivilege())
	{
		cont := &admin.AddressBookCollectionRule{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}
}
func UserTokenBind(rg *gin.RouterGroup) {
	aR := rg.Group("/user_token").Use(middleware.AdminPrivilege())
	cont := &admin.UserToken{}
	aR.GET("/list", cont.List)
	aR.POST("/delete", cont.Delete)
	aR.POST("/batchDelete", cont.BatchDelete)
}
func ConfigBind(rg *gin.RouterGroup) {
	aR := rg.Group("/config")
	rs := &admin.Config{}

	aR.GET("/admin", rs.AdminConfig)

	// /server и /app нужны всем авторизованным юзерам: web-client при
	// логине сохраняет id_server/key/api-server в localStorage и читает
	// флаг web_client для отображения раздела в UI. /all отдаёт
	// супермножество (включая register, ws_host, show_swagger) и нужен
	// только админ-панели — потому защищён AdminPrivilege дополнительно.
	aR.Use(middleware.BackendUserAuth())
	aR.GET("/server", rs.ServerConfig)
	aR.GET("/app", rs.AppConfig)

	aR.Use(middleware.AdminPrivilege())
	aR.GET("/all", rs.AllConfig)
}

/*
func FileBind(rg *gin.RouterGroup) {
	aR := rg.Group("/file")
	{
		cont := &admin.File{}
		aR.POST("/notify", cont.Notify)
		aR.OPTIONS("/oss_token", nil)
		aR.OPTIONS("/upload", nil)
		aR.GET("/oss_token", cont.OssToken)
		aR.POST("/upload", cont.Upload)
	}
}*/

func MyBind(rg *gin.RouterGroup) {
	{
		// Personal Address Book share rules need to list groups+users
		// for the picker without exposing the full admin directory.
		cont := &admin.User{}
		rg.POST("/my/groupUsers", cont.GroupUsersForShare)
	}

	{
		cont := &my.ShareRecord{}
		rg.GET("/my/share_record/list", cont.List)
		rg.POST("/my/share_record/delete", cont.Delete)
		rg.POST("/my/share_record/batchDelete", cont.BatchDelete)
	}

	{
		cont := &my.AddressBook{}
		rg.GET("/my/address_book/list", cont.List)
		rg.POST("/my/address_book/create", cont.Create)
		rg.POST("/my/address_book/update", cont.Update)
		rg.POST("/my/address_book/delete", cont.Delete)
		rg.POST("/my/address_book/batchCreateFromPeers", cont.BatchCreateFromPeers)
		rg.POST("/my/address_book/batchUpdateTags", cont.BatchUpdateTags)
	}

	{
		cont := &my.Tag{}
		rg.GET("/my/tag/list", cont.List)
		rg.POST("/my/tag/create", cont.Create)
		rg.POST("/my/tag/update", cont.Update)
		rg.POST("/my/tag/delete", cont.Delete)
	}

	{
		cont := &my.AddressBookCollection{}
		rg.GET("/my/address_book_collection/list", cont.List)
		rg.POST("/my/address_book_collection/create", cont.Create)
		rg.POST("/my/address_book_collection/update", cont.Update)
		rg.POST("/my/address_book_collection/delete", cont.Delete)
	}

	{
		cont := &my.AddressBookCollectionRule{}
		rg.GET("/my/address_book_collection_rule/list", cont.List)
		rg.POST("/my/address_book_collection_rule/create", cont.Create)
		rg.POST("/my/address_book_collection_rule/update", cont.Update)
		rg.POST("/my/address_book_collection_rule/delete", cont.Delete)
	}
	{
		cont := &my.Peer{}
		rg.GET("/my/peer/list", cont.List)
		rg.POST("/my/peer/delete", cont.Delete)
		rg.POST("/my/peer/batchDelete", cont.BatchDelete)

	}

	{
		cont := &my.LoginLog{}
		rg.GET("/my/login_log/list", cont.List)
		rg.POST("/my/login_log/delete", cont.Delete)
		rg.POST("/my/login_log/batchDelete", cont.BatchDelete)
	}
}

func DashboardBind(rg *gin.RouterGroup) {
	cont := &admin.Dashboard{}
	rg.GET("/dashboard/stats", cont.Stats)
	rg.GET("/dashboard/health", cont.Health)
}

func ShareRecordBind(rg *gin.RouterGroup) {
	aR := rg.Group("/share_record").Use(middleware.AdminPrivilege())
	{
		cont := &admin.ShareRecord{}
		aR.GET("/list", cont.List)
		aR.POST("/delete", cont.Delete)
		aR.POST("/batchDelete", cont.BatchDelete)
	}

}

func CustomBuildBind(rg *gin.RouterGroup) {
	cont := &admin.CustomBuild{}
	aR := rg.Group("/custom_build").Use(middleware.AdminPrivilege())
	aR.GET("/list", cont.List)
	// /detail/:id intentionally not exposed: never had a UI consumer (BUGS.md B-014).
	aR.POST("/create", cont.Create)
	aR.POST("/delete", cont.Delete)
}

func CustomPresetBind(rg *gin.RouterGroup) {
	cont := &admin.CustomPreset{}
	aR := rg.Group("/custom_preset").Use(middleware.AdminPrivilege())
	aR.GET("/list", cont.List)
	aR.GET("/detail/:id", cont.Detail)
	aR.POST("/create", cont.Create)
	aR.POST("/update", cont.Update)
	aR.POST("/delete", cont.Delete)
}

func GithubBuildConfigBind(rg *gin.RouterGroup) {
	cont := &admin.GithubBuildConfig{}
	aR := rg.Group("/github_build_config").Use(middleware.AdminPrivilege())
	aR.GET("/get", cont.Get)
	aR.POST("/save", cont.Save)
	aR.POST("/generate_key", cont.GenerateKey)
	aR.POST("/test", cont.Test)
	aR.POST("/sync_secret", cont.SyncSecret)
	aR.POST("/dispatch_test", cont.DispatchTest)
}

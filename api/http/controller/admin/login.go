package admin

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/controller/api"
	"rustdesk-server/api/http/request/admin"
	apiReq "rustdesk-server/api/http/request/api"
	"rustdesk-server/api/http/response"
	adResp "rustdesk-server/api/http/response/admin"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

type Login struct {
}

// Login authenticates an administrator before the admin middleware is applied.
// @Tags Auth
// @Summary Log in to the admin panel
// @Description Public pre-authentication route for administrator credentials; it returns the admin access-token payload on success.
// @Accept  json
// @Produce  json
// @Param body body admin.Login true "Admin login credentials payload"
// @Success 200 {object} response.Response{data=adResp.LoginPayload}
// @Failure 500 {object} response.Response
// @Router /admin/login [post]
func (ct *Login) Login(c *gin.Context) {
	if global.Config.App.DisablePwdLogin {
		response.Fail(c, 101, response.TranslateMsg(c, "PwdLoginDisabled"))
		return
	}

	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()
	_, needCaptcha := loginLimiter.CheckSecurityStatus(clientIp)

	f := &admin.Login{}
	err := c.ShouldBindJSON(f)
	if err != nil {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "ParamsError", c.RemoteIP(), clientIp))
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "ParamsError", c.RemoteIP(), clientIp))
		response.Fail(c, 101, errList[0])
		return
	}

	if needCaptcha {
		if f.CaptchaId == "" || f.Captcha == "" || !loginLimiter.VerifyCaptcha(f.CaptchaId, f.Captcha) {
			response.Fail(c, 101, response.TranslateMsg(c, "CaptchaError"))
			return
		}
	}

	u := service.AllService.UserService.InfoByUsernamePassword(f.Username, f.Password)

	if u.Id == 0 {
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "UsernameOrPasswordError", c.RemoteIP(), clientIp))
		loginLimiter.RecordFailedAttempt(clientIp)
		if _, needCaptcha = loginLimiter.CheckSecurityStatus(clientIp); needCaptcha {
			response.Fail(c, 110, response.TranslateMsg(c, "UsernameOrPasswordError"))
		} else {
			response.Fail(c, 101, response.TranslateMsg(c, "UsernameOrPasswordError"))
		}
		return
	}

	if !service.AllService.UserService.CheckUserEnable(u) {
		if needCaptcha {
			response.Fail(c, 110, response.TranslateMsg(c, "UserDisabled"))
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "UserDisabled"))
		return
	}

	ut := service.AllService.UserService.Login(u, &model.LoginLog{
		UserId:   u.Id,
		Client:   model.LoginLogClientWebAdmin,
		Uuid:     "", //must be empty
		Ip:       clientIp,
		Type:     model.LoginLogTypeAccount,
		Platform: f.Platform,
	})

	// пјЊ
	loginLimiter.RemoveAttempts(clientIp)
	responseLoginSuccess(c, u, ut.Token)
}

// Captcha returns a CAPTCHA challenge when the login limiter requires one.
// @Tags Auth
// @Summary Get an admin login CAPTCHA
// @Description Public pre-authentication route; a challenge is returned only when the client IP requires CAPTCHA verification.
// @Produce json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/captcha [get]
func (ct *Login) Captcha(c *gin.Context) {
	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()
	banned, needCaptcha := loginLimiter.CheckSecurityStatus(clientIp)
	if banned {
		response.Fail(c, 101, response.TranslateMsg(c, "LoginBanned"))
		return
	}
	if !needCaptcha {
		response.Fail(c, 101, response.TranslateMsg(c, "NoCaptchaRequired"))
		return
	}
	err, captcha := loginLimiter.RequireCaptcha()
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "CaptchaError")+err.Error())
		return
	}
	err, b64 := loginLimiter.DrawCaptcha(captcha.Content)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "CaptchaError")+err.Error())
		return
	}
	response.Success(c, gin.H{
		"captcha": gin.H{
			"id":  captcha.Id,
			"b64": b64,
		},
	})
}

// Logout ends the current admin session when an api-token is supplied.
// @Tags Auth
// @Summary Log out of the admin panel
// @Description Clears the supplied admin access token when it identifies a current user; the route is registered before the admin authentication middleware.
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/logout [post]
// @Security token
func (ct *Login) Logout(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	token, ok := c.Get("token")
	if ok {
		if err := service.AllService.UserService.Logout(u, token.(string)); err != nil {
			response.FailStatus(c, http.StatusInternalServerError, 101, "logout failed")
			return
		}
	}
	response.Success(c, nil)
}

// LoginOptions returns public admin login capabilities and configured OIDC providers.
// @Tags Auth
// @Summary Get admin login options
// @Description Public pre-authentication route returning registration, password-login, CAPTCHA, and OIDC-provider capabilities.
// @Produce  json
// @Success 200 {object} response.Response "Public admin login capabilities"
// @Failure 400 {object} response.Response "Login options could not be read"
// @Router /admin/login-options [get]
func (ct *Login) LoginOptions(c *gin.Context) {
	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()
	banned, needCaptcha := loginLimiter.CheckSecurityStatus(clientIp)
	if banned {
		response.Fail(c, 101, response.TranslateMsg(c, "LoginBanned"))
		return
	}
	ops := service.AllService.OauthService.GetOauthProviders()
	response.Success(c, gin.H{
		"ops":          ops,
		"register":     global.Config.App.Register,
		"need_captcha": needCaptcha,
		"disable_pwd":  global.Config.App.DisablePwdLogin,
		"auto_oidc":    global.Config.App.DisablePwdLogin && len(ops) == 1,
	})
}

// OidcAuth starts the public admin OIDC authorization flow.
// @Tags Oauth
// @Summary Start admin OIDC authorization
// @Description Public pre-authentication route returning the state code and provider URL for the admin OIDC flow.
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response{data=map[string]string} "Authorization state code and provider URL"
// @Failure 400 {object} response.ErrorResponse "OIDC authorization could not be started"
// @Router /admin/oidc/auth [post]
func (ct *Login) OidcAuth(c *gin.Context) {
	// o := &api.Oauth{}
	// o.OidcAuth(c)
	f := &apiReq.OidcAuthRequest{}
	err := c.ShouldBindJSON(f)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	err, state, verifier, nonce, url := service.AllService.OauthService.BeginAuth(f.Op)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, err.Error()))
		return
	}

	service.AllService.OauthService.SetOauthCache(state, &service.OauthCacheItem{
		Action:     service.OauthActionTypeLogin,
		Op:         f.Op,
		Id:         f.Id,
		DeviceType: "webadmin",
		// DeviceOs: ct.Platform(c),
		DeviceOs: f.DeviceInfo.Os,
		Uuid:     f.Uuid,
		Verifier: verifier,
		Nonce:    nonce,
	}, 5*60)

	response.Success(c, gin.H{
		"code": state,
		"url":  url,
	})
}

// OidcAuthQuery completes the public admin OIDC authorization flow.
// @Tags Oauth
// @Summary Complete admin OIDC authorization
// @Description Public pre-authentication route exchanging the OIDC state code and provider callback values for an admin login payload.
// @Produce  json
// @Success 200 {object} response.Response{data=adResp.LoginPayload}
// @Failure 400 {object} response.ErrorResponse "OIDC authorization query failed"
// @Router /admin/oidc/auth-query [get]
func (ct *Login) OidcAuthQuery(c *gin.Context) {
	o := &api.Oauth{}
	u, ut := o.OidcAuthQueryPre(c)
	if ut == nil {
		return
	}
	responseLoginSuccess(c, u, ut.Token)
}

func responseLoginSuccess(c *gin.Context, u *model.User, token string) {
	lp := &adResp.LoginPayload{}
	lp.FromUser(u)
	lp.Token = token
	lp.RouteNames = service.AllService.UserService.RouteNames(u)
	response.Success(c, lp)
}

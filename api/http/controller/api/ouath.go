package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/api"
	"rustdesk-server/api/http/response"
	apiResp "rustdesk-server/api/http/response/api"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

type Oauth struct {
}

// OidcAuth starts an OIDC authorization flow and returns its state code and URL.
// @Tags Oauth
// @Summary Start OIDC authorization
// @Description Starts OIDC authorization and returns the provider URL with the state code used by the follow-up query.
// @Accept  json
// @Produce  json
// @Success 200 {object} map[string]string "Authorization state code and provider URL"
// @Failure 400 {object} response.ErrorResponse "OIDC authorization could not be started"
// @Router /oidc/auth [post]
func (o *Oauth) OidcAuth(c *gin.Context) {
	f := &api.OidcAuthRequest{}
	err := c.ShouldBindJSON(&f)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	oauthService := service.AllService.OauthService

	err, state, verifier, nonce, url := oauthService.BeginAuth(f.Op)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, err.Error()))
		return
	}

	service.AllService.OauthService.SetOauthCache(state, &service.OauthCacheItem{
		Action:     service.OauthActionTypeLogin,
		Id:         f.Id,
		Op:         f.Op,
		Uuid:       f.Uuid,
		DeviceName: f.DeviceInfo.Name,
		DeviceOs:   f.DeviceInfo.Os,
		DeviceType: f.DeviceInfo.Type,
		Verifier:   verifier,
		Nonce:      nonce,
	}, 5*60)
	//fmt.Println("code url", code, url)
	c.JSON(http.StatusOK, gin.H{
		"code": state,
		"url":  url,
	})
}

func (o *Oauth) OidcAuthQueryPre(c *gin.Context) (*model.User, *model.UserToken) {
	var u *model.User
	var ut *model.UserToken
	q := &api.OidcAuthQuery{}

	if err := c.ShouldBindQuery(q); err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+": "+err.Error())
		return nil, nil
	}

	//  OAuth
	v := service.AllService.OauthService.GetOauthCache(q.Code)
	if v == nil {
		response.Error(c, response.TranslateMsg(c, "OauthExpired"))
		return nil, nil
	}

	//  UserId  0пјЊ
	if v.UserId == 0 {
		//fix: 1.4.2 webclient oidc
		c.JSON(http.StatusOK, gin.H{"message": "Authorization in progress, please login and bind", "error": "No authed oidc is found"})
		return nil, nil
	}

	u = service.AllService.UserService.InfoById(v.UserId)
	if u == nil {
		response.Error(c, response.TranslateMsg(c, "UserNotFound"))
		return nil, nil
	}

	//  OAuth
	service.AllService.OauthService.DeleteOauthCache(q.Code)

	ut = service.AllService.UserService.Login(u, &model.LoginLog{
		UserId:   u.Id,
		Client:   v.DeviceType,
		DeviceId: v.Id,
		Uuid:     v.Uuid,
		Ip:       c.ClientIP(),
		Type:     model.LoginLogTypeOauth,
		Platform: v.DeviceOs,
	})

	if ut == nil {
		response.Error(c, response.TranslateMsg(c, "LoginFailed"))
		return nil, nil
	}

	return u, ut
}

// OidcAuthQuery exchanges a completed OIDC authorization state for a login token.
// @Tags Oauth
// @Summary Complete OIDC authorization
// @Description Exchanges the OIDC state code and provider callback values for the Rust client login response.
// @Produce  json
// @Success 200 {object} apiResp.LoginRes
// @Failure 400 {object} response.ErrorResponse "OIDC authorization query failed"
// @Router /oidc/auth-query [get]
func (o *Oauth) OidcAuthQuery(c *gin.Context) {
	u, ut := o.OidcAuthQueryPre(c)
	if u == nil || ut == nil {
		return
	}
	c.JSON(http.StatusOK, apiResp.LoginRes{
		AccessToken: ut.Token,
		Type:        "access_token",
		User:        *(&apiResp.UserPayload{}).FromUser(u),
	})
}

// OauthCallback handles the provider callback for every registered OIDC/OAuth alias.
// @Tags Oauth
// @Summary Handle the OIDC provider callback
// @Description Renders the success/failure HTML result pages or redirects to the admin OAuth binding/app route. The same handler is registered for all four callback aliases below.
// @Produce text/html
// @Success 200 {string} string "Rendered OAuth success or failure HTML page"
// @Success 302 {string} string "Redirect to the admin OAuth binding or app route"
// @Header 302 {string} Location "Admin OAuth binding or app redirect target"
// @Router /oauth/callback [get]
// @Router /oauth/login [get]
// @Router /oidc/callback [get]
// @Router /oidc/login [get]
func (o *Oauth) OauthCallback(c *gin.Context) {
	state := c.Query("state")
	if state == "" {
		c.HTML(http.StatusOK, "oauth_fail.html", gin.H{
			"message":     "ParamIsEmpty",
			"sub_message": "state",
		})
		return
	}
	cacheKey := state
	oauthService := service.AllService.OauthService

	oauthCache := oauthService.GetOauthCache(cacheKey)
	if oauthCache == nil {
		c.HTML(http.StatusOK, "oauth_fail.html", gin.H{
			"message": "OauthExpired",
		})
		return
	}
	nonce := oauthCache.Nonce
	op := oauthCache.Op
	action := oauthCache.Action
	verifier := oauthCache.Verifier
	var user *model.User

	code := c.Query("code")
	err, oauthUser := oauthService.Callback(code, verifier, op, nonce)
	if err != nil {
		c.HTML(http.StatusOK, "oauth_fail.html", gin.H{
			"message":     "OauthFailed",
			"sub_message": err.Error(),
		})
		return
	}
	userId := oauthCache.UserId
	openid := oauthUser.OpenId
	if action == service.OauthActionTypeBind {

		//fmt.Println("bind", ty, userData)
		// openid
		utr := oauthService.UserThirdInfo(op, openid)
		if utr.UserId > 0 {
			c.HTML(http.StatusOK, "oauth_fail.html", gin.H{
				"message": "OauthHasBindOtherUser",
			})
			return
		}

		user = service.AllService.UserService.InfoById(userId)
		if user == nil {
			c.HTML(http.StatusOK, "oauth_fail.html", gin.H{
				"message": "ItemNotFound",
			})
			return
		}

		err := oauthService.BindOauthUser(userId, oauthUser, op)
		if err != nil {
			c.HTML(http.StatusOK, "oauth_fail.html", gin.H{
				"message": "BindFail",
			})
			return
		}
		c.HTML(http.StatusOK, "oauth_success.html", gin.H{
			"message": "BindSuccess",
		})
		return

	} else if action == service.OauthActionTypeLogin {

		if userId != 0 {
			c.HTML(http.StatusOK, "oauth_fail.html", gin.H{
				"message": "OauthHasBeenSuccess",
			})
			return
		}
		user = service.AllService.UserService.InfoByOauthId(op, openid)
		if user == nil {
			oauthConfig := oauthService.InfoByOp(op)
			if !*oauthConfig.AutoRegister {
				//c.String(http.StatusInternalServerError, "пјЊ")
				oauthCache.UpdateFromOauthUser(oauthUser)
				c.Redirect(http.StatusFound, "/admin/#/oauth/bind/"+cacheKey)
				return
			}

			err, user = service.AllService.UserService.RegisterByOauth(oauthUser, op)
			if err != nil {
				c.HTML(http.StatusOK, "oauth_fail.html", gin.H{
					"message": err.Error(),
				})
				return
			}
		}
		oauthCache.UserId = user.Id
		oauthService.SetOauthCache(cacheKey, oauthCache, 0)
		// webadminпјЊwebadmin
		if oauthCache.DeviceType == model.LoginLogClientWebAdmin {
			/*service.AllService.UserService.Login(u, &model.LoginLog{
				UserId:   u.Id,
				Client:   "webadmin",
				Uuid:     "", //must be empty
				Ip:       c.ClientIP(),
				Type:     model.LoginLogTypeOauth,
				Platform: oauthService.DeviceOs,
			})*/
			c.Redirect(http.StatusFound, "/admin/#/")
			return
		}
		c.HTML(http.StatusOK, "oauth_success.html", gin.H{
			"message": "OauthSuccess",
		})
		return
	} else {
		c.HTML(http.StatusOK, "oauth_fail.html", gin.H{
			"message": "ParamsError",
		})
		return
	}

}

type MessageParams struct {
	Lang  string `json:"lang" form:"lang"`
	Title string `json:"title" form:"title"`
	Msg   string `json:"msg" form:"msg"`
}

// Message returns localized JavaScript assignments for OAuth/OIDC pages.
// @Tags Oauth
// @Summary Get localized OAuth message script
// @Description Returns JavaScript with localized title and message assignments when the corresponding query parameters are supplied.
// @Produce application/javascript
// @Param lang query string false "Localization language"
// @Param title query string false "Localized title message ID"
// @Param msg query string false "Localized body message ID"
// @Success 200 {string} string "JavaScript response"
// @Router /oauth/msg [get]
// @Router /oidc/msg [get]
func (o *Oauth) Message(c *gin.Context) {
	mp := &MessageParams{}
	if err := c.ShouldBindQuery(mp); err != nil {
		return
	}
	localizer := global.Localizer(mp.Lang)
	// Emit values via encoding/json so that any character in the localized
	// string (apostrophes, line breaks, NUL, control chars, future
	// user-controlled text) ends up as a valid JS string literal. Earlier
	// versions concatenated raw strings into single-quoted literals, which
	// is a JS-injection sink waiting for the first translation containing a
	// quote — or for someone to route caller-controlled text into mp.Title /
	// mp.Msg.
	var sb strings.Builder
	if mp.Title != "" {
		title, err := localizer.LocalizeMessage(&i18n.Message{ID: mp.Title})
		if err == nil {
			if b, mErr := json.Marshal(title); mErr == nil {
				sb.WriteString(";title=")
				sb.Write(b)
				sb.WriteString(";")
			}
		}
	}
	if mp.Msg != "" {
		msg, err := localizer.LocalizeMessage(&i18n.Message{ID: mp.Msg})
		if err == nil {
			if b, mErr := json.Marshal(msg); mErr == nil {
				sb.WriteString("msg=")
				sb.Write(b)
				sb.WriteString(";")
			}
		}
	}

	c.Header("Content-Type", "application/javascript")
	c.String(http.StatusOK, sb.String())
}

package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	apiResp "rustdesk-server/api/http/response/api"
	"rustdesk-server/api/service"
)

type User struct {
}

// Info returns the current Rust client user from the Authorization header.
// @Tags User
// @Summary Get the current Rust client user
// @Description Returns the authenticated user for Rust client API requests.
// @Accept  json
// @Produce  json
// @Success 200 {object} apiResp.UserPayload
// @Failure 500 {object} response.Response
// @Router /user/info [get]
// @Router /currentUser [post]
// @Security BearerAuth
func (u *User) Info(c *gin.Context) {
	user := service.AllService.UserService.CurUser(c)
	up := (&apiResp.UserPayload{}).FromUser(user)
	c.JSON(http.StatusOK, up)
}

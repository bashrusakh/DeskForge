package api

import (
	"github.com/gin-gonic/gin"
	apiReq "rustdesk-server/api/http/request/api"
	"rustdesk-server/api/http/response"
	apiResp "rustdesk-server/api/http/response/api"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"net/http"
)

type Group struct {
}

// Users 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param page query int false ""
// @Param pageSize query int false ""
// @Param status query int false ""
// @Param accessible query string false "accessible"
// @Success 200 {object} response.DataResponse{data=[]apiResp.UserPayload}
// @Failure 500 {object} response.ErrorResponse
// @Router /users [get]
// @Security BearerAuth
func (g *Group) Users(c *gin.Context) {
	q := &apiReq.UserListQuery{}
	err := c.ShouldBindQuery(&q)
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)
	gr := service.AllService.GroupService.InfoById(u.GroupId)
	userList := &model.UserList{}
	if !*u.IsAdmin && gr.Type != model.GroupTypeShare {

		userList.Users = append(userList.Users, u)
		userList.Total = 1
	} else {
		userList = service.AllService.UserService.ListByGroupId(u.GroupId, q.Page, q.PageSize)
	}

	data := make([]*apiResp.UserPayload, 0, len(userList.Users))
	for _, user := range userList.Users {
		up := &apiResp.UserPayload{}
		up.FromUser(user)
		data = append(data, up)
	}
	c.JSON(http.StatusOK, response.DataResponse{
		Total: uint(userList.Total),
		Data:  data,
	})
}

// Peers
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param page query int false ""
// @Param pageSize query int false ""
// @Param status query int false ""
// @Param accessible query string false "accessible"
// @Success 200 {object} response.DataResponse
// @Failure 500 {object} response.Response
// @Router /peers [get]
// @Security BearerAuth
func (g *Group) Peers(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	q := &apiReq.PeerListQuery{}
	err := c.ShouldBindQuery(&q)
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	gr := service.AllService.GroupService.InfoById(u.GroupId)
	users := make([]*model.User, 0, 1)
	if !*u.IsAdmin && gr.Type != model.GroupTypeShare {

		users = append(users, u)
	} else {
		users = service.AllService.UserService.ListIdAndNameByGroupId(u.GroupId)
	}

	namesById := make(map[uint]string, len(users))
	userIds := make([]uint, 0, len(users))
	for _, user := range users {
		namesById[user.Id] = user.Username
		userIds = append(userIds, user.Id)
	}
	dGroupNameById := make(map[uint]string)
	allGroup := service.AllService.GroupService.DeviceGroupList(1, 999, nil)
	for _, group := range allGroup.DeviceGroups {
		dGroupNameById[group.Id] = group.Name
	}
	peerList := service.AllService.PeerService.ListByUserIds(userIds, q.Page, q.PageSize)
	data := make([]*apiResp.GroupPeerPayload, 0, len(peerList.Peers))
	for _, peer := range peerList.Peers {
		uname, ok := namesById[peer.UserId]
		if !ok {
			uname = ""
		}
		dGroupName, ok2 := dGroupNameById[peer.GroupId]
		if !ok2 {
			dGroupName = ""
		}
		pp := &apiResp.GroupPeerPayload{}
		pp.FromPeer(peer, uname, dGroupName)
		data = append(data, pp)

	}
	c.JSON(http.StatusOK, response.DataResponse{
		Total: uint(peerList.Total),
		Data:  data,
	})
}

// Device
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param page query int false ""
// @Param pageSize query int false ""
// @Param status query int false ""
// @Param accessible query string false "accessible"
// @Success 200 {object} response.DataResponse
// @Failure 500 {object} response.Response
// @Router /device-group/accessible [get]
// @Security BearerAuth
func (g *Group) Device(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	if !service.AllService.UserService.IsAdmin(u) {
		response.Error(c, "Permission denied")
		return
	}
	allGroup := service.AllService.GroupService.DeviceGroupList(1, 999, nil)

	c.JSON(http.StatusOK, response.DataResponse{
		Total: 0,
		Data:  allGroup.DeviceGroups,
	})
}

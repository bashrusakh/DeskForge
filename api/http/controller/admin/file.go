package admin

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/lib/upload"
	"os"
	"time"
)

type File struct {
}

// OssToken 
// @Tags 
// @Summary ossToken
// @Description ossToken
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/file/oss_token [get]
// @Security token
func (f *File) OssToken(c *gin.Context) {
	token := global.Oss.GetPolicyToken("")
	response.Success(c, token)
}

type FileBack struct {
	upload.CallbackBaseForm
	Url string `json:"url"`
}

// Notify 
func (f *File) Notify(c *gin.Context) {

	res := global.Oss.Verify(c.Request)
	if !res {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}
	fm := &FileBack{}
	if err := c.ShouldBind(fm); err != nil {
		fmt.Println(err)
	}
	fm.Url = global.Config.Oss.Host + "/" + fm.Filename
	response.Success(c, fm)

}

// Upload 
// @Tags 
// @Summary 
// @Description 
// @Accept  multipart/form-data
// @Produce  json
// @Param file formData file true ""
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/file/upload [post]
// @Security token
func (f *File) Upload(c *gin.Context) {
	file, _ := c.FormFile("file")
	if file == nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	// PNG-only validation (security)
	if len(file.Header) > 0 {
		ct := file.Header.Get("Content-Type")
		if ct != "" && ct != "image/png" {
			response.Fail(c, 101, "only PNG allowed")
			return
		}
	}
	timePath := time.Now().Format("20060102") + "/"
	webPath := "/upload/" + timePath
	// write under public/upload so g.StaticFS("/upload", ...) can serve it
	path := global.Config.Gin.ResourcesPath + "/public" + webPath
	dst := path + file.Filename
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}

	err = c.SaveUploadedFile(file, dst)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	// web
	response.Success(c, gin.H{
		"url": webPath + file.Filename,
	})
}

package admin

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"rustdesk-server/api/global"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/lib/upload"
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
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+": file required")
		return
	}
	// file size limit (5 MB) — enforce on actual bytes, not declared Content-Length
	const maxSize = 5 * 1024 * 1024
	src, err := file.Open()
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	defer src.Close()

	limited := io.LimitReader(src, maxSize+1)
	header := make([]byte, 8)
	if _, err := io.ReadFull(limited, header); err != nil {
		response.Fail(c, 101, "cannot read file header")
		return
	}
	pngSig := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	for i := range pngSig {
		if header[i] != pngSig[i] {
			response.Fail(c, 101, "only PNG files allowed")
			return
		}
	}

	timePath := time.Now().Format("20060102") + "/"
	webPath := "/upload/" + timePath
	path := global.Config.Gin.ResourcesPath + "/public" + webPath
	safeName := filepath.Base(file.Filename)
	if safeName == "." || safeName == ".." || safeName == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	ext := filepath.Ext(safeName)
	stem := safeName[:len(safeName)-len(ext)]
	uniqueName := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), stem, ext)
	dst := path + uniqueName
	err = os.MkdirAll(path, 0755)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}

	// write to disk enforcing size limit on actual bytes
	out, err := os.Create(dst)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	defer out.Close()
	if _, err := out.Write(header); err != nil {
		out.Close()
		os.Remove(dst)
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	written, err := io.CopyN(out, limited, maxSize+1)
	if err != nil && err != io.EOF {
		os.Remove(dst)
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	if written+8 > maxSize {
		os.Remove(dst)
		response.Fail(c, 101, "file too large (max 5 MB)")
		return
	}
	// Close explicitly before responding so a Close error (flush / disk-full) is
	// surfaced as a failure instead of being swallowed by defer after Success.
	if err := out.Close(); err != nil {
		os.Remove(dst)
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	// web
	response.Success(c, gin.H{
		"url": webPath + uniqueName,
	})
}

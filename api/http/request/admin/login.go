package admin

type Login struct {
	Username  string `json:"username" validate:"required" label:""`
	Password  string `json:"password,omitempty" validate:"required" label:""`
	Platform  string `json:"platform" label:""`
	Captcha   string `json:"captcha,omitempty" label:""`
	CaptchaId string `json:"captcha_id,omitempty"`
}

type LoginLogQuery struct {
	UserId int `form:"user_id"`
	IsMy   int `form:"is_my"`
	PageQuery
}
type LoginTokenQuery struct {
	UserId int `form:"user_id"`
	PageQuery
}

type LoginLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}

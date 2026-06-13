package api

/*
*

	message LoginRequest {
	  string username = 1;
	  bytes password = 2;
	  string my_id = 4;
	  string my_name = 5;
	  OptionMessage option = 6;
	  oneof union {
	    FileTransfer file_transfer = 7;
	    PortForward port_forward = 8;
	  }
	  bool video_ack_required = 9;
	  uint64 session_id = 10;
	  string version = 11;
	  OSLogin os_login = 12;
	  string my_platform = 13;
	  bytes hwid = 14;
	}
*/

type DeviceInfoInLogin struct {
	Name string `json:"name" label:"name"`
	Os   string `json:"os" label:"os"`
	Type string `json:"type" label:"type"`
}

type LoginForm struct {
	AutoLogin  bool              `json:"autoLogin" label:""`
	DeviceInfo DeviceInfoInLogin `json:"deviceInfo" label:""`
	Id         string            `json:"id"  label:"id"`
	Type       string            `json:"type"  label:"type"`
	Uuid       string            `json:"uuid"  label:"uuid"`
	Username   string            `json:"username" validate:"required,gte=2,lte=32" label:""`
	Password   string            `json:"password,omitempty" validate:"gte=4,lte=32" label:""`
}

type UserListQuery struct {
	Page       uint   `json:"page" form:"page" validate:"required" label:""`
	PageSize   uint   `json:"pageSize" form:"pageSize" validate:"required" label:""`
	Status     int    `json:"status" form:"status" label:""`
	Accessible string `json:"accessible" form:"accessible"`
}

type PeerListQuery struct {
	Page       uint   `json:"page" form:"page" validate:"required" label:""`
	PageSize   uint   `json:"pageSize" form:"pageSize" validate:"required" label:""`
	Status     int    `json:"status" form:"status" label:""`
	Accessible string `json:"accessible" form:"accessible"`
}

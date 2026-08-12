package model

type UserToken struct {
	IdModel
	UserId     uint   `json:"user_id" gorm:"default:0;not null;index"`
	DeviceUuid string `json:"device_uuid" gorm:"default:'';omitempty;"`
	DeviceId   string `json:"device_id" gorm:"default:'';omitempty;"`
	Token      string `json:"-" gorm:"default:'';not null;index"`
	ExpiredAt  int64  `json:"expired_at" gorm:"default:0;not null;"`
	TimeModel
}

type UserTokenList struct {
	UserTokens []UserToken `json:"list"`
	Pagination
}

// UserTokenSafe is the metadata exposed by token-management responses. Token
// retains the existing UI field as a server-side mask; the raw bearer secret
// never crosses the response boundary.
type UserTokenSafe struct {
	Id         uint   `json:"id"`
	UserId     uint   `json:"user_id"`
	DeviceUuid string `json:"device_uuid,omitempty"`
	DeviceId   string `json:"device_id,omitempty"`
	Token      string `json:"token"`
	ExpiredAt  int64  `json:"expired_at"`
	TimeModel
}

type UserTokenSafeList struct {
	UserTokens []UserTokenSafe `json:"list"`
	Pagination
}

// Safe returns token metadata without exposing the bearer secret.
func (t *UserToken) Safe() UserTokenSafe {
	if t == nil {
		return UserTokenSafe{}
	}
	return UserTokenSafe{
		Id:         t.Id,
		UserId:     t.UserId,
		DeviceUuid: t.DeviceUuid,
		DeviceId:   t.DeviceId,
		Token:      MaskUserToken(t.Token),
		ExpiredAt:  t.ExpiredAt,
		TimeModel:  t.TimeModel,
	}
}

// Safe returns token metadata for list responses. A masked token preserves the
// existing administrative display contract without disclosing its secret.
func (l *UserTokenList) Safe() *UserTokenSafeList {
	if l == nil {
		return nil
	}
	view := &UserTokenSafeList{Pagination: l.Pagination}
	if l.UserTokens != nil {
		view.UserTokens = make([]UserTokenSafe, 0, len(l.UserTokens))
		for i := range l.UserTokens {
			view.UserTokens = append(view.UserTokens, l.UserTokens[i].Safe())
		}
	}
	return view
}

// MaskUserToken returns the same short display form used by the admin UI.
func MaskUserToken(token string) string {
	if len(token) <= 8 {
		return ""
	}
	return token[:4] + "****" + token[len(token)-4:]
}

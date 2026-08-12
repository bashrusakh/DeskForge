package model

type ShareRecord struct {
	IdModel
	UserId       uint   `json:"user_id" gorm:"default:0;not null;index"`
	PeerId       string `json:"peer_id" gorm:"default:'';not null;index"`
	ShareToken   string `json:"-" gorm:"default:'';not null;index"`
	PasswordType string `json:"password_type" gorm:"default:'';not null;"`
	Password     string `json:"-" gorm:"default:'';not null;"`
	Expire       int64  `json:"expire" gorm:"default:0;not null;"`
	TimeModel
}

// ShareRecordList
type ShareRecordList struct {
	ShareRecords []*ShareRecord `json:"list,omitempty"`
	Pagination
}

// ShareRecordSafe is the response view for administrative and personal share
// record lists. ShareToken, PasswordType, and Password remain available on
// ShareRecord for internal reads and write/input handling, but never cross
// these response boundaries.
type ShareRecordSafe struct {
	Id     uint   `json:"id"`
	UserId uint   `json:"user_id"`
	PeerId string `json:"peer_id"`
	Expire int64  `json:"expire"`
	TimeModel
}

// ShareRecordSafeList is the paginated response view for share records.
type ShareRecordSafeList struct {
	ShareRecords []*ShareRecordSafe `json:"list,omitempty"`
	Pagination
}

// Safe returns a share record without its bearer token or credentials.
func (r *ShareRecord) Safe() *ShareRecordSafe {
	if r == nil {
		return nil
	}
	return &ShareRecordSafe{
		Id:        r.Id,
		UserId:    r.UserId,
		PeerId:    r.PeerId,
		Expire:    r.Expire,
		TimeModel: r.TimeModel,
	}
}

// Safe returns a paginated share-record response without bearer tokens or
// credentials.
func (l *ShareRecordList) Safe() *ShareRecordSafeList {
	if l == nil {
		return nil
	}
	view := &ShareRecordSafeList{Pagination: l.Pagination}
	if l.ShareRecords != nil {
		view.ShareRecords = make([]*ShareRecordSafe, 0, len(l.ShareRecords))
		for _, record := range l.ShareRecords {
			view.ShareRecords = append(view.ShareRecords, record.Safe())
		}
	}
	return view
}

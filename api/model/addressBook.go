package model

import "rustdesk-server/api/model/custom_types"

// final String id;
// String hash; // personal ab hash password
// String password; // shared ab password
// String username; // pc username
// String hostname;
// String platform;
// String alias;
// List<dynamic> tags;
// bool forceAlwaysRelay = false;
// String rdpPort;
// String rdpUsername;
// bool online = false;
// String loginName; //login username
// bool? sameServer;

// AddressBook Personal
type AddressBook struct {
	RowId            uint                   `gorm:"primaryKey" json:"row_id"`
	Id               string                 `json:"id" gorm:"default:0;not null;index"`
	Username         string                 `json:"username" gorm:"default:'';not null;"`
	Password         string                 `json:"-" gorm:"default:'';not null;"`
	Hostname         string                 `json:"hostname" gorm:"default:'';not null;"`
	Alias            string                 `json:"alias" gorm:"default:'';not null;"`
	Platform         string                 `json:"platform" gorm:"default:'';not null;"`
	Tags             custom_types.AutoJson  `json:"tags" gorm:"not null;" swaggertype:"array,string"`
	Hash             string                 `json:"-" gorm:"default:'';not null;"`
	UserId           uint                   `json:"user_id" gorm:"default:0;not null;index"`
	ForceAlwaysRelay bool                   `json:"forceAlwaysRelay" gorm:"default:0;not null;"`
	RdpPort          string                 `json:"rdpPort" gorm:"default:'';not null;"`
	RdpUsername      string                 `json:"rdpUsername" gorm:"default:'';not null;"`
	Online           bool                   `json:"online" gorm:"default:0;not null;"`
	LoginName        string                 `json:"loginName" gorm:"default:'';not null;"`
	SameServer       bool                   `json:"sameServer" gorm:"default:0;not null;"`
	CollectionId     uint                   `json:"collection_id" gorm:"default:0;not null;index"`
	Collection       *AddressBookCollection `json:"collection,omitempty"`
	TimeModel
}

type AddressBookList struct {
	AddressBooks []*AddressBook `json:"list"`
	Pagination
}

// AddressBookSafe is the normal response view. Password and Hash are stored
// credentials used by typed write/protocol flows and are intentionally absent
// from this DTO; all unrelated address-book metadata remains available.
type AddressBookSafe struct {
	RowId            uint                   `json:"row_id"`
	Id               string                 `json:"id"`
	Username         string                 `json:"username"`
	Hostname         string                 `json:"hostname"`
	Alias            string                 `json:"alias"`
	Platform         string                 `json:"platform"`
	Tags             custom_types.AutoJson  `json:"tags" gorm:"not null;" swaggertype:"array,string"`
	UserId           uint                   `json:"user_id"`
	ForceAlwaysRelay bool                   `json:"forceAlwaysRelay"`
	RdpPort          string                 `json:"rdpPort"`
	RdpUsername      string                 `json:"rdpUsername"`
	Online           bool                   `json:"online"`
	LoginName        string                 `json:"loginName"`
	SameServer       bool                   `json:"sameServer"`
	CollectionId     uint                   `json:"collection_id"`
	Collection       *AddressBookCollection `json:"collection,omitempty"`
	TimeModel
}

type AddressBookSafeList struct {
	AddressBooks []*AddressBookSafe `json:"list"`
	Pagination
}

// Safe returns the response-only address-book view without stored credentials.
func (a *AddressBook) Safe() *AddressBookSafe {
	if a == nil {
		return nil
	}
	return &AddressBookSafe{
		RowId:            a.RowId,
		Id:               a.Id,
		Username:         a.Username,
		Hostname:         a.Hostname,
		Alias:            a.Alias,
		Platform:         a.Platform,
		Tags:             a.Tags,
		UserId:           a.UserId,
		ForceAlwaysRelay: a.ForceAlwaysRelay,
		RdpPort:          a.RdpPort,
		RdpUsername:      a.RdpUsername,
		Online:           a.Online,
		LoginName:        a.LoginName,
		SameServer:       a.SameServer,
		CollectionId:     a.CollectionId,
		Collection:       a.Collection,
		TimeModel:        a.TimeModel,
	}
}

// Safe returns a paginated response-only address-book view.
func (l *AddressBookList) Safe() *AddressBookSafeList {
	if l == nil {
		return nil
	}
	view := &AddressBookSafeList{Pagination: l.Pagination}
	if l.AddressBooks != nil {
		view.AddressBooks = make([]*AddressBookSafe, 0, len(l.AddressBooks))
		for _, addressBook := range l.AddressBooks {
			view.AddressBooks = append(view.AddressBooks, addressBook.Safe())
		}
	}
	return view
}

type AddressBookCollection struct {
	IdModel
	UserId uint   `json:"user_id" gorm:"default:0;not null;index"`
	Name   string `json:"name" gorm:"default:'';not null;" validate:"required"`
	TimeModel
}
type AddressBookCollectionList struct {
	AddressBookCollection []*AddressBookCollection `json:"list"`
	Pagination
}
type AddressBookCollectionRule struct {
	IdModel
	UserId       uint `json:"user_id" gorm:"default:0;not null;"`
	CollectionId uint `json:"collection_id" gorm:"default:0;not null;index" validate:"required"`
	Rule         int  `json:"rule" gorm:"default:0;not null;" validate:"required,gte=1,lte=3"` // 0:  1:  2:   3:
	Type         int  `json:"type" gorm:"default:1;not null;" validate:"required,gte=1,lte=2"` // 1:  2:
	ToId         uint `json:"to_id" gorm:"default:0;not null;" validate:"required,gt=0"`
	TimeModel
}
type AddressBookCollectionRuleList struct {
	AddressBookCollectionRule []*AddressBookCollectionRule `json:"list"`
	Pagination
}

const (
	ShareAddressBookRuleTypePersonal = 1
	ShareAddressBookRuleTypeGroup    = 2
)
const (
	ShareAddressBookRuleRuleRead        = 1
	ShareAddressBookRuleRuleReadWrite   = 2
	ShareAddressBookRuleRuleFullControl = 3
)

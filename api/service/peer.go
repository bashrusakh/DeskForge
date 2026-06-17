package service

import (
	"errors"

	"rustdesk-server/api/model"
	"gorm.io/gorm"
)

type PeerService struct {
}

// FindById id
func (ps *PeerService) FindById(id string) *model.Peer {
	p := &model.Peer{}
	DB.Where("id = ?", id).First(p)
	return p
}
func (ps *PeerService) FindByUuid(uuid string) *model.Peer {
	p := &model.Peer{}
	DB.Where("uuid = ?", uuid).First(p)
	return p
}
func (ps *PeerService) InfoByRowId(id uint) *model.Peer {
	p := &model.Peer{}
	DB.Where("row_id = ?", id).First(p)
	return p
}

// FindByUserIdAndUuid iduuidpeer
func (ps *PeerService) FindByUserIdAndUuid(uuid string, userId uint) *model.Peer {
	p := &model.Peer{}
	DB.Where("uuid = ? and user_id = ?", uuid, userId).First(p)
	return p
}

// UuidBindUserId id
func (ps *PeerService) UuidBindUserId(deviceId string, uuid string, userId uint) {
	peer := ps.FindByUuid(uuid)

	if peer.RowId > 0 {
		peer.UserId = userId
		ps.Update(peer)
	} else {

		/*if deviceId != "" {
			DB.Create(&model.Peer{
				Id:     deviceId,
				Uuid:   uuid,
				UserId: userId,
			})
		}*/
	}
}

// UuidUnbindUserId id, 
func (ps *PeerService) UuidUnbindUserId(uuid string, userId uint) {
	peer := ps.FindByUserIdAndUuid(uuid, userId)
	if peer.RowId > 0 {
		DB.Model(peer).Update("user_id", 0)
	}
}

// EraseUserId id, 
func (ps *PeerService) EraseUserId(userId uint) error {
	return DB.Model(&model.Peer{}).Where("user_id = ?", userId).Update("user_id", 0).Error
}

// ListByUserIds id
func (ps *PeerService) ListByUserIds(userIds []uint, page, pageSize uint) (res *model.PeerList) {
	res = &model.PeerList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Peer{})
	tx.Where("user_id in (?)", userIds)
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Peers)
	return
}

func (ps *PeerService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.PeerList) {
	res = &model.PeerList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Peer{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Peers)
	return
}

// ListFilterByUserId idPeer
func (ps *PeerService) ListFilterByUserId(page, pageSize uint, where func(tx *gorm.DB), userId uint) (res *model.PeerList) {
	userWhere := func(tx *gorm.DB) {
		tx.Where("user_id = ?", userId)
		// пјЊ
		if where != nil {
			where(tx)
		}
	}
	return ps.List(page, pageSize, userWhere)
}

// Create 
func (ps *PeerService) Create(u *model.Peer) error {
	res := DB.Create(u).Error
	return res
}

// Delete , token
func (ps *PeerService) Delete(u *model.Peer) error {
	uuid := u.Uuid
	err := DB.Delete(u).Error
	if err != nil {
		return err
	}
	// token
	return AllService.UserService.FlushTokenByUuid(uuid)
}

// GetUuidListByIDs idsuuid
func (ps *PeerService) GetUuidListByIDs(ids []uint) ([]string, error) {
	var uuids []string
	err := DB.Model(&model.Peer{}).
		Where("row_id in (?)", ids).
		Pluck("uuid", &uuids).Error
	var newUuids []string
	for _, uuid := range uuids {
		if uuid != "" {
			newUuids = append(newUuids, uuid)
		}
	}
	return newUuids, err
}

// GetUuidListByIDsAndOwner  idsuuid scoped to owner
func (ps *PeerService) GetUuidListByIDsAndOwner(ids []uint, userId uint) ([]string, error) {
	var uuids []string
	err := DB.Model(&model.Peer{}).
		Where("row_id in (?) AND user_id = ?", ids, userId).
		Pluck("uuid", &uuids).Error
	var newUuids []string
	for _, uuid := range uuids {
		if uuid != "" {
			newUuids = append(newUuids, uuid)
		}
	}
	return newUuids, err
}

// BatchDelete , token
func (ps *PeerService) BatchDelete(ids []uint) error {
	uuids, err := ps.GetUuidListByIDs(ids)
	if err != nil {
		return err
	}
	err = DB.Where("row_id in (?)", ids).Delete(&model.Peer{}).Error
	if err != nil {
		return err
	}
	// token
	return AllService.UserService.FlushTokenByUuids(uuids)
}

// DeleteWithOwner  peer owned by user
func (ps *PeerService) DeleteWithOwner(rowId uint, userId uint) error {
	tx := DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	var peer model.Peer
	if err := tx.Where("row_id = ? AND user_id = ?", rowId, userId).First(&peer).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // idempotent: already gone or not owned
		}
		return err
	}
	if err := tx.Delete(&peer).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	if peer.Uuid != "" {
		return AllService.UserService.FlushTokenByUuid(peer.Uuid)
	}
	return nil
}

// BatchDeleteByOwner  peers owned by user
func (ps *PeerService) BatchDeleteByOwner(rowIds []uint, userId uint) error {
	if len(rowIds) == 0 {
		return nil
	}
	var uuids []string
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Peer{}).
			Where("row_id in (?) AND user_id = ?", rowIds, userId).
			Pluck("uuid", &uuids).Error; err != nil {
			return err
		}
		result := tx.Where("row_id in (?) AND user_id = ?", rowIds, userId).Delete(&model.Peer{})
		if result.Error != nil {
			return result.Error
		}
		return nil
	})
	if err != nil {
		return err
	}
	return AllService.UserService.FlushTokenByUuids(uuids)
}

// Update 
func (ps *PeerService) Update(u *model.Peer) error {
	return DB.Model(u).Updates(u).Error
}

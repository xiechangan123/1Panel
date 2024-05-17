package service

import (
	"github.com/1Panel-dev/1Panel/backend/app/dto"
	"github.com/1Panel-dev/1Panel/backend/app/model"
	"github.com/1Panel-dev/1Panel/backend/constant"
	"github.com/1Panel-dev/1Panel/backend/utils/encrypt"
	"github.com/1Panel-dev/1Panel/backend/utils/toolbox"
	"github.com/jinzhu/copier"
	"github.com/pkg/errors"
)

type FtpService struct{}

type IFtpService interface {
	SearchWithPage(search dto.SearchWithPage) (int64, interface{}, error)
	Create(req dto.FtpCreate) error
	Delete(req dto.BatchDeleteReq) error
	Update(req dto.FtpUpdate) error
	Sync() error
}

func NewIFtpService() IFtpService {
	return &FtpService{}
}

func (f *FtpService) SearchWithPage(req dto.SearchWithPage) (int64, interface{}, error) {
	total, lists, err := ftpRepo.Page(req.Page, req.PageSize, ftpRepo.WithByUser(req.Info), commonRepo.WithOrderBy("created_at desc"))
	if err != nil {
		return 0, nil, err
	}
	var users []dto.FtpInfo
	for _, user := range lists {
		var item dto.FtpInfo
		if err := copier.Copy(&item, &user); err != nil {
			return 0, nil, errors.WithMessage(constant.ErrStructTransform, err.Error())
		}
		item.Password, _ = encrypt.StringDecrypt(item.Password)
		users = append(users, item)
	}
	return total, users, err
}

func (f *FtpService) Sync() error {
	client, err := toolbox.NewFtpClient()
	if err != nil {
		return err
	}
	lists, err := client.LoadList()
	if err != nil {
		return nil
	}
	listsInDB, err := ftpRepo.GetList()
	if err != nil {
		return err
	}
	sameData := make(map[string]struct{})
	for _, item := range lists {
		for _, itemInDB := range listsInDB {
			if item.User == itemInDB.User {
				sameData[item.User] = struct{}{}
				if item.Path != itemInDB.Path {
					_ = ftpRepo.Update(itemInDB.ID, map[string]interface{}{"path": item.Path, "status": constant.StatusDisable})
				}
				break
			}
		}
	}
	for _, item := range lists {
		if _, ok := sameData[item.User]; !ok {
			_ = ftpRepo.Create(&model.Ftp{User: item.User, Path: item.Path, Status: constant.StatusDisable})
		}
	}
	for _, item := range listsInDB {
		if _, ok := sameData[item.User]; !ok {
			_ = ftpRepo.Update(item.ID, map[string]interface{}{"status": "deleted"})
		}
	}
	return nil
}

func (f *FtpService) Create(req dto.FtpCreate) error {
	pass, err := encrypt.StringEncrypt(req.Password)
	if err != nil {
		return err
	}
	userInDB, _ := ftpRepo.Get(hostRepo.WithByUser(req.User))
	if userInDB.ID != 0 {
		return constant.ErrRecordExist
	}
	client, err := toolbox.NewFtpClient()
	if err != nil {
		return err
	}
	if err := client.UserAdd(req.User, req.Password, req.Path); err != nil {
		return err
	}
	var ftp model.Ftp
	if err := copier.Copy(&ftp, &req); err != nil {
		return errors.WithMessage(constant.ErrStructTransform, err.Error())
	}
	ftp.Status = constant.StatusEnable
	ftp.Password = pass
	if err := ftpRepo.Create(&ftp); err != nil {
		return err
	}
	return nil
}

func (f *FtpService) Delete(req dto.BatchDeleteReq) error {
	client, err := toolbox.NewFtpClient()
	if err != nil {
		return err
	}
	for _, id := range req.Ids {
		ftpItem, err := ftpRepo.Get(commonRepo.WithByID(id))
		if err != nil {
			return err
		}
		_ = client.UserDel(ftpItem.User)
		_ = ftpRepo.Delete(commonRepo.WithByID(id))
	}
	return nil
}

func (f *FtpService) Update(req dto.FtpUpdate) error {
	pass, err := encrypt.StringEncrypt(req.Password)
	if err != nil {
		return err
	}
	ftpItem, _ := ftpRepo.Get(commonRepo.WithByID(req.ID))
	if ftpItem.ID == 0 {
		return constant.ErrRecordNotFound
	}
	passItem, err := encrypt.StringDecrypt(ftpItem.Password)
	if err != nil {
		return err
	}

	client, err := toolbox.NewFtpClient()
	if err != nil {
		return err
	}
	needReload := false
	updates := make(map[string]interface{})
	if req.Password != passItem {
		if err := client.SetPasswd(ftpItem.User, req.Password); err != nil {
			return err
		}
		updates["password"] = pass
		needReload = true
	}
	if req.Status != ftpItem.Status {
		if err := client.SetStatus(ftpItem.User, req.Status); err != nil {
			return err
		}
		updates["status"] = req.Status
		needReload = true
	}
	if req.Path != ftpItem.Path {
		if err := client.SetPath(ftpItem.User, req.Path); err != nil {
			return err
		}
		updates["path"] = req.Path
		needReload = true
	}
	if req.Description != ftpItem.Description {
		updates["description"] = req.Description
	}
	if needReload {
		_ = client.Reload()
	}
	if len(updates) != 0 {
		return ftpRepo.Update(ftpItem.ID, updates)
	}
	return nil
}
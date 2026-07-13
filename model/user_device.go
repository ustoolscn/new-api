package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const DeviceFingerprintHeader = "X-Device-Fingerprint"

type UserDevice struct {
	Id                int    `json:"id"`
	UserId            int    `json:"user_id" gorm:"index;uniqueIndex:idx_user_device"`
	FingerprintHash   string `json:"fingerprint_hash" gorm:"type:varchar(128);index;uniqueIndex:idx_user_device"`
	FirstIp           string `json:"first_ip" gorm:"type:varchar(64);index"`
	UserAgent         string `json:"user_agent" gorm:"type:varchar(512)"`
	Source            string `json:"source" gorm:"type:varchar(32)"`
	Email             string `json:"email" gorm:"type:varchar(50);index"`
	Phone             string `json:"phone" gorm:"type:varchar(20);index"`
	FirstSeenAt       int64  `json:"first_seen_at" gorm:"index"`
	LastSeenAt        int64  `json:"last_seen_at" gorm:"index"`
	Banned            bool   `json:"banned" gorm:"index"`
	BannedAt          int64  `json:"banned_at"`
	Username          string `json:"username" gorm:"-:all"`
	DeviceUserCount   int64  `json:"device_user_count" gorm:"->;-:migration"`
	DeviceRecordCount int64  `json:"device_record_count" gorm:"->;-:migration"`
	IpUserCount       int64  `json:"ip_user_count" gorm:"->;-:migration"`
	IpRecordCount     int64  `json:"ip_record_count" gorm:"->;-:migration"`
}

func HashDeviceFingerprint(fingerprint string) string {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return ""
	}
	return common.GenerateHMAC(fingerprint)
}

func IsDeviceFingerprintBanned(fingerprint string) (bool, error) {
	hash := HashDeviceFingerprint(fingerprint)
	if hash == "" {
		return false, nil
	}
	var count int64
	err := DB.Model(&UserDevice{}).Where("fingerprint_hash = ? AND banned = ?", hash, true).Count(&count).Error
	return count > 0, err
}

func RecordUserDevice(user *User, fingerprint, ip, userAgent, source string) error {
	hash := HashDeviceFingerprint(fingerprint)
	if hash == "" {
		return nil
	}
	now := common.GetTimestamp()
	device := UserDevice{UserId: user.Id, FingerprintHash: hash, FirstIp: ip, UserAgent: userAgent, Source: source, Email: user.Email, Phone: user.Phone, FirstSeenAt: now, LastSeenAt: now}
	return DB.Where("user_id = ? AND fingerprint_hash = ?", user.Id, hash).
		Assign(map[string]any{"last_seen_at": now, "user_agent": userAgent, "source": source, "email": user.Email, "phone": user.Phone}).FirstOrCreate(&device).Error
}

func GetUserDevices(pageInfo *common.PageInfo, keyword string, banned *bool, duplicate string) ([]UserDevice, int64, error) {
	query := DB.Table("user_devices").Select(`user_devices.*, users.username,
		(SELECT COUNT(DISTINCT device_matches.user_id) FROM user_devices AS device_matches WHERE device_matches.fingerprint_hash = user_devices.fingerprint_hash) AS device_user_count,
		(SELECT COUNT(*) FROM user_devices AS device_matches WHERE device_matches.fingerprint_hash = user_devices.fingerprint_hash) AS device_record_count,
		(SELECT COUNT(DISTINCT ip_matches.user_id) FROM user_devices AS ip_matches WHERE ip_matches.first_ip = user_devices.first_ip AND ip_matches.first_ip <> '') AS ip_user_count,
		(SELECT COUNT(*) FROM user_devices AS ip_matches WHERE ip_matches.first_ip = user_devices.first_ip AND ip_matches.first_ip <> '') AS ip_record_count`).
		Joins("LEFT JOIN users ON users.id = user_devices.user_id")
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("user_devices.fingerprint_hash LIKE ? OR users.username LIKE ? OR user_devices.email LIKE ? OR user_devices.phone LIKE ? OR user_devices.first_ip LIKE ?", like, like, like, like, like)
	}
	if banned != nil {
		query = query.Where("user_devices.banned = ?", *banned)
	}
	if duplicate == "device" {
		query = query.Where("user_devices.fingerprint_hash IN (?)", DB.Model(&UserDevice{}).Select("fingerprint_hash").Group("fingerprint_hash").Having("COUNT(DISTINCT user_id) > 1"))
	} else if duplicate == "ip" {
		query = query.Where("user_devices.first_ip <> '' AND user_devices.first_ip IN (?)", DB.Model(&UserDevice{}).Select("first_ip").Where("first_ip <> ''").Group("first_ip").Having("COUNT(DISTINCT user_id) > 1"))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var devices []UserDevice
	err := query.Order("user_devices.id DESC").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Scan(&devices).Error
	return devices, total, err
}

func SetUserDeviceBan(id int, banned bool) ([]int, error) {
	var userIds []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var device UserDevice
		if err := tx.First(&device, id).Error; err != nil {
			return err
		}
		bannedAt := int64(0)
		if banned {
			bannedAt = common.GetTimestamp()
		}
		if err := tx.Model(&UserDevice{}).Where("fingerprint_hash = ?", device.FingerprintHash).Updates(map[string]any{"banned": banned, "banned_at": bannedAt}).Error; err != nil {
			return err
		}
		if !banned {
			return nil
		}
		if err := tx.Model(&UserDevice{}).Where("fingerprint_hash = ?", device.FingerprintHash).Distinct().Pluck("user_id", &userIds).Error; err != nil {
			return err
		}
		if len(userIds) == 0 {
			return nil
		}
		if err := tx.Model(&User{}).Where("id IN ?", userIds).Update("status", common.UserStatusDisabled).Error; err != nil {
			return err
		}
		return tx.Model(&Token{}).Where("user_id IN ?", userIds).Update("status", common.TokenStatusDisabled).Error
	})
	if err != nil {
		return nil, err
	}
	for _, userId := range userIds {
		_ = InvalidateUserCache(userId)
		_ = InvalidateUserTokensCache(userId)
	}
	return userIds, nil
}

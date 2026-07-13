package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserDeviceTestDB(t *testing.T) {
	t.Helper()
	original := DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &UserDevice{}))
	DB = db
	t.Cleanup(func() { DB = original })
}

func TestRecordUserDeviceStoresHashedRegistrationMetadata(t *testing.T) {
	setupUserDeviceTestDB(t)
	user := User{Username: "registered-device-user", Password: "password", Status: common.UserStatusEnabled, Email: "device@example.com", Phone: "13800138000", AffCode: "registered-device"}
	require.NoError(t, DB.Create(&user).Error)

	require.NoError(t, RecordUserDevice(&user, "raw-device-fingerprint", "203.0.113.8", "Example Browser", "password"))

	var device UserDevice
	require.NoError(t, DB.First(&device, "user_id = ?", user.Id).Error)
	assert.Equal(t, HashDeviceFingerprint("raw-device-fingerprint"), device.FingerprintHash)
	assert.NotEqual(t, "raw-device-fingerprint", device.FingerprintHash)
	assert.Equal(t, "203.0.113.8", device.FirstIp)
	assert.Equal(t, "Example Browser", device.UserAgent)
	assert.Equal(t, user.Email, device.Email)
	assert.Equal(t, user.Phone, device.Phone)
	assert.Equal(t, "password", device.Source)
}

func TestRecordUserDeviceAssociatesLoginAndRefreshesMetadata(t *testing.T) {
	setupUserDeviceTestDB(t)
	user := User{Username: "existing-device-user", Password: "password", Status: common.UserStatusEnabled, Email: "old@example.com", Phone: "13800138000", AffCode: "existing-device"}
	require.NoError(t, DB.Create(&user).Error)
	existing := UserDevice{
		UserId:          user.Id,
		FingerprintHash: HashDeviceFingerprint("existing-fingerprint"),
		FirstIp:         "192.0.2.1",
		UserAgent:       "Old Browser",
		Source:          "password",
		Email:           user.Email,
		Phone:           user.Phone,
		FirstSeenAt:     1,
		LastSeenAt:      1,
	}
	require.NoError(t, DB.Create(&existing).Error)

	user.Email = "new@example.com"
	user.Phone = "13900139000"
	require.NoError(t, RecordUserDevice(&user, "existing-fingerprint", "198.51.100.9", "New Browser", "login:password"))

	var devices []UserDevice
	require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&devices).Error)
	require.Len(t, devices, 1)
	device := devices[0]
	assert.Equal(t, int64(1), device.FirstSeenAt)
	assert.Equal(t, "192.0.2.1", device.FirstIp)
	assert.Greater(t, device.LastSeenAt, int64(1))
	assert.Equal(t, "New Browser", device.UserAgent)
	assert.Equal(t, "login:password", device.Source)
	assert.Equal(t, user.Email, device.Email)
	assert.Equal(t, user.Phone, device.Phone)
}

func TestSetUserDeviceBanDisablesEveryAssociatedUserAndToken(t *testing.T) {
	setupUserDeviceTestDB(t)
	users := []User{{Username: "device-user-a", Password: "password", Status: common.UserStatusEnabled, AffCode: "device-a"}, {Username: "device-user-b", Password: "password", Status: common.UserStatusEnabled, AffCode: "device-b"}}
	for i := range users {
		require.NoError(t, DB.Create(&users[i]).Error)
	}
	for _, user := range users {
		require.NoError(t, DB.Create(&Token{UserId: user.Id, Key: "key-" + user.Username, Status: common.TokenStatusEnabled}).Error)
	}
	hash := HashDeviceFingerprint("same-device")
	devices := []UserDevice{{UserId: users[0].Id, FingerprintHash: hash}, {UserId: users[1].Id, FingerprintHash: hash}}
	for i := range devices {
		require.NoError(t, DB.Create(&devices[i]).Error)
	}

	affected, err := SetUserDeviceBan(devices[0].Id, true)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{users[0].Id, users[1].Id}, affected)
	var bannedDevices int64
	require.NoError(t, DB.Model(&UserDevice{}).Where("fingerprint_hash = ? AND banned = ?", hash, true).Count(&bannedDevices).Error)
	assert.Equal(t, int64(2), bannedDevices)
	var disabledUsers, disabledTokens int64
	require.NoError(t, DB.Model(&User{}).Where("id IN ? AND status = ?", affected, common.UserStatusDisabled).Count(&disabledUsers).Error)
	require.NoError(t, DB.Model(&Token{}).Where("user_id IN ? AND status = ?", affected, common.TokenStatusDisabled).Count(&disabledTokens).Error)
	assert.Equal(t, int64(2), disabledUsers)
	assert.Equal(t, int64(2), disabledTokens)
}

func TestUnbanDeviceDoesNotRestoreUsersOrTokens(t *testing.T) {
	setupUserDeviceTestDB(t)
	user := User{Username: "disabled-device-user", Password: "password", Status: common.UserStatusDisabled, AffCode: "disabled-device"}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{UserId: user.Id, Key: "disabled-device-token", Status: common.TokenStatusDisabled}
	require.NoError(t, DB.Create(&token).Error)
	device := UserDevice{UserId: user.Id, FingerprintHash: HashDeviceFingerprint("blocked-device"), Banned: true, BannedAt: common.GetTimestamp()}
	require.NoError(t, DB.Create(&device).Error)

	affected, err := SetUserDeviceBan(device.Id, false)
	require.NoError(t, err)
	assert.Empty(t, affected)
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.NoError(t, DB.First(&token, token.Id).Error)
	require.NoError(t, DB.First(&device, device.Id).Error)
	assert.False(t, device.Banned)
	assert.Equal(t, common.UserStatusDisabled, user.Status)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
}

func TestGetUserDevicesReturnsDuplicateCountsAndFilters(t *testing.T) {
	setupUserDeviceTestDB(t)
	users := []User{
		{Username: "duplicate-user-a", Password: "password", Status: common.UserStatusEnabled, AffCode: "duplicate-a"},
		{Username: "duplicate-user-b", Password: "password", Status: common.UserStatusEnabled, AffCode: "duplicate-b"},
		{Username: "duplicate-user-c", Password: "password", Status: common.UserStatusEnabled, AffCode: "duplicate-c"},
	}
	for i := range users {
		require.NoError(t, DB.Create(&users[i]).Error)
	}
	sharedDevice := HashDeviceFingerprint("shared-device")
	sharedIP := "203.0.113.20"
	devices := []UserDevice{
		{UserId: users[0].Id, FingerprintHash: sharedDevice, FirstIp: sharedIP},
		{UserId: users[0].Id, FingerprintHash: HashDeviceFingerprint("user-a-second-device"), FirstIp: sharedIP},
		{UserId: users[1].Id, FingerprintHash: sharedDevice, FirstIp: "198.51.100.10"},
		{UserId: users[2].Id, FingerprintHash: HashDeviceFingerprint("user-c-device"), FirstIp: sharedIP},
		{UserId: users[2].Id, FingerprintHash: HashDeviceFingerprint("empty-ip-device"), FirstIp: ""},
	}
	for i := range devices {
		require.NoError(t, DB.Create(&devices[i]).Error)
	}
	page := &common.PageInfo{Page: 1, PageSize: 100}

	all, total, err := GetUserDevices(page, "", nil, "")
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	require.Len(t, all, 5)
	byID := make(map[int]UserDevice, len(all))
	for _, device := range all {
		byID[device.Id] = device
	}
	assert.Equal(t, int64(2), byID[devices[0].Id].DeviceUserCount)
	assert.Equal(t, int64(2), byID[devices[0].Id].DeviceRecordCount)
	assert.Equal(t, int64(2), byID[devices[0].Id].IpUserCount)
	assert.Equal(t, int64(3), byID[devices[0].Id].IpRecordCount)
	assert.Zero(t, byID[devices[4].Id].IpUserCount)
	assert.Zero(t, byID[devices[4].Id].IpRecordCount)

	duplicateDevices, total, err := GetUserDevices(page, "", nil, "device")
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, duplicateDevices, 2)
	for _, device := range duplicateDevices {
		assert.Equal(t, sharedDevice, device.FingerprintHash)
	}

	duplicateIPs, total, err := GetUserDevices(page, "", nil, "ip")
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, duplicateIPs, 3)
	for _, device := range duplicateIPs {
		assert.Equal(t, sharedIP, device.FirstIp)
	}
}

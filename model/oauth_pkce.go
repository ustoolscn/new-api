package model

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

const (
	OAuthPublicClientID   = "hi-codex"
	OAuthPublicClientName = "Hi Codex"
	OAuthPublicScope      = "api"
	OAuthTokenSource      = "oauth"

	OAuthAuthorizationRequestTTL = int64(10 * 60)
	OAuthAuthorizationCodeTTL    = int64(2 * 60)
	OAuthTokenTTL                = int64(30 * 24 * 60 * 60)
)

const (
	OAuthAuthorizationRequestPending = iota
	OAuthAuthorizationRequestApproved
	OAuthAuthorizationRequestDenied
	OAuthAuthorizationRequestExpired
)

var (
	ErrOAuthAuthorizationRequestNotFound = errors.New("oauth authorization request not found")
	ErrOAuthAuthorizationRequestExpired  = errors.New("oauth authorization request expired")
	ErrOAuthAuthorizationRequestUsed     = errors.New("oauth authorization request already used")
	ErrOAuthAuthorizationNonceInvalid    = errors.New("oauth authorization request nonce invalid")
	ErrOAuthAuthorizationCodeInvalid     = errors.New("oauth authorization code invalid")
	ErrOAuthAuthorizationCodeExpired     = errors.New("oauth authorization code expired")
	ErrOAuthAuthorizationCodeUsed        = errors.New("oauth authorization code already used")
	ErrOAuthAuthorizationUserDisabled    = errors.New("oauth authorization user disabled")
	ErrOAuthAuthorizationTokenLimit      = errors.New("oauth token limit reached")
)

// OAuthAuthorizationRequest is the short-lived browser authorization request.
// RequestIDHash and BrowserNonceHash are deliberately the only representations
// persisted for those two bearer values.
type OAuthAuthorizationRequest struct {
	Id                  int    `json:"-"`
	RequestIDHash       string `json:"-" gorm:"type:char(64);uniqueIndex;not null"`
	BrowserNonceHash    string `json:"-" gorm:"type:char(64);not null"`
	UserId              int    `json:"-" gorm:"index;not null"`
	ClientID            string `json:"-" gorm:"type:varchar(64);not null"`
	RedirectURI         string `json:"-" gorm:"type:varchar(2048);not null"`
	State               string `json:"-" gorm:"type:varchar(256);not null"`
	Scope               string `json:"-" gorm:"type:varchar(64);not null"`
	CodeChallenge       string `json:"-" gorm:"type:varchar(128);not null"`
	CodeChallengeMethod string `json:"-" gorm:"type:varchar(16);not null"`
	Status              int    `json:"-" gorm:"index;not null"`
	CreatedAt           int64  `json:"-" gorm:"bigint;not null"`
	ExpiresAt           int64  `json:"-" gorm:"bigint;not null;index"`
}

// OAuthAuthorizationCode is a one-time authorization code. The plaintext
// code is never written to the database; only CodeHash is persisted.
type OAuthAuthorizationCode struct {
	Id                  int    `json:"-"`
	CodeHash            string `json:"-" gorm:"type:char(64);uniqueIndex;not null"`
	RequestIDHash       string `json:"-" gorm:"type:char(64);index;not null"`
	UserId              int    `json:"-" gorm:"index;not null"`
	ClientID            string `json:"-" gorm:"type:varchar(64);not null"`
	RedirectURI         string `json:"-" gorm:"type:varchar(2048);not null"`
	State               string `json:"-" gorm:"type:varchar(256);not null"`
	Scope               string `json:"-" gorm:"type:varchar(64);not null"`
	CodeChallenge       string `json:"-" gorm:"type:varchar(128);not null"`
	CodeChallengeMethod string `json:"-" gorm:"type:varchar(16);not null"`
	CreatedAt           int64  `json:"-" gorm:"bigint;not null"`
	ExpiresAt           int64  `json:"-" gorm:"bigint;not null;index"`
	ConsumedAt          *int64 `json:"-" gorm:"bigint;index"`
	TokenId             int    `json:"-" gorm:"index"`
}

type OAuthAuthorizationDecision struct {
	Code        string
	RedirectURI string
	State       string
}

// HashOAuthValue returns the SHA-256 hash used for short-lived OAuth bearer
// values in the database and session key derivation.
func HashOAuthValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func generateOAuthSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// ValidateOAuthCodeVerifier checks the RFC 7636 unreserved character set and
// length bounds. It intentionally rejects padding and all other characters.
func ValidateOAuthCodeVerifier(verifier string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	for i := 0; i < len(verifier); i++ {
		ch := verifier[i]
		if (ch >= 'A' && ch <= 'Z') ||
			(ch >= 'a' && ch <= 'z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '.' || ch == '_' || ch == '~' {
			continue
		}
		return false
	}
	return true
}

// ValidateOAuthCodeChallenge validates an S256 challenge's canonical
// base64url representation (32 bytes, no padding).
func ValidateOAuthCodeChallenge(challenge string) bool {
	if len(challenge) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(challenge)
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	return base64.RawURLEncoding.EncodeToString(decoded) == challenge
}

func VerifyOAuthCodeChallenge(verifier string, challenge string) bool {
	if !ValidateOAuthCodeVerifier(verifier) || !ValidateOAuthCodeChallenge(challenge) {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// CreateOAuthAuthorizationRequest persists a request while returning its
// plaintext request id and browser nonce for the active session.
func CreateOAuthAuthorizationRequest(userID int, clientID, redirectURI, state, scope, codeChallenge, codeChallengeMethod string, now int64) (string, string, error) {
	if DB == nil {
		return "", "", gorm.ErrInvalidDB
	}
	if codeChallengeMethod != "S256" || !ValidateOAuthCodeChallenge(codeChallenge) {
		return "", "", ErrOAuthAuthorizationCodeInvalid
	}
	requestID, err := generateOAuthSecret()
	if err != nil {
		return "", "", err
	}
	browserNonce, err := generateOAuthSecret()
	if err != nil {
		return "", "", err
	}
	request := &OAuthAuthorizationRequest{
		RequestIDHash:       HashOAuthValue(requestID),
		BrowserNonceHash:    HashOAuthValue(browserNonce),
		UserId:              userID,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		State:               state,
		Scope:               scope,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Status:              OAuthAuthorizationRequestPending,
		CreatedAt:           now,
		ExpiresAt:           now + OAuthAuthorizationRequestTTL,
	}
	if err := DB.Create(request).Error; err != nil {
		return "", "", err
	}
	return requestID, browserNonce, nil
}

// DecideOAuthAuthorizationRequest atomically consumes a pending browser
// request. Approval creates a one-time code in the same transaction.
func DecideOAuthAuthorizationRequest(requestIDHash string, userID int, browserNonceHash string, approve bool, now int64) (*OAuthAuthorizationDecision, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	var request OAuthAuthorizationRequest
	if err := lockForUpdate(tx).
		Where("request_id_hash = ? AND user_id = ?", requestIDHash, userID).
		First(&request).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOAuthAuthorizationRequestNotFound
		}
		return nil, err
	}
	if request.Status != OAuthAuthorizationRequestPending {
		tx.Rollback()
		if request.Status == OAuthAuthorizationRequestExpired || request.ExpiresAt <= now {
			return nil, ErrOAuthAuthorizationRequestExpired
		}
		return nil, ErrOAuthAuthorizationRequestUsed
	}
	if request.ExpiresAt <= now {
		if err := tx.Model(&request).Update("status", OAuthAuthorizationRequestExpired).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Commit().Error; err != nil {
			return nil, err
		}
		return nil, ErrOAuthAuthorizationRequestExpired
	}
	if subtle.ConstantTimeCompare([]byte(request.BrowserNonceHash), []byte(browserNonceHash)) != 1 {
		tx.Rollback()
		return nil, ErrOAuthAuthorizationNonceInvalid
	}

	decision := &OAuthAuthorizationDecision{
		RedirectURI: request.RedirectURI,
		State:       request.State,
	}
	if !approve {
		if err := tx.Model(&request).Updates(map[string]any{
			"status": OAuthAuthorizationRequestDenied,
		}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Commit().Error; err != nil {
			return nil, err
		}
		return decision, nil
	}

	code, err := generateOAuthSecret()
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	codeRecord := &OAuthAuthorizationCode{
		CodeHash:            HashOAuthValue(code),
		RequestIDHash:       request.RequestIDHash,
		UserId:              request.UserId,
		ClientID:            request.ClientID,
		RedirectURI:         request.RedirectURI,
		State:               request.State,
		Scope:               request.Scope,
		CodeChallenge:       request.CodeChallenge,
		CodeChallengeMethod: request.CodeChallengeMethod,
		CreatedAt:           now,
		ExpiresAt:           now + OAuthAuthorizationCodeTTL,
	}
	if err := tx.Create(codeRecord).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Model(&request).Updates(map[string]any{
		"status": OAuthAuthorizationRequestApproved,
	}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	decision.Code = code
	return decision, nil
}

// RedeemOAuthAuthorizationCode atomically validates and consumes a code,
// creating the dedicated Hi Codex API token in the same transaction.
func RedeemOAuthAuthorizationCode(code, clientID, redirectURI, codeVerifier string, now int64) (*Token, string, error) {
	if DB == nil {
		return nil, "", gorm.ErrInvalidDB
	}
	if !ValidateOAuthCodeVerifier(codeVerifier) {
		return nil, "", ErrOAuthAuthorizationCodeInvalid
	}
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, "", tx.Error
	}
	var codeRecord OAuthAuthorizationCode
	if err := lockForUpdate(tx).Where("code_hash = ?", HashOAuthValue(code)).First(&codeRecord).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrOAuthAuthorizationCodeInvalid
		}
		return nil, "", err
	}
	if codeRecord.ConsumedAt != nil {
		tx.Rollback()
		return nil, "", ErrOAuthAuthorizationCodeUsed
	}
	if codeRecord.ExpiresAt <= now {
		tx.Rollback()
		return nil, "", ErrOAuthAuthorizationCodeExpired
	}
	if codeRecord.ClientID != clientID || codeRecord.RedirectURI != redirectURI ||
		codeRecord.CodeChallengeMethod != "S256" ||
		!VerifyOAuthCodeChallenge(codeVerifier, codeRecord.CodeChallenge) {
		tx.Rollback()
		return nil, "", ErrOAuthAuthorizationCodeInvalid
	}

	var user User
	if err := lockForUpdate(tx).Select("id", "status").Where("id = ?", codeRecord.UserId).First(&user).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrOAuthAuthorizationCodeInvalid
		}
		return nil, "", err
	}
	if user.Status != common.UserStatusEnabled {
		tx.Rollback()
		return nil, "", ErrOAuthAuthorizationUserDisabled
	}
	maxTokens := operation_setting.GetMaxUserTokens()
	var tokenCount int64
	if err := tx.Model(&Token{}).Where("user_id = ?", codeRecord.UserId).Count(&tokenCount).Error; err != nil {
		tx.Rollback()
		return nil, "", err
	}
	if maxTokens <= 0 || tokenCount >= int64(maxTokens) {
		tx.Rollback()
		return nil, "", ErrOAuthAuthorizationTokenLimit
	}

	key, err := common.GenerateKey()
	if err != nil {
		tx.Rollback()
		return nil, "", err
	}
	token := &Token{
		UserId:         codeRecord.UserId,
		Key:            key,
		Status:         common.TokenStatusEnabled,
		Name:           OAuthPublicClientName,
		CreatedTime:    now,
		AccessedTime:   now,
		ExpiredTime:    now + OAuthTokenTTL,
		UnlimitedQuota: true,
		Source:         OAuthTokenSource,
		OAuthClientID:  codeRecord.ClientID,
		OAuthScopes:    codeRecord.Scope,
	}
	if err := tx.Create(token).Error; err != nil {
		tx.Rollback()
		return nil, "", err
	}
	consumedAt := now
	if err := tx.Model(&codeRecord).Updates(map[string]any{
		"consumed_at": consumedAt,
		"token_id":    token.Id,
	}).Error; err != nil {
		tx.Rollback()
		return nil, "", err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, "", err
	}
	return token, "sk-" + key, nil
}

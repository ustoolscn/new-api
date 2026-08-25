package model

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	OAuthPublicClientID         = "hi-codex"
	OAuthPublicClientName       = "Hi Codex"
	OAuthDreamFactoryClientID   = "dreamfactory"
	OAuthDreamFactoryClientName = "DreamFactory"
	OAuthScopeAPIKeysRead       = "api_keys:read"
	OAuthScopeAccountRead       = "account:read"
	OAuthPublicScope            = OAuthScopeAPIKeysRead + " " + OAuthScopeAccountRead

	OAuthAuthorizationRequestTTL = int64(10 * 60)
	OAuthAuthorizationCodeTTL    = int64(2 * 60)
	OAuthTokenTTL                = int64(30 * 24 * 60 * 60)
)

// OAuthPublicClientNameForID returns the display name for a supported public
// OAuth client. Empty string means the client ID is not registered.
func OAuthPublicClientNameForID(clientID string) string {
	switch clientID {
	case OAuthPublicClientID:
		return OAuthPublicClientName
	case OAuthDreamFactoryClientID:
		return OAuthDreamFactoryClientName
	default:
		return ""
	}
}

func IsOAuthPublicClient(clientID string) bool {
	return OAuthPublicClientNameForID(clientID) != ""
}

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
	// TokenId is retained for incremental compatibility with databases that
	// were migrated by an unreleased OAuth API-key implementation. New code
	// must never populate this legacy column.
	TokenId       int `json:"-" gorm:"index"`
	AccessTokenId int `json:"-" gorm:"index"`
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

// NormalizeOAuthScope validates the public OAuth scope contract and returns
// its canonical representation. Scope values are whitespace-separated and
// may be supplied in either order, but both permissions must appear exactly
// once and no unknown permissions are accepted.
func NormalizeOAuthScope(scope string) (string, error) {
	parts := strings.Fields(scope)
	if len(parts) != 2 {
		return "", errors.New("scope must contain api_keys:read and account:read exactly once")
	}
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if part != OAuthScopeAPIKeysRead && part != OAuthScopeAccountRead {
			return "", errors.New("scope contains an unknown permission")
		}
		if _, exists := seen[part]; exists {
			return "", errors.New("scope contains duplicate permissions")
		}
		seen[part] = struct{}{}
	}
	if len(seen) != 2 {
		return "", errors.New("scope must contain api_keys:read and account:read")
	}
	return OAuthPublicScope, nil
}

// OAuthScopeAllows reports whether a canonical OAuth scope grants every
// requested permission.
func OAuthScopeAllows(scope string, requiredScopes ...string) bool {
	normalized, err := NormalizeOAuthScope(scope)
	if err != nil {
		return false
	}
	granted := make(map[string]struct{}, 2)
	for _, part := range strings.Fields(normalized) {
		granted[part] = struct{}{}
	}
	for _, required := range requiredScopes {
		required = strings.TrimSpace(required)
		if _, ok := granted[required]; !ok {
			return false
		}
	}
	return true
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
	normalizedScope, err := NormalizeOAuthScope(scope)
	if err != nil {
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
		Scope:               normalizedScope,
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
// creating one dedicated OAuth access token in the same transaction. SQLite
// has no row-level FOR UPDATE and may report a transient writer deadlock when
// two redemptions race; retrying the whole transaction lets the winner commit
// and the loser observe consumed_at on its next attempt.
func RedeemOAuthAuthorizationCode(code, clientID, redirectURI, codeVerifier string, now int64) (*OAuthAccessToken, string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		accessToken, value, err := redeemOAuthAuthorizationCodeOnce(code, clientID, redirectURI, codeVerifier, now)
		if !isOAuthSQLiteBusyError(err) || attempt == 2 {
			return accessToken, value, err
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return nil, "", ErrOAuthAuthorizationCodeUsed
}

func isOAuthSQLiteBusyError(err error) bool {
	if err == nil || !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "database is deadlocked") ||
		strings.Contains(message, "database is busy")
}

func redeemOAuthAuthorizationCodeOnce(code, clientID, redirectURI, codeVerifier string, now int64) (*OAuthAccessToken, string, error) {
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
	normalizedScope, err := NormalizeOAuthScope(codeRecord.Scope)
	if err != nil {
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
	secret, err := generateOAuthSecret()
	if err != nil {
		tx.Rollback()
		return nil, "", err
	}
	consumedAt := now
	// Claim the one-time code before inserting the access token. On SQLite
	// this turns a concurrent loser into a retry/used result instead of two
	// transactions both attempting to create token rows while reading the
	// same unconsumed code snapshot.
	claim := tx.Model(&OAuthAuthorizationCode{}).
		Where("id = ? AND consumed_at IS NULL", codeRecord.Id).
		Update("consumed_at", consumedAt)
	if claim.Error != nil {
		tx.Rollback()
		return nil, "", claim.Error
	}
	if claim.RowsAffected != 1 {
		tx.Rollback()
		return nil, "", ErrOAuthAuthorizationCodeUsed
	}
	accessTokenValue := "oa-" + secret
	accessToken := &OAuthAccessToken{
		TokenHash: HashOAuthValue(accessTokenValue),
		UserId:    codeRecord.UserId,
		ClientID:  codeRecord.ClientID,
		Scope:     normalizedScope,
		CreatedAt: now,
		ExpiresAt: now + OAuthTokenTTL,
	}
	if err := tx.Create(accessToken).Error; err != nil {
		tx.Rollback()
		return nil, "", err
	}
	result := tx.Model(&OAuthAuthorizationCode{}).
		Where("id = ? AND consumed_at IS NOT NULL", codeRecord.Id).
		Update("access_token_id", accessToken.Id)
	if result.Error != nil {
		tx.Rollback()
		return nil, "", result.Error
	}
	if result.RowsAffected != 1 {
		tx.Rollback()
		return nil, "", ErrOAuthAuthorizationCodeUsed
	}
	if err := tx.Commit().Error; err != nil {
		return nil, "", err
	}
	return accessToken, accessTokenValue, nil
}

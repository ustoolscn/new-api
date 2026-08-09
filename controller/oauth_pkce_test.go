package controller

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	oauthPKCETestVerifier = "oauth-test-verifier-abcdefghijklmnopqrstuvwxyz-0123456789"
	oauthPKCETestRedirect = "http://127.0.0.1:45678/callback"
)

type oauthPKCEErrorResponse struct {
	Success          bool   `json:"success"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Message          string `json:"message"`
}

type oauthPKCEAuthorizeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		RequestID string `json:"request_id"`
	} `json:"data"`
}

type oauthPKCETokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
}

func setupOAuthPKCETestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.OAuthAuthorizationRequest{},
		&model.OAuthAuthorizationCode{},
	))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled = oldRedisEnabled
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func seedOAuthPKCETestUser(t *testing.T, db *gorm.DB) *model.User {
	t.Helper()

	user := &model.User{
		Username:    "oauth-pkce-user",
		Password:    "password123",
		DisplayName: "OAuth PKCE User",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

func newOAuthPKCETestServer(t *testing.T, userID int) (*httptest.Server, *http.Client) {
	t.Helper()

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("oauth-pkce-test-secret"))))
	router.GET("/test-login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", userID)
		session.Set("username", "oauth-pkce-user")
		session.Set("role", common.RoleCommonUser)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/oauth/authorize", middleware.SessionOnlyAuth(), OAuthAuthorize)
	router.POST("/oauth/authorize/decision", middleware.SessionOnlyAuth(), OAuthAuthorizeDecision)
	router.POST("/oauth/token", OAuthToken)

	server := httptest.NewServer(router)
	client := server.Client()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client.Jar = jar
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	t.Cleanup(server.Close)
	return server, client
}

func loginOAuthPKCETestClient(t *testing.T, server *httptest.Server, client *http.Client) {
	t.Helper()

	response, err := client.Get(server.URL + "/test-login")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusNoContent, response.StatusCode)
}

func oauthPKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func oauthPKCEAuthorizeValues(verifier, state, redirectURI string) url.Values {
	return url.Values{
		"client_id":             {model.OAuthPublicClientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"scope":                 {model.OAuthPublicScope},
		"code_challenge":        {oauthPKCEChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
}

func readOAuthPKCEBody(t *testing.T, response *http.Response) []byte {
	t.Helper()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	return body
}

func readOAuthPKCEErrorResponse(t *testing.T, response *http.Response) oauthPKCEErrorResponse {
	t.Helper()

	body := readOAuthPKCEBody(t, response)
	var payload oauthPKCEErrorResponse
	require.NoError(t, common.Unmarshal(body, &payload))
	return payload
}

func readOAuthPKCEAuthorizeResponse(t *testing.T, response *http.Response) oauthPKCEAuthorizeResponse {
	t.Helper()

	body := readOAuthPKCEBody(t, response)
	var payload oauthPKCEAuthorizeResponse
	require.NoError(t, common.Unmarshal(body, &payload))
	return payload
}

func readOAuthPKCETokenResponse(t *testing.T, response *http.Response) oauthPKCETokenResponse {
	t.Helper()

	body := readOAuthPKCEBody(t, response)
	var payload oauthPKCETokenResponse
	require.NoError(t, common.Unmarshal(body, &payload))
	return payload
}

func beginOAuthPKCEAuthorization(t *testing.T, server *httptest.Server, client *http.Client, verifier, state, redirectURI string) string {
	t.Helper()

	values := oauthPKCEAuthorizeValues(verifier, state, redirectURI)
	response, err := client.Get(server.URL + "/oauth/authorize?" + values.Encode())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	payload := readOAuthPKCEAuthorizeResponse(t, response)
	require.True(t, payload.Success, payload.Message)
	require.NotEmpty(t, payload.Data.RequestID)
	return payload.Data.RequestID
}

func decideOAuthPKCEAuthorization(t *testing.T, server *httptest.Server, client *http.Client, requestID, decision string) *url.URL {
	t.Helper()

	response, err := client.PostForm(server.URL+"/oauth/authorize/decision", url.Values{
		"request_id": {requestID},
		"decision":   {decision},
	})
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusFound, response.StatusCode)
	location, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	return location
}

func postOAuthPKCEToken(t *testing.T, server *httptest.Server, client *http.Client, code, clientID, redirectURI, verifier string) (int, oauthPKCETokenResponse, oauthPKCEErrorResponse) {
	t.Helper()

	response, err := client.PostForm(server.URL+"/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	require.NoError(t, err)
	status := response.StatusCode
	if status == http.StatusOK {
		return status, readOAuthPKCETokenResponse(t, response), oauthPKCEErrorResponse{}
	}
	return status, oauthPKCETokenResponse{}, readOAuthPKCEErrorResponse(t, response)
}

func TestOAuthPKCEAuthorizeApproveTokenFlow(t *testing.T) {
	db := setupOAuthPKCETestDB(t)
	user := seedOAuthPKCETestUser(t, db)
	server, client := newOAuthPKCETestServer(t, user.Id)
	loginOAuthPKCETestClient(t, server, client)

	requestID := beginOAuthPKCEAuthorization(t, server, client, oauthPKCETestVerifier, "state-123", oauthPKCETestRedirect)
	location := decideOAuthPKCEAuthorization(t, server, client, requestID, "approve")
	query := location.Query()
	assert.Equal(t, "state-123", query.Get("state"))
	code := query.Get("code")
	require.NotEmpty(t, code)

	status, payload, _ := postOAuthPKCEToken(t, server, client, code, model.OAuthPublicClientID, oauthPKCETestRedirect, oauthPKCETestVerifier)
	require.Equal(t, http.StatusOK, status)
	assert.True(t, strings.HasPrefix(payload.AccessToken, "sk-"))
	assert.Equal(t, "Bearer", payload.TokenType)
	assert.Equal(t, model.OAuthPublicScope, payload.Scope)
	assert.GreaterOrEqual(t, payload.ExpiresIn, model.OAuthTokenTTL-1)
	assert.LessOrEqual(t, payload.ExpiresIn, model.OAuthTokenTTL)

	var token model.Token
	require.NoError(t, db.Where("user_id = ? AND source = ?", user.Id, model.OAuthTokenSource).First(&token).Error)
	assert.Equal(t, user.Id, token.UserId)
	assert.Equal(t, model.OAuthPublicClientID, token.OAuthClientID)
	assert.Equal(t, model.OAuthPublicScope, token.OAuthScopes)
	assert.Equal(t, model.OAuthTokenTTL, token.ExpiredTime-token.CreatedTime)
	assert.Equal(t, "sk-"+token.Key, payload.AccessToken)

	var tokenCount int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ? AND source = ?", user.Id, model.OAuthTokenSource).Count(&tokenCount).Error)
	assert.Equal(t, int64(1), tokenCount)
}

func TestOAuthPKCEAuthorizeRequiresLogin(t *testing.T) {
	db := setupOAuthPKCETestDB(t)
	user := seedOAuthPKCETestUser(t, db)
	server, client := newOAuthPKCETestServer(t, user.Id)
	values := oauthPKCEAuthorizeValues(oauthPKCETestVerifier, "state", oauthPKCETestRedirect)

	response, err := client.Get(server.URL + "/oauth/authorize?" + values.Encode())
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	payload := readOAuthPKCEErrorResponse(t, response)
	assert.False(t, payload.Success)
	assert.Equal(t, "login_required", payload.Error)
}

func TestOAuthPKCEAuthorizeRejectsInvalidParameters(t *testing.T) {
	db := setupOAuthPKCETestDB(t)
	user := seedOAuthPKCETestUser(t, db)
	server, client := newOAuthPKCETestServer(t, user.Id)
	loginOAuthPKCETestClient(t, server, client)

	tests := []struct {
		name  string
		field string
		value string
	}{
		{name: "client", field: "client_id", value: "other-client"},
		{name: "response type", field: "response_type", value: "token"},
		{name: "scope", field: "scope", value: "openid"},
		{name: "external host", field: "redirect_uri", value: "http://example.com:45678/callback"},
		{name: "localhost name", field: "redirect_uri", value: "http://localhost:45678/callback"},
		{name: "missing port", field: "redirect_uri", value: "http://127.0.0.1/callback"},
		{name: "userinfo", field: "redirect_uri", value: "http://user@127.0.0.1:45678/callback"},
		{name: "fragment", field: "redirect_uri", value: "http://127.0.0.1:45678/callback#fragment"},
		{name: "query", field: "redirect_uri", value: "http://127.0.0.1:45678/callback?next=1"},
		{name: "plain pkce", field: "code_challenge_method", value: "plain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := oauthPKCEAuthorizeValues(oauthPKCETestVerifier, "state", oauthPKCETestRedirect)
			values.Set(tt.field, tt.value)
			response, err := client.Get(server.URL + "/oauth/authorize?" + values.Encode())
			require.NoError(t, err)
			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			payload := readOAuthPKCEErrorResponse(t, response)
			assert.False(t, payload.Success)
			assert.Equal(t, "invalid_request", payload.Error)
		})
	}
}

func TestOAuthPKCEDeniedAuthorizationPreservesState(t *testing.T) {
	db := setupOAuthPKCETestDB(t)
	user := seedOAuthPKCETestUser(t, db)
	server, client := newOAuthPKCETestServer(t, user.Id)
	loginOAuthPKCETestClient(t, server, client)

	requestID := beginOAuthPKCEAuthorization(t, server, client, oauthPKCETestVerifier, "deny-state", oauthPKCETestRedirect)
	location := decideOAuthPKCEAuthorization(t, server, client, requestID, "deny")
	query := location.Query()
	assert.Equal(t, "deny-state", query.Get("state"))
	assert.Equal(t, "access_denied", query.Get("error"))
	assert.Empty(t, query.Get("code"))

	var request model.OAuthAuthorizationRequest
	require.NoError(t, db.Where("request_id_hash = ?", model.HashOAuthValue(requestID)).First(&request).Error)
	assert.Equal(t, model.OAuthAuthorizationRequestDenied, request.Status)
	var codeCount int64
	require.NoError(t, db.Model(&model.OAuthAuthorizationCode{}).Where("request_id_hash = ?", request.RequestIDHash).Count(&codeCount).Error)
	assert.Zero(t, codeCount)
}

func TestOAuthPKCEWrongVerifierDoesNotConsumeCode(t *testing.T) {
	db := setupOAuthPKCETestDB(t)
	user := seedOAuthPKCETestUser(t, db)
	server, client := newOAuthPKCETestServer(t, user.Id)
	loginOAuthPKCETestClient(t, server, client)

	requestID := beginOAuthPKCEAuthorization(t, server, client, oauthPKCETestVerifier, "state", oauthPKCETestRedirect)
	location := decideOAuthPKCEAuthorization(t, server, client, requestID, "approve")
	code := location.Query().Get("code")
	require.NotEmpty(t, code)

	wrongVerifier := strings.Repeat("x", 43)
	status, _, payload := postOAuthPKCEToken(t, server, client, code, model.OAuthPublicClientID, oauthPKCETestRedirect, wrongVerifier)
	require.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_grant", payload.Error)

	var authorizationCode model.OAuthAuthorizationCode
	require.NoError(t, db.Where("code_hash = ?", model.HashOAuthValue(code)).First(&authorizationCode).Error)
	assert.Nil(t, authorizationCode.ConsumedAt)
	assert.Zero(t, authorizationCode.TokenId)

	status, tokenPayload, _ := postOAuthPKCEToken(t, server, client, code, model.OAuthPublicClientID, oauthPKCETestRedirect, oauthPKCETestVerifier)
	require.Equal(t, http.StatusOK, status)
	assert.NotEmpty(t, tokenPayload.AccessToken)
}

func TestOAuthPKCEGrantRejectsMismatchedClientAndRedirectWithoutConsumingCode(t *testing.T) {
	db := setupOAuthPKCETestDB(t)
	user := seedOAuthPKCETestUser(t, db)
	server, client := newOAuthPKCETestServer(t, user.Id)
	loginOAuthPKCETestClient(t, server, client)

	requestID := beginOAuthPKCEAuthorization(t, server, client, oauthPKCETestVerifier, "state", oauthPKCETestRedirect)
	location := decideOAuthPKCEAuthorization(t, server, client, requestID, "approve")
	code := location.Query().Get("code")
	require.NotEmpty(t, code)

	status, _, payload := postOAuthPKCEToken(t, server, client, code, "other-client", oauthPKCETestRedirect, oauthPKCETestVerifier)
	require.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_client", payload.Error)

	status, _, payload = postOAuthPKCEToken(t, server, client, code, model.OAuthPublicClientID, "http://127.0.0.1:45679/callback", oauthPKCETestVerifier)
	require.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_grant", payload.Error)

	status, tokenPayload, _ := postOAuthPKCEToken(t, server, client, code, model.OAuthPublicClientID, oauthPKCETestRedirect, oauthPKCETestVerifier)
	require.Equal(t, http.StatusOK, status)
	assert.NotEmpty(t, tokenPayload.AccessToken)
}

func TestOAuthPKCEExpiredCodeCannotBeRedeemed(t *testing.T) {
	db := setupOAuthPKCETestDB(t)
	user := seedOAuthPKCETestUser(t, db)
	server, client := newOAuthPKCETestServer(t, user.Id)
	loginOAuthPKCETestClient(t, server, client)

	requestID := beginOAuthPKCEAuthorization(t, server, client, oauthPKCETestVerifier, "state", oauthPKCETestRedirect)
	location := decideOAuthPKCEAuthorization(t, server, client, requestID, "approve")
	code := location.Query().Get("code")
	require.NotEmpty(t, code)

	require.NoError(t, db.Model(&model.OAuthAuthorizationCode{}).
		Where("code_hash = ?", model.HashOAuthValue(code)).
		Update("expires_at", common.GetTimestamp()-1).Error)

	status, _, payload := postOAuthPKCEToken(t, server, client, code, model.OAuthPublicClientID, oauthPKCETestRedirect, oauthPKCETestVerifier)
	require.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_grant", payload.Error)

	var authorizationCode model.OAuthAuthorizationCode
	require.NoError(t, db.Where("code_hash = ?", model.HashOAuthValue(code)).First(&authorizationCode).Error)
	assert.Nil(t, authorizationCode.ConsumedAt)
}

func TestOAuthPKCEAuthorizationCodeCanOnlyBeRedeemedOnce(t *testing.T) {
	db := setupOAuthPKCETestDB(t)
	user := seedOAuthPKCETestUser(t, db)
	server, client := newOAuthPKCETestServer(t, user.Id)
	loginOAuthPKCETestClient(t, server, client)

	requestID := beginOAuthPKCEAuthorization(t, server, client, oauthPKCETestVerifier, "state", oauthPKCETestRedirect)
	location := decideOAuthPKCEAuthorization(t, server, client, requestID, "approve")
	code := location.Query().Get("code")
	require.NotEmpty(t, code)

	status, tokenPayload, _ := postOAuthPKCEToken(t, server, client, code, model.OAuthPublicClientID, oauthPKCETestRedirect, oauthPKCETestVerifier)
	require.Equal(t, http.StatusOK, status)
	assert.NotEmpty(t, tokenPayload.AccessToken)

	status, _, errorPayload := postOAuthPKCEToken(t, server, client, code, model.OAuthPublicClientID, oauthPKCETestRedirect, oauthPKCETestVerifier)
	require.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_grant", errorPayload.Error)

	var tokenCount int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ? AND source = ?", user.Id, model.OAuthTokenSource).Count(&tokenCount).Error)
	assert.Equal(t, int64(1), tokenCount)
}

package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	oauthAuthorizationResponseType = "code"
	oauthCodeChallengeMethod       = "S256"
	oauthStateMaxLength            = 256
	oauthRequestIDFormField        = "request_id"
	oauthDecisionFormField         = "decision"
)

func oauthSessionNonceKey(requestID string) string {
	return "oauth_pkce_nonce:" + model.HashOAuthValue(requestID)
}

func oauthError(c *gin.Context, status int, code string, description string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(status, gin.H{
		"error":             code,
		"error_description": description,
	})
}

func oauthAuthorizationParam(values url.Values, name string, required bool) (string, error) {
	items, exists := values[name]
	if !exists || len(items) == 0 || (required && len(items[0]) == 0) {
		if required {
			return "", fmt.Errorf("missing %s", name)
		}
		return "", nil
	}
	if len(items) != 1 {
		return "", fmt.Errorf("duplicate %s", name)
	}
	return items[0], nil
}

func validateOAuthState(state string) error {
	if state == "" {
		return errors.New("state is required")
	}
	if len(state) > oauthStateMaxLength {
		return errors.New("state is too long")
	}
	for _, r := range state {
		if unicode.IsControl(r) {
			return errors.New("state contains control characters")
		}
	}
	return nil
}

func validateOAuthRedirectURI(rawURI string) error {
	if rawURI == "" || len(rawURI) > 2048 {
		return errors.New("redirect_uri is invalid")
	}
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		strings.Contains(rawURI, "#") {
		return errors.New("redirect_uri must be an http loopback URL without query, fragment, or userinfo")
	}
	hostname := parsed.Hostname()
	if hostname != "127.0.0.1" && hostname != "::1" {
		return errors.New("redirect_uri host must be 127.0.0.1 or ::1")
	}
	portText := parsed.Port()
	if portText == "" {
		return errors.New("redirect_uri must include an explicit port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("redirect_uri port must be between 1 and 65535")
	}
	return nil
}

func validateOAuthAuthorizeParams(c *gin.Context) (url.Values, string, string, string, string, string, string, error) {
	values := c.Request.URL.Query()
	clientID, err := oauthAuthorizationParam(values, "client_id", true)
	if err != nil || clientID != model.OAuthPublicClientID {
		return nil, "", "", "", "", "", "", errors.New("client_id is invalid")
	}
	responseType, err := oauthAuthorizationParam(values, "response_type", true)
	if err != nil || responseType != oauthAuthorizationResponseType {
		return nil, "", "", "", "", "", "", errors.New("response_type must be code")
	}
	redirectURI, err := oauthAuthorizationParam(values, "redirect_uri", true)
	if err != nil {
		return nil, "", "", "", "", "", "", err
	}
	if err := validateOAuthRedirectURI(redirectURI); err != nil {
		return nil, "", "", "", "", "", "", err
	}
	state, err := oauthAuthorizationParam(values, "state", true)
	if err != nil {
		return nil, "", "", "", "", "", "", err
	}
	if err := validateOAuthState(state); err != nil {
		return nil, "", "", "", "", "", "", err
	}
	scope, err := oauthAuthorizationParam(values, "scope", true)
	if err != nil {
		return nil, "", "", "", "", "", "", err
	}
	normalizedScope, err := model.NormalizeOAuthScope(scope)
	if err != nil {
		return nil, "", "", "", "", "", "", err
	}
	scope = normalizedScope
	codeChallenge, err := oauthAuthorizationParam(values, "code_challenge", true)
	if err != nil || !model.ValidateOAuthCodeChallenge(codeChallenge) {
		return nil, "", "", "", "", "", "", errors.New("code_challenge is invalid")
	}
	codeChallengeMethod, err := oauthAuthorizationParam(values, "code_challenge_method", true)
	if err != nil || codeChallengeMethod != oauthCodeChallengeMethod {
		return nil, "", "", "", "", "", "", errors.New("code_challenge_method must be S256")
	}
	return values, clientID, redirectURI, state, scope, codeChallenge, codeChallengeMethod, nil
}

// OAuthAuthorize starts a short-lived authorization request for the logged-in
// dashboard user. The request id is returned in the normal project envelope;
// the browser nonce remains in the server-side session.
func OAuthAuthorize(c *gin.Context) {
	_, clientID, redirectURI, state, scope, codeChallenge, codeChallengeMethod, err := validateOAuthAuthorizeParams(c)
	if err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	now := common.GetTimestamp()
	requestID, browserNonce, err := model.CreateOAuthAuthorizationRequest(
		c.GetInt("id"), clientID, redirectURI, state, scope, codeChallenge, codeChallengeMethod, now,
	)
	if err != nil {
		common.SysError("failed to create OAuth authorization request: " + err.Error())
		oauthError(c, http.StatusInternalServerError, "server_error", "failed to create authorization request")
		return
	}
	session := sessions.Default(c)
	session.Set(oauthSessionNonceKey(requestID), browserNonce)
	if err := session.Save(); err != nil {
		common.SysError("failed to save OAuth browser nonce: " + err.Error())
		oauthError(c, http.StatusInternalServerError, "server_error", "failed to save authorization session")
		return
	}
	user := gin.H{
		"id":       c.GetInt("id"),
		"username": c.GetString("username"),
	}
	data := gin.H{
		"request_id":   requestID,
		"client_id":    clientID,
		"client_name":  model.OAuthPublicClientName,
		"redirect_uri": redirectURI,
		"scopes":       []string{model.OAuthScopeAPIKeysRead, model.OAuthScopeAccountRead},
		"user":         user,
		"expires_at":   now + model.OAuthAuthorizationRequestTTL,
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

func parseOAuthDecision(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approve", "approved", "allow", "yes", "true", "1":
		return true, nil
	case "deny", "denied", "reject", "no", "false", "0":
		return false, nil
	default:
		return false, errors.New("decision must be approve or deny")
	}
}

func oauthDecisionRedirect(decision *model.OAuthAuthorizationDecision, approved bool) (string, error) {
	if err := validateOAuthRedirectURI(decision.RedirectURI); err != nil {
		return "", err
	}
	parsed, err := url.Parse(decision.RedirectURI)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("state", decision.State)
	if approved {
		query.Set("code", decision.Code)
	} else {
		query.Set("error", "access_denied")
		query.Set("error_description", "the resource owner denied the request")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// OAuthAuthorizeDecision approves or denies an authorization request from the
// dashboard's native form. The redirect URI is always read from the database.
func OAuthAuthorizeDecision(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	requestID := c.PostForm(oauthRequestIDFormField)
	if requestID == "" || len(requestID) > 256 {
		oauthError(c, http.StatusBadRequest, "invalid_request", "request_id is invalid")
		return
	}
	decisionValue := c.PostForm(oauthDecisionFormField)
	if decisionValue == "" {
		decisionValue = c.PostForm("action")
	}
	if decisionValue == "" {
		decisionValue = c.PostForm("approve")
	}
	if decisionValue == "" {
		oauthError(c, http.StatusBadRequest, "invalid_request", "decision is required")
		return
	}
	approved, err := parseOAuthDecision(decisionValue)
	if err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	session := sessions.Default(c)
	nonce, ok := session.Get(oauthSessionNonceKey(requestID)).(string)
	if !ok || nonce == "" {
		oauthError(c, http.StatusBadRequest, "invalid_request", "authorization session is missing or expired")
		return
	}
	if providedNonce := c.PostForm("browser_nonce"); providedNonce != "" && providedNonce != nonce {
		oauthError(c, http.StatusBadRequest, "invalid_request", "browser nonce is invalid")
		return
	}
	decision, err := model.DecideOAuthAuthorizationRequest(
		model.HashOAuthValue(requestID), c.GetInt("id"), model.HashOAuthValue(nonce), approved, common.GetTimestamp(),
	)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrOAuthAuthorizationRequestNotFound),
			errors.Is(err, model.ErrOAuthAuthorizationRequestExpired),
			errors.Is(err, model.ErrOAuthAuthorizationRequestUsed),
			errors.Is(err, model.ErrOAuthAuthorizationNonceInvalid):
			oauthError(c, http.StatusBadRequest, "invalid_request", err.Error())
		default:
			common.SysError("failed to decide OAuth authorization request: " + err.Error())
			oauthError(c, http.StatusInternalServerError, "server_error", "failed to process authorization request")
		}
		return
	}
	session.Delete(oauthSessionNonceKey(requestID))
	if err := session.Save(); err != nil {
		common.SysLog("failed to clear OAuth browser nonce: " + err.Error())
	}
	redirectURI, err := oauthDecisionRedirect(decision, approved)
	if err != nil {
		oauthError(c, http.StatusInternalServerError, "server_error", "failed to build authorization redirect")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, redirectURI)
}

func tokenFormValue(c *gin.Context, name string) string {
	return c.PostForm(name)
}

// OAuthToken exchanges a one-time authorization code for a dedicated API
// token. It intentionally does not implement refresh tokens.
func OAuthToken(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	grantType := tokenFormValue(c, "grant_type")
	if grantType == "" {
		oauthError(c, http.StatusBadRequest, "invalid_request", "grant_type is required")
		return
	}
	if grantType != "authorization_code" {
		oauthError(c, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code")
		return
	}
	clientID := tokenFormValue(c, "client_id")
	if clientID != model.OAuthPublicClientID {
		oauthError(c, http.StatusBadRequest, "invalid_client", "client_id is invalid")
		return
	}
	code := tokenFormValue(c, "code")
	redirectURI := c.PostForm("redirect_uri")
	codeVerifier := tokenFormValue(c, "code_verifier")
	if code == "" || len(code) > 256 || redirectURI == "" || codeVerifier == "" {
		oauthError(c, http.StatusBadRequest, "invalid_request", "code, redirect_uri, and code_verifier are required")
		return
	}
	if err := validateOAuthRedirectURI(redirectURI); err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_grant", "redirect_uri is invalid")
		return
	}
	if !model.ValidateOAuthCodeVerifier(codeVerifier) {
		oauthError(c, http.StatusBadRequest, "invalid_grant", "code_verifier is invalid")
		return
	}
	_, accessToken, err := model.RedeemOAuthAuthorizationCode(code, clientID, redirectURI, codeVerifier, common.GetTimestamp())
	if err != nil {
		switch {
		case errors.Is(err, model.ErrOAuthAuthorizationCodeInvalid),
			errors.Is(err, model.ErrOAuthAuthorizationCodeExpired),
			errors.Is(err, model.ErrOAuthAuthorizationCodeUsed),
			errors.Is(err, model.ErrOAuthAuthorizationUserDisabled):
			oauthError(c, http.StatusBadRequest, "invalid_grant", "authorization code is invalid, expired, or already used")
		default:
			common.SysError("failed to redeem OAuth authorization code: " + err.Error())
			oauthError(c, http.StatusInternalServerError, "server_error", "failed to issue access token")
		}
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   model.OAuthTokenTTL,
		"scope":        model.OAuthPublicScope,
	})
}

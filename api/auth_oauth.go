package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"

	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
)

const oauthStateCookieName = "oauthstate"

func (server *Server) initiateOAuth(ctx *gin.Context) {
	provider := ctx.Param("provider")
	sessionID := uuid.New().String()

	server.oauthSessionStore.CreateSession(sessionID)

	var authCodeURL string
	switch strings.ToLower(provider) {
	case "google":
		conf := server.getGoogleOAuthConfig()
		authCodeURL = conf.AuthCodeURL(sessionID)
	default:
		log.Warn().Str("provider", provider).Msg("unsupported provider")
		ctx.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("unsupported provider: %s", provider)))
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"session_id":    sessionID,
		"auth_code_url": authCodeURL,
	})
}

func (server *Server) pollOAuth(ctx *gin.Context) {
	sessionID := ctx.Param("session_id")

	// First, peek at the session without deleting it.
	session, ok := server.oauthSessionStore.PeekSession(sessionID)
	if !ok {
		ctx.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("session not found or already processed")))
		return
	}

	// If the session is still pending, let the client know to poll again.
	if session.Status == StatusPending {
		ctx.JSON(http.StatusOK, gin.H{"status": "pending"})
		return
	}

	// If the session is completed or failed, get the data and clear it from the store.
	finalSession, _ := server.oauthSessionStore.GetAndClearSession(sessionID)

	switch finalSession.Status {
	case StatusCompleted:
		ctx.JSON(http.StatusOK, gin.H{
			"status":                  "completed",
			"access_token":            finalSession.AccessToken,
			"access_token_expires_at": finalSession.AccessTokenExpiresAt,
			"parent":                  newParentResponse(*finalSession.Parent),
		})
	case StatusFailed:
		ctx.JSON(http.StatusBadRequest, gin.H{
			"status":  "failed",
			"message": finalSession.ErrorMessage,
		})
	}
}

func (server *Server) oauthParentStart(ctx *gin.Context) {
	provider := ctx.Param("provider")
	log.Info().Str("provider", provider).Msg("starting oauth flow")

	// Generate a random state string for CSRF protection.
	state, err := generateRandomState()
	if err != nil {
		log.Error().Err(err).Msg("failed to generate state")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Set the state in a secure, http-only cookie with SameSite=Lax.
	secureCookie := server.config.Environment == "production"

	// Parse domain from the redirect URL to handle ngrok domains
	redirectURL, err := url.Parse(server.config.OauthRedirectBaseURL)
	if err != nil {
		log.Error().Err(err).Str("url", server.config.OauthRedirectBaseURL).Msg("failed to parse redirect URL")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	http.SetCookie(ctx.Writer, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Domain:   redirectURL.Hostname(),
		Path:     "/",
		MaxAge:   int(10 * time.Minute.Seconds()),
		HttpOnly: true,
		Secure:   secureCookie,
		SameSite: http.SameSiteLaxMode,
	})

	log.Info().Str("state", state).Str("domain", redirectURL.Hostname()).Msg("set state cookie")

	var authCodeURL string
	switch strings.ToLower(provider) {
	case "google":
		conf := server.getGoogleOAuthConfig()
		authCodeURL = conf.AuthCodeURL(state)
		log.Info().Str("redirect_url", authCodeURL).Msg("redirecting user to provider for auth")
	default:
		log.Warn().Str("provider", provider).Msg("unsupported provider")
		ctx.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("unsupported provider: %s", provider)))
		return
	}

	ctx.Redirect(http.StatusTemporaryRedirect, authCodeURL)
}

func (server *Server) oauthParentCallback(ctx *gin.Context) {
	provider := ctx.Param("provider")
	sessionID := ctx.Query("state") // The state is our session ID
	log.Info().Str("provider", provider).Str("session_id", sessionID).Msg("handling oauth callback")

	// This function will now handle the entire logic and then return a simple HTML page.
	// We wrap the core logic in a function to easily handle success/failure session updates.
	parent, accessToken, accessPayload, err := server.processOAuthCallback(ctx, provider)
	if err != nil {
		// If any part of the process fails, we record the failure and show an error page.
		server.oauthSessionStore.FailSession(sessionID, err.Error())
		ctx.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fmt.Sprintf("<h1>Error</h1><p>An error occurred: %s</p><p>You can close this window.</p>", err.Error())))
		return
	}

	// If successful, we complete the session in the store.
	server.oauthSessionStore.CompleteSession(sessionID, accessToken, accessPayload.ExpiredAt, &parent)

	// Respond with a simple success page.
	ctx.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<h1>Success!</h1><p>You have successfully authenticated. You can now close this window and return to the Headlamp app.</p>"))
}

// processOAuthCallback contains the core logic for handling the callback from the provider.
func (server *Server) processOAuthCallback(ctx *gin.Context, provider string) (db.Parent, string, *token.Payload, error) {
	var (
		userInfo      GoogleUserInfo
		parent        db.Parent
		accessToken   string
		accessPayload *token.Payload
		token         *oauth2.Token
		payload       *idtoken.Payload
		authProvider  db.AuthProvider
		err           error
	)

	switch strings.ToLower(provider) {
	case "google":
		conf := server.getGoogleOAuthConfig()
		token, err = conf.Exchange(ctx, ctx.Query("code"))
		if err != nil {
			return db.Parent{}, "", nil, err
		}

		idTokenString, ok := token.Extra("id_token").(string)
		if !ok {
			return db.Parent{}, "", nil, fmt.Errorf("id_token not found in oauth token")
		}

		payload, err = idtoken.Validate(ctx, idTokenString, server.config.GoogleOauthClientID)
		if err != nil {
			return db.Parent{}, "", nil, err
		}

		userInfo, err = newGoogleUserInfoFromClaims(payload.Claims)
		if err != nil {
			return db.Parent{}, "", nil, err
		}
	default:
		return db.Parent{}, "", nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	log.Info().Str("email", userInfo.Email).Msg("verified user info")

	if !userInfo.EmailVerified {
		return db.Parent{}, "", nil, fmt.Errorf("email not verified by provider")
	}

	authProvider, err = ToAuthProvider(provider)
	if err != nil {
		return db.Parent{}, "", nil, err
	}

	parent, err = server.store.GetParentByProvider(ctx, db.GetParentByProviderParams{
		AuthProvider:    db.NullAuthProvider{AuthProvider: authProvider, Valid: true},
		ProviderSubject: pgtype.Text{String: userInfo.ID, Valid: true},
	})
	if err != nil {
		switch err {
		case db.ErrRecordNotFound:
			var existingParent db.Parent
			existingParent, err = server.store.GetParentByEmail(ctx, userInfo.Email)
			switch err {
			case nil:
				parent, err = server.store.LinkParentProvider(ctx, db.LinkParentProviderParams{
					ProviderSubject: pgtype.Text{String: userInfo.ID, Valid: true},
					AuthProvider:    db.NullAuthProvider{AuthProvider: authProvider, Valid: true},
					EmailVerified:   userInfo.EmailVerified,
					ID:              existingParent.ID,
				})
				if err != nil {
					return db.Parent{}, "", nil, err
				}
			case db.ErrRecordNotFound:
				txResult, err := server.store.CreateParentSocialTx(ctx, db.CreateParentSocialTxParams{
					Firstname:       userInfo.FirstName,
					Surname:         userInfo.LastName,
					Email:           userInfo.Email,
					AuthProvider:    db.NullAuthProvider{AuthProvider: authProvider, Valid: true},
					ProviderSubject: userInfo.ID,
					EmailVerified:   userInfo.EmailVerified,
				})
				if err != nil {
					return db.Parent{}, "", nil, err
				}
				parent = txResult.Parent
			default:
				return db.Parent{}, "", nil, err
			}
		default:
			return db.Parent{}, "", nil, err
		}
	}

	// Note: With Paseto, we don't need to sync users with Firebase

	accessToken, accessPayload, err = server.tokenMaker.CreateToken(
		parent.ParentID,
		parent.FamilyID, // familyID
		"",              // deviceID
		ParentUserProfile,
		time.Duration(BaseUserAccessTokenDurationInDays)*24*time.Hour,
	)
	if err != nil {
		return db.Parent{}, "", nil, err
	}

	return parent, accessToken, accessPayload, nil
}

type GoogleUserInfo struct {
	ID            string
	Email         string
	EmailVerified bool
	FirstName     string
	LastName      string
	Picture       string
}

func newGoogleUserInfoFromClaims(claims map[string]interface{}) (GoogleUserInfo, error) {
	var userInfo GoogleUserInfo
	var ok bool

	userInfo.ID, ok = claims["sub"].(string)
	if !ok {
		return userInfo, fmt.Errorf("sub claim is not a string")
	}

	userInfo.Email, ok = claims["email"].(string)
	if !ok {
		return userInfo, fmt.Errorf("email claim is not a string")
	}

	userInfo.EmailVerified, ok = claims["email_verified"].(bool)
	if !ok {
		return userInfo, fmt.Errorf("email_verified claim is not a bool")
	}

	userInfo.FirstName, _ = claims["given_name"].(string) // Optional
	userInfo.LastName, _ = claims["family_name"].(string) // Optional
	userInfo.Picture, _ = claims["picture"].(string)      // Optional

	return userInfo, nil
}

func (server *Server) getGoogleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     server.config.GoogleOauthClientID,
		ClientSecret: server.config.GoogleOauthClientSecret,
		RedirectURL:  fmt.Sprintf("%s/v1/auth/parent/oauth/google/callback", server.config.OauthRedirectBaseURL),
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
}

func generateRandomState() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func ToAuthProvider(provider string) (db.AuthProvider, error) {
	switch provider {
	case "google":
		return db.AuthProviderGoogle, nil
	case "apple":
		return db.AuthProviderApple, nil
	}
	return "", fmt.Errorf("unsupported provider: %s", provider)
}

// processOAuthIdTokenRequest represents the request body for processing an OAuth ID token.
type processOAuthIdTokenRequest struct {
	IdToken string `json:"id_token" binding:"required"`
}

// processOAuthIdTokenResponse represents the unified response for OAuth ID token processing.
type processOAuthIdTokenResponse struct {
	AccessToken          string         `json:"access_token"`
	AccessTokenExpiresAt time.Time      `json:"access_token_expires_at"`
	Parent               parentResponse `json:"parent"`
	Family               familyResponse `json:"family"`
}

// processOAuthIdToken handles the unified OAuth flow where the frontend sends an ID token.
// This works for all providers (Google, Apple, etc.) by verifying the ID token with the appropriate validator.
func (server *Server) processOAuthIdToken(ctx *gin.Context) {
	provider := ctx.Param("provider")

	var req processOAuthIdTokenRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	log.Info().Str("provider", provider).Msg("processing oauth id token")

	// Verify and extract user info based on provider
	var userInfo GoogleUserInfo
	var err error

	switch strings.ToLower(provider) {
	case "google":
		userInfo, err = server.verifyGoogleIdToken(ctx, req.IdToken)
		if err != nil {
			log.Error().Err(err).Msg("failed to verify google id token")
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid id token"})
			return
		}
	case "apple":
		// TODO: Apple Sign In needs to be reimplemented without Firebase.
		// Previously relied on Firebase ID token verification.
		// Needs direct Apple JWT verification or server-side OAuth flow.
		log.Error().Msg("apple sign in not supported without firebase")
		ctx.JSON(http.StatusNotImplemented, gin.H{"error": "apple sign in is not currently supported"})
		return
	default:
		log.Error().Str("provider", provider).Msg("unsupported provider")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported provider: %s", provider)})
		return
	}

	log.Info().
		Str("email", userInfo.Email).
		Str("provider", provider).
		Bool("email_verified", userInfo.EmailVerified).
		Msg("successfully verified id token")

	// Ensure email is verified
	if !userInfo.EmailVerified {
		log.Warn().Str("email", userInfo.Email).Msg("email not verified")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "email not verified"})
		return
	}

	// Convert provider string to AuthProvider enum
	authProvider, err := ToAuthProvider(provider)
	if err != nil {
		log.Error().Err(err).Str("provider", provider).Msg("unsupported provider")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported provider: %s", provider)})
		return
	}

	// Try to find parent by email (primary identifier for social auth)
	parent, err := server.store.GetParentByEmail(ctx, userInfo.Email)

	if err != nil {
		if err == db.ErrRecordNotFound {
			// Parent doesn't exist, create a new one
			log.Info().Str("email", userInfo.Email).Msg("creating new parent via oauth")

			txResult, err := server.store.CreateParentSocialTx(ctx, db.CreateParentSocialTxParams{
				Firstname:       userInfo.FirstName,
				Surname:         userInfo.LastName,
				Email:           userInfo.Email,
				AuthProvider:    db.NullAuthProvider{AuthProvider: authProvider, Valid: true},
				ProviderSubject: userInfo.ID,
				EmailVerified:   userInfo.EmailVerified,
			})
			if err != nil {
				log.Error().Err(err).Str("email", userInfo.Email).Msg("failed to create parent via oauth")
				ctx.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			parent = txResult.Parent

			log.Info().
				Str("parent_id", parent.ParentID).
				Str("family_id", parent.FamilyID).
				Str("email", userInfo.Email).
				Msg("successfully created new parent via oauth")
		} else {
			log.Error().Err(err).Str("email", userInfo.Email).Msg("failed to get parent by email")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
	} else {
		// Parent exists, check if we need to link the provider
		if !parent.AuthProvider.Valid || parent.ProviderSubject.String != userInfo.ID {
			log.Info().
				Str("parent_id", parent.ParentID).
				Str("email", userInfo.Email).
				Msg("linking oauth provider to existing parent")

			parent, err = server.store.LinkParentProvider(ctx, db.LinkParentProviderParams{
				ProviderSubject: pgtype.Text{String: userInfo.ID, Valid: true},
				AuthProvider:    db.NullAuthProvider{AuthProvider: authProvider, Valid: true},
				EmailVerified:   userInfo.EmailVerified,
				ID:              parent.ID,
			})
			if err != nil {
				log.Error().Err(err).Str("parent_id", parent.ParentID).Msg("failed to link provider to parent")
				ctx.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
		}

		log.Info().
			Str("parent_id", parent.ParentID).
			Str("email", userInfo.Email).
			Msg("parent logged in via oauth")
	}

	// Get family info
	family, err := server.store.GetFamily(ctx, parent.FamilyID)
	if err != nil {
		log.Error().Err(err).Str("family_id", parent.FamilyID).Msg("failed to get family")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Create our custom access token
	accessToken, accessPayload, err := server.tokenMaker.CreateToken(
		parent.ParentID,
		parent.FamilyID,
		"", // deviceID
		ParentUserProfile,
		time.Duration(BaseUserAccessTokenDurationInDays)*24*time.Hour,
	)
	if err != nil {
		log.Error().Err(err).Str("parent_id", parent.ParentID).Msg("failed to create access token")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Note: With Paseto, we don't need to sync users with an external auth provider

	log.Info().
		Str("parent_id", parent.ParentID).
		Str("family_id", family.ID).
		Msg("oauth id token processed successfully")

	ctx.JSON(http.StatusOK, processOAuthIdTokenResponse{
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessPayload.ExpiredAt,
		Parent:               newParentResponse(parent),
		Family: familyResponse{
			ID:        family.ID,
			CreatedAt: family.CreatedAt,
		},
	})
}

// verifyGoogleIdToken verifies a Google OAuth ID token and extracts user information.
func (server *Server) verifyGoogleIdToken(ctx *gin.Context, idTokenString string) (GoogleUserInfo, error) {
	var userInfo GoogleUserInfo

	// Validate the Google ID token using the Google OAuth client ID
	payload, err := idtoken.Validate(ctx, idTokenString, server.config.GoogleOauthClientID)
	if err != nil {
		return userInfo, fmt.Errorf("failed to validate google id token: %w", err)
	}

	// Extract user info from the validated token payload
	userInfo, err = newGoogleUserInfoFromClaims(payload.Claims)
	if err != nil {
		return userInfo, fmt.Errorf("failed to extract user info from claims: %w", err)
	}

	return userInfo, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Firebase unified auth  (Google + Apple)
// ─────────────────────────────────────────────────────────────────────────────

// processFirebaseIdTokenRequest is the request body Flutter sends after signing
// in with Google or Apple via the Firebase Auth SDK.
type processFirebaseIdTokenRequest struct {
	// IdToken is the Firebase ID token returned by FirebaseAuth.currentUser.getIdToken()
	IdToken string `json:"id_token" binding:"required"`
}

// processFirebaseIdToken verifies a Firebase ID token (issued after the user
// authenticates with Google or Apple inside the Flutter app) and returns a
// Headlamp PASETO access token.
//
// POST /v1/auth/parent/firebase
func (server *Server) processFirebaseIdToken(ctx *gin.Context) {
	if server.firebaseAuth == nil {
		log.Error().Msg("processFirebaseIdToken: Firebase auth client not initialised – set FIREBASE_SERVICE_ACCOUNT_JSON")
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "firebase auth is not configured on this server"})
		return
	}

	var req processFirebaseIdTokenRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	// ── 1. Verify the Firebase ID token ──────────────────────────────────────
	fbToken, err := server.firebaseAuth.VerifyIDToken(ctx, req.IdToken)
	if err != nil {
		log.Error().Err(err).Msg("processFirebaseIdToken: invalid firebase id token")
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid firebase id token"})
		return
	}

	// ── 2. Extract user info from the verified token ──────────────────────────
	claims := fbToken.Claims

	email, _ := claims["email"].(string)
	emailVerified, _ := claims["email_verified"].(bool)

	if email == "" {
		log.Error().Str("uid", fbToken.UID).Msg("processFirebaseIdToken: no email in firebase token")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "firebase token does not contain an email address"})
		return
	}
	if !emailVerified {
		log.Warn().Str("email", email).Msg("processFirebaseIdToken: email not verified")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "email address is not verified"})
		return
	}

	// Determine which social provider was used (google.com or apple.com)
	providerStr := "google" // default
	if identities, ok := fbToken.Firebase.Identities["apple.com"]; ok && identities != nil {
		providerStr = "apple"
	}

	authProvider, err := ToAuthProvider(providerStr)
	if err != nil {
		log.Error().Err(err).Str("provider", providerStr).Msg("processFirebaseIdToken: unsupported provider")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider"})
		return
	}

	// providerUID is the user's ID within the specific provider (google / apple)
	providerUID := fbToken.UID // Firebase UID — stable and unique per user
	if ids, ok := fbToken.Firebase.Identities[providerStr+".com"]; ok {
		if list, ok := ids.([]interface{}); ok && len(list) > 0 {
			if s, ok := list[0].(string); ok {
				providerUID = s
			}
		}
	}

	firstName, _ := claims["given_name"].(string)
	lastName, _ := claims["family_name"].(string)
	// Apple often withholds name after first login; fall back gracefully
	if firstName == "" {
		if name, ok := claims["name"].(string); ok {
			parts := strings.SplitN(name, " ", 2)
			firstName = parts[0]
			if len(parts) > 1 {
				lastName = parts[1]
			}
		}
	}

	log.Info().
		Str("email", email).
		Str("provider", providerStr).
		Str("firebase_uid", fbToken.UID).
		Msg("processFirebaseIdToken: firebase token verified")

	// ── 3. Find or create the parent record ───────────────────────────────────
	parent, err := server.store.GetParentByProvider(ctx, db.GetParentByProviderParams{
		AuthProvider:    db.NullAuthProvider{AuthProvider: authProvider, Valid: true},
		ProviderSubject: pgtype.Text{String: providerUID, Valid: true},
	})
	if err != nil {
		if err != db.ErrRecordNotFound {
			log.Error().Err(err).Msg("processFirebaseIdToken: GetParentByProvider failed")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}

		// Provider link not found — try by email
		existingParent, emailErr := server.store.GetParentByEmail(ctx, email)
		if emailErr != nil && emailErr != db.ErrRecordNotFound {
			log.Error().Err(emailErr).Msg("processFirebaseIdToken: GetParentByEmail failed")
			ctx.JSON(http.StatusInternalServerError, errorResponse(emailErr))
			return
		}

		if emailErr == nil {
			// Account exists under this email — link the provider
			parent, err = server.store.LinkParentProvider(ctx, db.LinkParentProviderParams{
				ProviderSubject: pgtype.Text{String: providerUID, Valid: true},
				AuthProvider:    db.NullAuthProvider{AuthProvider: authProvider, Valid: true},
				EmailVerified:   emailVerified,
				ID:              existingParent.ID,
			})
			if err != nil {
				log.Error().Err(err).Msg("processFirebaseIdToken: LinkParentProvider failed")
				ctx.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			log.Info().Str("parent_id", parent.ParentID).Msg("processFirebaseIdToken: linked provider to existing parent")
		} else {
			// Brand-new user — create parent + family in one transaction
			txResult, err := server.store.CreateParentSocialTx(ctx, db.CreateParentSocialTxParams{
				Firstname:       firstName,
				Surname:         lastName,
				Email:           email,
				AuthProvider:    db.NullAuthProvider{AuthProvider: authProvider, Valid: true},
				ProviderSubject: providerUID,
				EmailVerified:   emailVerified,
			})
			if err != nil {
				log.Error().Err(err).Msg("processFirebaseIdToken: CreateParentSocialTx failed")
				ctx.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			parent = txResult.Parent
			log.Info().
				Str("parent_id", parent.ParentID).
				Str("provider", providerStr).
				Msg("processFirebaseIdToken: new parent created via firebase")
		}
	}

	// ── 4. Fetch family ───────────────────────────────────────────────────────
	family, err := server.store.GetFamily(ctx, parent.FamilyID)
	if err != nil {
		log.Error().Err(err).Str("family_id", parent.FamilyID).Msg("processFirebaseIdToken: GetFamily failed")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// ── 5. Issue PASETO access token ──────────────────────────────────────────
	accessToken, accessPayload, err := server.tokenMaker.CreateToken(
		parent.ParentID,
		parent.FamilyID,
		"", // deviceID — parents are not device-bound
		ParentUserProfile,
		time.Duration(BaseUserAccessTokenDurationInDays)*24*time.Hour,
	)
	if err != nil {
		log.Error().Err(err).Msg("processFirebaseIdToken: CreateToken failed")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	log.Info().
		Str("parent_id", parent.ParentID).
		Str("family_id", family.ID).
		Str("provider", providerStr).
		Msg("processFirebaseIdToken: login successful")

	ctx.JSON(http.StatusOK, processOAuthIdTokenResponse{
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessPayload.ExpiredAt,
		Parent:               newParentResponse(parent),
		Family: familyResponse{
			ID:        family.ID,
			CreatedAt: family.CreatedAt,
		},
	})
}

package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	firebaseAdmin "firebase.google.com/go/v4"
	firebaseAuth "firebase.google.com/go/v4/auth"
	firebaseMessaging "firebase.google.com/go/v4/messaging"
	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/gpt"
	"github.com/The-You-School-HeadLamp/headlamp_backend/service"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
)

// Server serves HTTP requests for our service.
type Server struct {
	config            util.Config
	store             db.Store
	tokenMaker        token.Maker
	router            *gin.Engine
	httpServer        *http.Server
	oauthSessionStore *OAuthSessionStore
	uploader          *util.Uploader
	gptClient         gpt.GptClient
	firebaseAuth      *firebaseAuth.Client
	firebaseMsg       *firebaseMessaging.Client

	// Email
	emailService *util.EmailService

	// Rate limiters
	otpSendLimiter   *util.SlidingWindowRateLimiter // 3 per 15 min per email
	otpVerifyLimiter *util.SlidingWindowRateLimiter // 5 per 15 min per email

	// Services
	notificationService    *service.NotificationService
	reflectionService      *service.ReflectionService
	reflectionScheduler    *service.ReflectionScheduler
	insightsService        *service.InsightsService
	parentInsightService   *service.ParentInsightService
	parentInsightScheduler *service.ParentInsightScheduler
	sessionHub             *SessionHub
}

// NewServer creates a new HTTP server and sets up routing.
func NewServer(config util.Config, store db.Store, tokenMaker token.Maker, gptClient gpt.GptClient) (*Server, error) {
	uploader := util.NewUploader(config.ExternalContentBaseURL, config.ExternalContentToken)

	reflectionSvc := service.NewReflectionService(store, gptClient)
	insightsSvc := service.NewInsightsService(store, gptClient)
	parentInsightSvc := service.NewParentInsightService(store, gptClient)

	// Initialize Firebase Admin SDK
	fbAuthClient, fbMsgClient, err := initFirebaseApp(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase: %w", err)
	}

	notificationSvc := service.NewNotificationService(store, fbMsgClient)

	// Build email service from config (SMTP_HOST is optional; if blank, email is disabled)
	smtpHost := config.SMTPHost
	smtpPort := config.SMTPPort
	if smtpPort == 0 {
		smtpPort = 587 // default STARTTLS port
	}
	smtpUser := config.SMTPUser
	if smtpUser == "" {
		smtpUser = config.FromEmail // fall back to FROM_EMAIL as the auth username
	}
	emailSvc := util.NewEmailService(smtpHost, smtpPort, smtpUser, config.SMTPPass, config.FromEmail, config.FromName, "templates", config.AppBaseURL, config.LogoURL)

	server := &Server{
		config:                 config,
		store:                  store,
		tokenMaker:             tokenMaker,
		uploader:               uploader,
		oauthSessionStore:      NewOAuthSessionStore(3 * time.Minute),
		gptClient:              gptClient,
		firebaseAuth:           fbAuthClient,
		firebaseMsg:            fbMsgClient,
		emailService:           emailSvc,
		otpSendLimiter:         util.NewSlidingWindowRateLimiter(3, 15*time.Minute),
		otpVerifyLimiter:       util.NewSlidingWindowRateLimiter(5, 15*time.Minute),
		notificationService:    notificationSvc,
		reflectionService:      reflectionSvc,
		reflectionScheduler:    service.NewReflectionScheduler(store, reflectionSvc, notificationSvc, config.ReflectionTestMode),
		insightsService:        insightsSvc,
		parentInsightService:   parentInsightSvc,
		parentInsightScheduler: service.NewParentInsightScheduler(store, parentInsightSvc, notificationSvc),
		sessionHub:             NewSessionHub(),
	}

	SetupValidator()

	server.setupRouter()
	server.setupFrontendRoutes()

	// Start the background worker for session expiry
	server.startSessionExpiryWorker()

	// Start the daily reflection scheduler
	if err := server.reflectionScheduler.Start(config.ReflectionCronSchedule); err != nil {
		return nil, err
	}

	// Start the nightly parent insight scheduler
	if err := server.parentInsightScheduler.Start(config.ParentInsightCronSchedule); err != nil {
		return nil, err
	}

	return server, nil
}

func (server *Server) setupRouter() {
	router := gin.Default()

	// Auth routes
	v1 := router.Group("/v1")
	v1.POST("/auth/parent", server.signUpParent)
	v1.POST("/auth/parent/login", server.loginParent)
	v1.POST("/auth/parent/oauth/:provider/initiate", server.initiateOAuth)
	v1.GET("/auth/parent/oauth/poll/:session_id", server.pollOAuth)
	v1.GET("/auth/parent/oauth/:provider/start", server.oauthParentStart)
	v1.GET("/auth/parent/oauth/:provider/callback", server.oauthParentCallback)
	v1.POST("/auth/parent/oauth/:provider/process", server.processOAuthIdToken)
	v1.POST("/auth/parent/firebase", server.processFirebaseIdToken)

	// Password reset flow (public – no auth required)
	v1.POST("/auth/parent/forgot-password", server.forgotPassword)
	v1.POST("/auth/parent/resend-otp", server.resendOTP)
	v1.POST("/auth/parent/verify-otp", server.verifyOTP)
	v1.POST("/auth/parent/reset-password", server.resetPassword)

	// Public routes
	v1.POST("/child/link-code/verify", server.verifyLinkCode)

	// Notification routes - require any authenticated user
	notificationRoutes := router.Group("/v1/notifications")
	notificationRoutes.Use(server.simpleAuthMiddleware("parent", "child"))
	{
		notificationRoutes.POST("/device/register", server.registerDevice)
		notificationRoutes.GET("", server.getNotifications)
		notificationRoutes.GET("/summary", server.getNotificationSummary)
		notificationRoutes.POST("/read-all", server.markAllNotificationsAsRead)
		notificationRoutes.POST("/:id/read", server.markNotificationAsRead)
	}

	// Child routes - require device authentication
	childRoutes := router.Group("/v1/child")
	childRoutes.Use(server.deviceAuthMiddleware())
	{
		childRoutes.GET("/", server.getChild)
		childRoutes.POST("/logout", server.logoutChild)
		childRoutes.GET("/boosters", server.getThisWeeksBooster)
		childRoutes.POST("/booster/:booster_id/reflection", server.addReflectionVideo)
		childRoutes.GET("/booster/:booster_id/quiz", server.getBoosterQuiz)
		childRoutes.POST("/booster/:booster_id/quiz/submit", server.submitBoosterQuiz)
		childRoutes.GET("/:id/social-media", server.getSocialMediaForChild)
		childRoutes.PATCH("/", server.updateChildProfile)
		childRoutes.POST("/:id/quiz/:quiz_id/submit", server.submitQuizAnswers) // Note: This was misplaced before

		// Reflections
		childRoutes.GET("/reflections/pending", server.getPendingReflections)
		childRoutes.POST("/reflections/:id/respond", server.respondToReflection)
		childRoutes.POST("/reflections/:id/acknowledge", server.acknowledgeReflection)
		childRoutes.GET("/reflections/history", server.getReflectionHistory)
		childRoutes.GET("/reflections/stats", server.getReflectionStats)
		childRoutes.GET("/reflections/daily", server.getDailyReflection)

		// Course progress routes for child
		childRoutes.GET("/courses", server.getMyCoursesForChild)
		childRoutes.GET("/courses/stats", server.getMyCoursesStatsForChild)
	}

	// Activity tracking routes
	activityRoutes := router.Group("/v1/child/activity")
	activityRoutes.Use(server.deviceAuthMiddleware())
	{
		activityRoutes.POST("/ping", server.ping)
		activityRoutes.POST("/session/start", server.startSession)
		activityRoutes.POST("/session/end", server.endSession)
		activityRoutes.GET("/session/:id", server.getSessionStatus)
		activityRoutes.GET("/ws", server.handleSessionWS)
	}

	// Parent-facing routes that require authentication
	parentRoutes := router.Group("/v1/parent")
	parentRoutes.Use(server.authMiddleware("parent"))
	{
		parentRoutes.GET("/child/:id/activity/summary", server.getActivitySummary)
		parentRoutes.GET("/child/:id/activity/weekly-summary", server.getWeeklyActivitySummary)
		parentRoutes.POST("/child", server.createChild)
		parentRoutes.GET("/child/all", server.getAllChildren)
		parentRoutes.GET("/child/:id", server.getParentChild)
		parentRoutes.PATCH("/child/:id", server.updateChild)
		parentRoutes.DELETE("/child/:id", server.deleteChild)
		parentRoutes.GET("/child/:id/link-code", server.generateLinkCode)
		parentRoutes.GET("/child/:id/course/:course_id", server.getChildCourse)
		parentRoutes.GET("/child/:id/course/:course_id/module/:module_id", server.getChildCourseModule)
		parentRoutes.GET("/child/:id/course/:course_id/module/:module_id/quiz/:quiz_id", server.getQuizAttemptsForChild)
		parentRoutes.POST("/child/:id/course/:course_id/module/:module_id/quiz/:quiz_id/submit", server.submitQuizAnswers)
		parentRoutes.GET("/child/:id/course/latest", server.getLatestCourseForChild)
		parentRoutes.GET("/child/:id/courses", server.getAllCoursesForChild)
		parentRoutes.GET("/child/:id/courses/latest", server.getLatestCourseForChild)
		parentRoutes.GET("/child/:id/courses/stats", server.getCoursesStatsForChild)
		parentRoutes.GET("/courses", server.getAllCourses)
		parentRoutes.PATCH("/", server.updateParentProfile)
		parentRoutes.GET("/", server.getParentProfile)
		parentRoutes.GET("/child/:id/social-media", server.getParentSocialMediaSettings)
		parentRoutes.POST("/child/:id/social-media", server.setSocialMediaAccess)
		parentRoutes.GET("/child/:id/boosters", server.getBoostersForChildByParent)
		parentRoutes.GET("/child/:id/booster-reflections", server.getReflectionVideosForChild)
		parentRoutes.GET("/child/:id/digital-permit-test/ws", server.handleDigitalPermitTestWS)
		parentRoutes.GET("/child/:id/digital-permit-test/v2/ws", server.handleDigitalPermitTestWSV2)

		// Parent: view and trigger reflections for their child
		parentRoutes.GET("/child/:id/reflections", server.getChildReflectionsForParent)
		parentRoutes.POST("/child/:id/reflections/trigger", server.triggerReflectionForChild)

		// AI Insights routes
		parentRoutes.GET("/child/:id/insights/dashboard", server.getDashboardInsights)
		parentRoutes.GET("/child/:id/insights/engagement", server.getEngagementOverview)
		parentRoutes.GET("/child/:id/insights/content-monitoring", server.getContentMonitoringSummary)
		parentRoutes.POST("/child/:id/insights/content-monitoring/event", server.postContentMonitoringEvent)

		// Parent daily insights
		parentRoutes.GET("/child/:id/insights/daily", server.getParentDailyInsight)
		parentRoutes.GET("/child/:id/insights/daily/history", server.getParentDailyInsightHistory)
		parentRoutes.POST("/child/:id/insights/daily/:insight_id/read", server.markParentInsightRead)
	}

	server.router = router
}

// setupFrontendRoutes sets up the routes for the simple HTML test client.
func (server *Server) setupFrontendRoutes() {
	log.Info().Msg("setting up frontend test routes for development")
	frontend := server.router.Group("/frontend")
	frontend.GET("/test", func(ctx *gin.Context) {
		ctx.File("./frontend/test.html")
	})
	frontend.GET("/callback", func(ctx *gin.Context) {
		ctx.File("./frontend/callback.html")
	})
}

// Start runs the HTTP server on a specific address.
func (server *Server) Start(address string) error {
	server.httpServer = &http.Server{
		Addr:    address,
		Handler: server.router,
	}
	return server.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
func (server *Server) Shutdown(ctx context.Context) error {
	return server.httpServer.Shutdown(ctx)
}

// StopScheduler stops the reflection cron scheduler.
func (server *Server) StopScheduler() {
	if server.reflectionScheduler != nil {
		server.reflectionScheduler.Stop()
	}
}

func errorResponse(err error) gin.H {
	return gin.H{"error": err.Error()}
}

// initFirebaseApp initialises both a Firebase Auth client and a Firebase Messaging client.
// It prefers FIREBASE_SERVICE_ACCOUNT_JSON_FILE (a path to the JSON file on disk), then
// FIREBASE_SERVICE_ACCOUNT_JSON_BASE64 (base64-encoded JSON, safe for env vars), then
// FIREBASE_SERVICE_ACCOUNT_JSON (raw inline JSON, dev only). Returns (nil, nil, nil) when
// none is set so that local development environments still start up without Firebase.
func initFirebaseApp(ctx context.Context, config util.Config) (*firebaseAuth.Client, *firebaseMessaging.Client, error) {
	var opt option.ClientOption

	switch {
	case config.FirebaseServiceAccountJSONFile != "":
		// Option 1: load from file (no escaping issues).
		log.Info().Str("path", config.FirebaseServiceAccountJSONFile).Msg("loading Firebase credentials from file")
		opt = option.WithCredentialsFile(config.FirebaseServiceAccountJSONFile)
	case config.FirebaseServiceAccountJSONBase64 != "":
		// Option 2: base64-encoded JSON — safe to store in systemd env files.
		decoded, err := base64.StdEncoding.DecodeString(config.FirebaseServiceAccountJSONBase64)
		if err != nil {
			return nil, nil, fmt.Errorf("firebase: failed to base64-decode credentials: %w", err)
		}
		opt = option.WithCredentialsJSON(decoded)
	case config.FirebaseServiceAccountJSON != "":
		// Option 3: raw inline JSON (dev only — newlines are corrupted by systemd).
		opt = option.WithCredentialsJSON([]byte(config.FirebaseServiceAccountJSON))
	default:
		log.Warn().Msg("no Firebase credentials configured – Firebase disabled")
		return nil, nil, nil
	}
	app, err := firebaseAdmin.NewApp(ctx, &firebaseAdmin.Config{
		ProjectID: config.FirebaseProjectID,
	}, opt)
	if err != nil {
		return nil, nil, fmt.Errorf("firebase.NewApp: %w", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("firebase app.Auth: %w", err)
	}

	msgClient, err := app.Messaging(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("firebase app.Messaging: %w", err)
	}

	log.Info().Str("project_id", config.FirebaseProjectID).Msg("Firebase Auth and Messaging clients initialised")
	return authClient, msgClient, nil
}

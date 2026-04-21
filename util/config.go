package util

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config stores all configuration of the application.
// The values are read by viper from the config file or environment variable.
type Config struct {
	Environment                string        `mapstructure:"ENVIRONMENT"`
	AllowedOrigins             []string      `mapstructure:"ALLOWED_ORIGINS"`
	DBDriver                   string        `mapstructure:"DB_DRIVER"`
	DBSource                   string        `mapstructure:"DB_SOURCE"`
	MigrationURL               string        `mapstructure:"MIGRATION_URL"`
	RedisAddress               string        `mapstructure:"REDIS_ADDRESS"`
	HTTPServerAddress          string        `mapstructure:"HTTP_SERVER_ADDRESS"`
	ServerAddress              string        `mapstructure:"SERVER_ADDRESS"`
	TokenSymmetricKey          string        `mapstructure:"TOKEN_SYMMETRIC_KEY"`
	AccessTokenDuration        time.Duration `mapstructure:"ACCESS_TOKEN_DURATION"`
	RefreshTokenDuration       time.Duration `mapstructure:"REFRESH_TOKEN_DURATION"`
	EmailSenderName            string        `mapstructure:"EMAIL_SENDER_NAME"`
	EmailSenderAddress         string        `mapstructure:"EMAIL_SENDER_ADDRESS"`
	EmailSenderPassword        string        `mapstructure:"EMAIL_SENDER_PASSWORD"`
	GoogleOauthClientID        string        `mapstructure:"GOOGLE_OAUTH_CLIENT_ID"`
	GoogleOauthClientSecret    string        `mapstructure:"GOOGLE_OAUTH_CLIENT_SECRET"`
	AppleTeamID                string        `mapstructure:"APPLE_TEAM_ID"`
	AppleKeyID                 string        `mapstructure:"APPLE_KEY_ID"`
	ApplePrivateKey            string        `mapstructure:"APPLE_PRIVATE_KEY"`
	AppleClientID              string        `mapstructure:"APPLE_CLIENT_ID"`
	OauthRedirectBaseURL       string        `mapstructure:"OAUTH_REDIRECT_BASE_URL"`
	ParentPasswordAuthDisabled bool          `mapstructure:"PARENT_PASSWORD_AUTH_DISABLED"`

	// External content provider (agnostic)
	ExternalContentBaseURL string        `mapstructure:"EXTERNAL_CONTENT_BASE_URL"`
	ExternalContentToken   string        `mapstructure:"EXTERNAL_CONTENT_TOKEN"`
	ExternalRequestTimeout time.Duration `mapstructure:"EXTERNAL_REQUEST_TIMEOUT"`
	OpenAIAPIKey           string        `mapstructure:"OPENAI_API_KEY"`

	// Firebase Admin SDK
	FirebaseProjectID          string `mapstructure:"FIREBASE_PROJECT_ID"`
	FirebaseServiceAccountJSON string `mapstructure:"FIREBASE_SERVICE_ACCOUNT_JSON"` // raw JSON string of the service account key

	// Reflection scheduler (standard 5-field cron expression)
	ReflectionCronSchedule string `mapstructure:"REFLECTION_CRON_SCHEDULE"`
	ReflectionTestMode     bool   `mapstructure:"REFLECTION_TEST_MODE"` // bypass idempotency for testing
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("app")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	if err = viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Config file was found but another error was produced
			return
		}
	}

	// Bind environment variables to ensure they are loaded
	viper.BindEnv("ENVIRONMENT")
	viper.BindEnv("ALLOWED_ORIGINS")
	viper.BindEnv("DB_DRIVER")
	viper.BindEnv("DB_SOURCE")
	viper.BindEnv("MIGRATION_URL")
	viper.BindEnv("REDIS_ADDRESS")
	viper.BindEnv("HTTP_SERVER_ADDRESS")
	viper.BindEnv("SERVER_ADDRESS")
	viper.BindEnv("TOKEN_SYMMETRIC_KEY")
	viper.BindEnv("ACCESS_TOKEN_DURATION")
	viper.BindEnv("REFRESH_TOKEN_DURATION")
	viper.BindEnv("EMAIL_SENDER_NAME")
	viper.BindEnv("EMAIL_SENDER_ADDRESS")
	viper.BindEnv("EMAIL_SENDER_PASSWORD")
	viper.BindEnv("GOOGLE_OAUTH_CLIENT_ID")
	viper.BindEnv("GOOGLE_OAUTH_CLIENT_SECRET")
	viper.BindEnv("APPLE_TEAM_ID")
	viper.BindEnv("APPLE_KEY_ID")
	viper.BindEnv("APPLE_PRIVATE_KEY")
	viper.BindEnv("OAUTH_REDIRECT_BASE_URL")
	viper.BindEnv("PARENT_PASSWORD_AUTH_DISABLED")
	viper.BindEnv("EXTERNAL_CONTENT_BASE_URL")
	viper.BindEnv("EXTERNAL_CONTENT_TOKEN")
	viper.BindEnv("EXTERNAL_REQUEST_TIMEOUT")
	viper.BindEnv("OPENAI_API_KEY")
	viper.BindEnv("ONESIGNAL_APP_ID")
	viper.BindEnv("ONESIGNAL_API_KEY")
	viper.BindEnv("FIREBASE_PROJECT_ID")
	viper.BindEnv("FIREBASE_SERVICE_ACCOUNT_JSON")

	err = viper.Unmarshal(&config)
	fmt.Println(config)
	return
}

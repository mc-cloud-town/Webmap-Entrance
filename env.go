package main

import (
	"log"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

type appConfig struct {
	DiscordClientID     string
	DiscordToken        string
	DiscordClientSecret string
	DiscordRedirectURI  string
	DiscordGuildID      string
	DiscordRolesID      []string
	TargetURL           string
	SessionSecret       string
	DevMode             bool
}

var cfg appConfig
var oauthCfg *oauth2.Config

func mustEnv(key string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}

func loadConfig() {
	env := os.Getenv("GO_ENV")
	devMode := false
	if env == "" || env == "development" {
		_ = godotenv.Load()
		devMode = true
	}

	cfg = appConfig{
		DiscordClientID:     mustEnv("DISCORD_CLIENT_ID"),
		DiscordToken:        mustEnv("DISCORD_TOKEN"),
		DiscordClientSecret: mustEnv("DISCORD_CLIENT_SECRET"),
		DiscordRedirectURI:  mustEnv("DISCORD_REDIRECT_URI"),
		DiscordGuildID:      "933290709589577728",
		DiscordRolesID: []string{
			"933382711148695673",  // 雲鎮伙伴-member
			"1049504039211118652", // 二審中-trialing
		},
		TargetURL:     mustEnv("TARGET_URL"),
		SessionSecret: mustEnv("SESSION_SECRET"),
		DevMode:       devMode,
	}

	u, _ := url.Parse(cfg.DiscordRedirectURI)
	u.Path = path.Join(u.Path, "$__hook_sess___/callback")
	oauthCfg = &oauth2.Config{
		ClientID:     cfg.DiscordClientID,
		ClientSecret: cfg.DiscordClientSecret,
		RedirectURL:  u.String(),
		Scopes:       []string{"identify", "guilds"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://discord.com/api/oauth2/authorize",
			TokenURL: "https://discord.com/api/oauth2/token",
		},
	}
}

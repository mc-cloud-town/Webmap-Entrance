package main

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

//go:embed pages/index.html
var indexHTML []byte

//go:embed pages/403.html
var forbiddenHTML []byte

//go:embed pages/500.html
var internalErrorHTML []byte

//go:embed pages/502.html
var badGatewayHTML []byte

//go:embed static/*
var staticFS embed.FS

var permCache = cache.New(5*time.Minute, 10*time.Minute)

func initLogger() {
	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	if os.Getenv("ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	} else {
		log.Logger = log.Output(os.Stdout)
	}
}

func randState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func newReverseProxy(target string) *httputil.ReverseProxy {
	u, err := url.Parse(target)
	if err != nil {
		log.Fatal().Err(err).Msg("invalid proxy target")
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = u.Host
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error().Err(err).Str("path", r.URL.Path).Msg("Proxy error")
		w.WriteHeader(http.StatusBadGateway)
		w.Write(badGatewayHTML)
	}

	proxy.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return proxy
}

func checkMemberPermission(bot *discordgo.Session, userID string) (bool, error) {
	if val, found := permCache.Get(userID); found {
		return val.(bool), nil
	}

	member, err := bot.State.Member(cfg.DiscordGuildID, userID)
	if err != nil || member == nil {
		member, err = bot.GuildMember(cfg.DiscordGuildID, userID)
		if err != nil {
			log.Warn().Err(err).Str("user_id", userID).Msg("Failed to fetch guild member")
			permCache.Set(userID, false, cache.DefaultExpiration)

			// User is not a member of the guild
			return false, nil
		}
	}

	hasRole := false
	for _, roleID := range cfg.DiscordRolesID {
		if slices.Contains(member.Roles, roleID) {
			hasRole = true
			break
		}
	}

	permCache.Set(userID, hasRole, cache.DefaultExpiration)
	return hasRole, nil
}

func authMiddleware(bot *discordgo.Session) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userID := session.Get("user_id")

		if userID == nil {
			if c.Request.URL.Path == "/" {
				c.Data(200, "text/html; charset=utf-8", indexHTML)
			} else {
				c.Redirect(302, "/")
			}
			c.Abort()
			return
		}

		allowed, err := checkMemberPermission(bot, userID.(string))
		if err != nil {
			log.Error().Err(err).Str("user_id", userID.(string)).Msg("Failed to check Discord membership")
			c.Data(500, "text/html; charset=utf-8", internalErrorHTML)
			c.Abort()
			return
		}

		if !allowed {
			log.Warn().Str("user_id", userID.(string)).Msg("User lacks required roles")
			c.Data(403, "text/html; charset=utf-8", forbiddenHTML)
			c.Abort()
			return
		}

		c.Next()
	}
}

func main() {
	initLogger()
	loadConfig()

	if cfg.DevMode {
		log.Info().Msg("Running in development mode")
	} else {
		log.Info().Msg("Running in production mode")
		gin.SetMode(gin.ReleaseMode)
	}

	bot, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Discord session")
	}

	bot.Identify.Intents = discordgo.IntentsGuildMembers
	if err := bot.Open(); err != nil {
		log.Error().Err(err).Msg("Failed to open Discord websocket connection, falling back to REST API only mode")
	}
	defer bot.Close()

	proxy := newReverseProxy(cfg.TargetURL)

	r := gin.Default()
	r.TrustedPlatform = gin.PlatformCloudflare
	store := cookie.NewStore([]byte(cfg.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   !cfg.DevMode, // HTTPS only in prod
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("hook_sess", store))

	r.Use(func(c *gin.Context) {
		reqID := randState()
		c.Set("req_id", reqID)
		start := time.Now()

		c.Next()

		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("duration", time.Since(start)).
			Str("req_id", reqID).
			Msg("Handled request")
	})

	r.GET("/", authMiddleware(bot), func(ctx *gin.Context) {
		proxy.ServeHTTP(ctx.Writer, ctx.Request)
	})

	r.GET("/$__hook_sess___/login", func(ctx *gin.Context) {
		state := randState()
		session := sessions.Default(ctx)
		session.Set("oauth_state", state)
		session.Save()

		ctx.Redirect(302, oauthCfg.AuthCodeURL(state))
	})

	r.GET("/$__hook_sess___/callback", func(ctx *gin.Context) {
		session := sessions.Default(ctx)
		expectedState := session.Get("oauth_state")
		if expectedState == nil || expectedState != ctx.Query("state") {
			ctx.String(400, "invalid oauth state")
			return
		}

		session.Delete("oauth_state")

		token, err := oauthCfg.Exchange(ctx, ctx.Query("code"))
		if err != nil {
			log.Error().Err(err).Msg("OAuth token exchange failed")
			ctx.String(500, "token exchange failed")
			return
		}

		client := oauthCfg.Client(ctx, token)
		resp, err := client.Get("https://discord.com/api/users/@me")
		if err != nil || resp.StatusCode != 200 {
			log.Error().Err(err).Int("status_code", resp.StatusCode).Msg("Failed to fetch user info")
			ctx.String(502, "failed to fetch user info")
			return
		}
		defer resp.Body.Close()

		var user struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
			log.Error().Err(err).Msg("Failed to decode user info")
			ctx.String(500, "failed to decode user info")
			return
		}

		session.Set("user_id", user.ID)
		session.Save()

		ctx.Redirect(302, "/")
	})

	subFS, _ := fs.Sub(staticFS, "static")
	r.StaticFS("/$__hook_sess___/static", http.FS(subFS))

	r.NoRoute(
		authMiddleware(bot),
		func(ctx *gin.Context) {
			proxy.ServeHTTP(ctx.Writer, ctx.Request)
		},
	)

	log.Info().Msg("Listening on :3000")
	r.Run(":3000")
}

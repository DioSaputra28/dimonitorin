package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DioSaputra28/monitor-vps/internal/app"
	"github.com/DioSaputra28/monitor-vps/internal/auth"
	"github.com/DioSaputra28/monitor-vps/internal/config"
	"github.com/DioSaputra28/monitor-vps/internal/db"
	"github.com/DioSaputra28/monitor-vps/internal/monitor"
	"github.com/DioSaputra28/monitor-vps/internal/views"
	staticassets "github.com/DioSaputra28/monitor-vps/static"
	"github.com/a-h/templ"
)

type Server struct {
	cfg       config.Config
	store     *db.Store
	collector *monitor.Collector
	sampler   monitor.Sampler
	sessions  *auth.SessionManager
	protector *auth.LoginProtector
}

type sessionContextKey struct{}

type Session struct {
	ID        string
	Username  string
	CSRFToken string
	Theme     string
}

func New(cfg config.Config, store *db.Store, collector *monitor.Collector) *Server {
	return &Server{
		cfg:       cfg,
		store:     store,
		collector: collector,
		sampler:   monitor.Sampler{Collector: collector},
		sessions:  auth.NewSessionManager(cfg.Auth.CookieSecret),
		protector: auth.NewLoginProtector(),
	}
}

func (s *Server) Routes() (http.Handler, error) {
	mux := http.NewServeMux()
	sub, err := fs.Sub(staticassets.FS, ".")
	if err != nil {
		return nil, err
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("/theme", s.requireAuth(s.handleTheme))
	mux.HandleFunc("/", s.requireAuth(s.handleDashboard))
	mux.HandleFunc("/services", s.requireAuth(s.handleServices))
	mux.HandleFunc("/history", s.requireAuth(s.handleHistory))
	mux.HandleFunc("/settings", s.requireAuth(s.handleSettings))
	mux.HandleFunc("/partials/summary", s.requireAuth(s.handleSummaryPartial))
	mux.HandleFunc("/partials/services", s.requireAuth(s.handleServicesPartial))
	mux.HandleFunc("/partials/processes", s.requireAuth(s.handleProcessesPartial))
	mux.HandleFunc("/partials/events", s.requireAuth(s.handleEventsPartial))
	mux.HandleFunc("/chart/network", s.requireAuth(s.handleChartNetwork))
	return mux, nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.currentSession(r.Context()); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		token := s.ensureLoginCSRF(w, r)
		s.renderTempl(w, r, views.LoginPage(app.LoginPageData{Theme: s.cfg.UI.DefaultTheme, CSRFToken: token}))
	case http.MethodPost:
		if !s.validLoginCSRF(r) {
			s.renderTempl(w, r, views.LoginPage(app.LoginPageData{Theme: s.cfg.UI.DefaultTheme, CSRFToken: s.ensureLoginCSRF(w, r), Error: "Invalid CSRF token"}))
			return
		}
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		ip := auth.ClientIP(r)
		if err := s.protector.Check(ip, time.Now()); err != nil {
			s.renderTempl(w, r, views.LoginPage(app.LoginPageData{Theme: s.cfg.UI.DefaultTheme, CSRFToken: s.ensureLoginCSRF(w, r), Error: err.Error()}))
			return
		}
		hash, err := s.store.GetAdminHash(r.Context(), username)
		if err != nil || !auth.ComparePassword(hash, password) {
			s.protector.Fail(ip, time.Now())
			s.renderTempl(w, r, views.LoginPage(app.LoginPageData{Theme: s.cfg.UI.DefaultTheme, CSRFToken: s.ensureLoginCSRF(w, r), Error: "Invalid username or password"}))
			return
		}
		s.protector.Success(ip)
		sessionID, _ := auth.RandomToken()
		csrfToken, _ := auth.RandomToken()
		theme := s.cfg.UI.DefaultTheme
		if err := s.store.CreateSession(r.Context(), sessionID, username, csrfToken, theme, time.Now().Add(24*time.Hour)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = s.store.AddAudit(r.Context(), "login", username, "Admin login successful")
		s.sessions.Set(w, sessionID, s.secureCookie(r))
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.verifyCSRF(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if session, ok := s.currentSession(r.Context()); ok {
		_ = s.store.DeleteSession(r.Context(), session.ID)
		_ = s.store.AddAudit(r.Context(), "logout", session.Username, "Admin logout")
	}
	s.sessions.Clear(w, s.secureCookie(r))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.verifyCSRF(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	session, ok := s.currentSession(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	theme := r.FormValue("theme")
	if theme != "light" {
		theme = "dark"
	}
	if err := s.store.UpdateSessionTheme(r.Context(), session.ID, theme); err == nil {
		_ = s.store.AddAudit(r.Context(), "theme_change", session.Username, "Theme set to "+theme)
	}
	back := r.Referer()
	if back == "" {
		back = "/"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	snapshot, events, session, err := s.pageState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTempl(w, r, views.DashboardPage(app.DashboardData{
		Snapshot:        snapshot,
		Events:          events,
		Theme:           session.Theme,
		CSRFToken:       session.CSRFToken,
		ChartRanges:     []string{"1h", "6h", "24h"},
		SelectedRange:   "1h",
		RefreshSummary:  "10s",
		RefreshServices: "15s",
		RefreshEvents:   "15s",
		RefreshChart:    "20s",
	}))
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	snapshot, events, session, err := s.pageState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTempl(w, r, views.ServicesPage(app.ServicesPageData{Snapshot: snapshot, Events: events, Theme: session.Theme, CSRFToken: session.CSRFToken}))
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	snapshot, _, session, err := s.pageState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rng := normalizeRange(r.URL.Query().Get("range"))
	s.renderTempl(w, r, views.HistoryPage(app.HistoryPageData{Snapshot: snapshot, Theme: session.Theme, CSRFToken: session.CSRFToken, SelectedRange: rng, Available: []string{"1h", "6h", "24h"}}))
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	snapshot, _, session, err := s.pageState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := map[string]string{
		"server.host":               s.cfg.Server.Host,
		"server.port":               fmt.Sprintf("%d", s.cfg.Server.Port),
		"sampling.interval":         s.cfg.Sampling.Interval.String(),
		"retention.days":            fmt.Sprintf("%d", s.cfg.Retention.Days),
		"ui.default_theme":          s.cfg.UI.DefaultTheme,
		"monitor.primary_interface": s.cfg.Monitor.PrimaryInterface,
		"monitor.tracked_services":  strings.Join(s.cfg.Monitor.TrackedServices, ", "),
		"paths.database":            s.cfg.Paths.Database,
		"security.trusted_proxies":  strings.Join(s.cfg.Security.TrustedProxies, ", "),
	}
	s.renderTempl(w, r, views.SettingsPage(app.SettingsPageData{Hostname: snapshot.Hostname, Theme: session.Theme, CSRFToken: session.CSRFToken, Config: cfg}))
}

func (s *Server) handleSummaryPartial(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.latestSnapshot(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTempl(w, r, views.SummaryPartial(snapshot))
}

func (s *Server) handleServicesPartial(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.latestSnapshot(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTempl(w, r, views.ServicesPartial(snapshot))
}

func (s *Server) handleProcessesPartial(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.latestSnapshot(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTempl(w, r, views.ProcessesPartial(snapshot))
}

func (s *Server) handleEventsPartial(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.RecentEvents(r.Context(), 8)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTempl(w, r, views.EventsPartial(events))
}

func (s *Server) handleChartNetwork(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-rangeDuration(normalizeRange(r.URL.Query().Get("range"))))
	snapshots, err := s.store.History(r.Context(), since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type chartPayload struct {
		Series []app.ChartSeries `json:"series"`
	}
	payload := chartPayload{Series: []app.ChartSeries{{Name: "Download", Points: make([]app.MetricPoint, 0, len(snapshots))}, {Name: "Upload", Points: make([]app.MetricPoint, 0, len(snapshots))}}}
	for _, snap := range snapshots {
		payload.Series[0].Points = append(payload.Series[0].Points, app.MetricPoint{Timestamp: snap.CollectedAt, Value: snap.Network.RXBytesPerS})
		payload.Series[1].Points = append(payload.Series[1].Points, app.MetricPoint{Timestamp: snap.CollectedAt, Value: snap.Network.TXBytesPerS})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) pageState(ctx context.Context) (app.HostSnapshot, []app.Event, Session, error) {
	snapshot, err := s.latestSnapshot(ctx)
	if err != nil {
		return app.HostSnapshot{}, nil, Session{}, err
	}
	events, err := s.store.RecentEvents(ctx, 8)
	if err != nil {
		return app.HostSnapshot{}, nil, Session{}, err
	}
	session, _ := s.currentSession(ctx)
	return snapshot, events, session, nil
}

func (s *Server) latestSnapshot(ctx context.Context) (app.HostSnapshot, error) {
	snapshot, err := s.store.LatestSample(ctx)
	if err == nil {
		return snapshot, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return app.HostSnapshot{}, err
	}
	snapshot = s.collector.Collect(ctx)
	if insertErr := s.store.InsertSample(ctx, snapshot); insertErr != nil {
		return app.HostSnapshot{}, insertErr
	}
	return snapshot, nil
}

func (s *Server) renderTempl(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := s.sessions.Read(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		username, csrfToken, theme, expiresAt, err := s.store.GetSession(r.Context(), sessionID)
		if err != nil || time.Now().After(expiresAt) {
			s.sessions.Clear(w, s.secureCookie(r))
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), sessionContextKey{}, Session{ID: sessionID, Username: username, CSRFToken: csrfToken, Theme: theme})
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) currentSession(ctx context.Context) (Session, bool) {
	session, ok := ctx.Value(sessionContextKey{}).(Session)
	return session, ok
}

func (s *Server) verifyCSRF(r *http.Request) bool {
	session, ok := s.currentSession(r.Context())
	if !ok {
		return false
	}
	return session.CSRFToken != "" && session.CSRFToken == r.FormValue("csrf_token")
}

func (s *Server) ensureLoginCSRF(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie("dimonitorin_login_csrf"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	token, _ := auth.RandomToken()
	http.SetCookie(w, &http.Cookie{Name: "dimonitorin_login_csrf", Value: token, Path: "/login", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.secureCookie(r)})
	return token
}

func (s *Server) validLoginCSRF(r *http.Request) bool {
	cookie, err := r.Cookie("dimonitorin_login_csrf")
	if err != nil {
		return false
	}
	return cookie.Value != "" && cookie.Value == r.FormValue("csrf_token")
}

func (s *Server) secureCookie(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func normalizeRange(v string) string {
	switch v {
	case "6h", "24h":
		return v
	default:
		return "1h"
	}
}

func rangeDuration(v string) time.Duration {
	switch v {
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}

func (s *Server) BootstrapSample(ctx context.Context) error {
	if _, err := s.latestSnapshot(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func (s *Server) StartBackground(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.Sampling.Interval)
	pruneTicker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	defer pruneTicker.Stop()

	var previous app.HostSnapshot
	current, err := s.latestSnapshot(ctx)
	if err == nil {
		previous = current
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snapshot := s.collector.Collect(ctx)
			if err := s.store.InsertSample(ctx, snapshot); err == nil {
				for _, event := range s.sampler.EmitEvents(previous, snapshot) {
					_ = s.store.AddEvent(ctx, event)
				}
				previous = snapshot
			}
		case <-pruneTicker.C:
			_ = s.store.Prune(ctx, s.cfg.Retention.Days)
			_ = s.store.PurgeExpiredSessions(ctx, time.Now())
		}
	}
}

func WriteReverseProxyExamples(appDir string) error {
	caddy := fmt.Sprintf("example.com {\n    reverse_proxy 127.0.0.1:8080\n}\n")
	nginx := `server {
    listen 443 ssl;
    server_name example.com;

    location / {
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://127.0.0.1:8080;
    }
}
`
	if err := os.WriteFile(appDir+"/reverse-proxy.caddy", []byte(caddy), 0o644); err != nil {
		return err
	}
	return os.WriteFile(appDir+"/reverse-proxy.nginx", []byte(nginx), 0o644)
}

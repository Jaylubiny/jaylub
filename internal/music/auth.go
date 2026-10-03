package music

import (
	"context"
	"net/http"
	"strconv"
)

type userContextKey struct{}

type User struct {
	ID       int64
	Username string
}

func withUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func userFromRequest(r *http.Request) (User, bool) {
	user, ok := r.Context().Value(userContextKey{}).(User)
	return user, ok && user.ID > 0
}

// MockAuthMiddleware is for local development and tests only. Production must
// provide an authentication middleware backed by the site's real session store.
func MockAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAssetPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		rawID := r.Header.Get("X-Music-User-ID")
		userID := int64(1)
		if rawID != "" {
			var err error
			userID, err = strconv.ParseInt(rawID, 10, 64)
			if err != nil || userID <= 0 {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		username := r.Header.Get("X-Music-Username")
		if username == "" {
			username = "Local user"
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), User{ID: userID, Username: username})))
	})
}

// SiteAuthMiddleware adapts the website's authenticated user context to music.
// The supplied middleware must reject unauthenticated requests before next runs.
func SiteAuthMiddleware(siteMiddleware func(http.Handler) http.Handler, next http.Handler, siteUser func(*http.Request) (int64, string, bool)) http.Handler {
	protected := siteMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAssetPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		userID, username, ok := siteUser(r)
		if !ok || userID <= 0 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), User{ID: userID, Username: username})))
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAssetPath(r.URL.Path) || r.URL.Path == "/login" || r.URL.Path == "/terms" {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

func isAssetPath(path string) bool {
	return path == "/assets/app.js" || path == "/assets/styles.css"
}

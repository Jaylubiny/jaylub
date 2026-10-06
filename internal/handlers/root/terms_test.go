package handlers

import (
	"html/template"
	"jaylub/internal/auth"
	"jaylub/internal/views"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestTermsHandlerRequiresExplicitAcceptance(t *testing.T) {
	service, err := auth.New(t.TempDir() + "/users.db")
	if err != nil {
		t.Fatal(err)
	}

	defer service.Close()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DB().Exec(
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		"terms-user",
		string(passwordHash),
	); err != nil {
		t.Fatal(err)
	}

	loginRequest := httptest.NewRequest(http.MethodPost, "https://jaylub.com/login", nil)
	loginResponse := httptest.NewRecorder()
	if err := service.Login(loginResponse, loginRequest, "terms-user", "password"); err != nil {
		t.Fatal(err)
	}
	sessionCookie := loginResponse.Result().Cookies()[0]
	handler := service.Middleware(Terms(service))

	for _, test := range []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "checkbox missing", body: "", wantStatus: http.StatusBadRequest},
		{name: "accepted", body: "accept_terms=yes", wantStatus: http.StatusSeeOther},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost,
				"https://jaylub.com/terms",
				strings.NewReader(test.body),
			)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.AddCookie(sessionCookie)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.name == "checkbox missing" && len(response.Result().Cookies()) != 0 {
				t.Fatal("terms cookie was issued without explicit acceptance")
			}
			if test.name == "accepted" {
				cookies := response.Result().Cookies()
				if len(cookies) != 1 || cookies[0].Name != "jaylub_terms_device" ||
					!cookies[0].HttpOnly || !cookies[0].Secure ||
					cookies[0].SameSite != http.SameSiteLaxMode ||
					cookies[0].Path != "/" || cookies[0].Domain != "jaylub.com" ||
					cookies[0].MaxAge < 364*24*60*60 {
					t.Fatalf("device terms cookie attributes = %+v", cookies)
				}
				checkRequest := httptest.NewRequest(http.MethodGet, "https://jaylub.com/chat", nil)
				checkRequest.AddCookie(cookies[0])
				accepted, err := service.DeviceTermsAccepted(checkRequest)
				if err != nil || !accepted {
					t.Fatalf("accepted device cookie validation = (%v, %v)", accepted, err)
				}
			}
		})
	}
	var accountAcceptances int
	if err := service.DB().QueryRow(`SELECT COUNT(*) FROM terms_acceptances`).Scan(&accountAcceptances); err != nil {
		t.Fatal(err)
	}
	if accountAcceptances != 0 {
		t.Errorf("device acceptance unexpectedly recorded %d account-wide acceptance rows", accountAcceptances)
	}
}

func TestTermsTemplateRendersAgreementAndRequiredCheckbox(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source")
	}
	projectRoot := filepath.Join(filepath.Dir(sourceFile), "..", "..", "..")
	tmpl, err := template.ParseFiles(
		filepath.Join(projectRoot, "web/templates/layout.html"),
		filepath.Join(projectRoot, "web/templates/terms.html"),
	)
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	data := views.PageData{Title: "terms", TermsVersion: auth.CurrentTermsVersion}
	if err := tmpl.ExecuteTemplate(response, "layout.html", data); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"<h1>TERMS AND CONDITIONS</h1>",
		"Keep the website and its content private",
		"Cookies and device-level terms acceptance",
		"jaylub_session",
		"jaylub_terms_device",
		"up to 30 days",
		"expires after one year",
		"This identifies a browser, not a physical device.",
		"Version " + auth.CurrentTermsVersion,
		`name="accept_terms" value="yes" required`,
		"Accept and continue",
		"Log out instead",
		"These terms do not create an automatic monetary fine.",
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("rendered terms page is missing %q", expected)
		}
	}
	if strings.Contains(response.Body.String(), "Before you continue") {
		t.Error("terms page still contains the removed introductory label")
	}
}

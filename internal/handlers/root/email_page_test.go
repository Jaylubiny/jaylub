package handlers

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"testing"

	"jaylub/internal/views"
)

func TestEmailTemplateRendersSharedLayoutAndMailboxAddress(t *testing.T) {
	templateDir := filepath.Join("..", "..", "..", "web", "templates")
	tmpl, err := template.ParseFiles(
		filepath.Join(templateDir, "layout.html"),
		filepath.Join(templateDir, "email.html"),
	)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	data := views.PageData{Title: "email", Username: "alice", Initials: "A"}
	if err := tmpl.ExecuteTemplate(&output, "layout.html", data); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"href=\"/me\">Stats</a>",
		"alice@jaylub.com",
		"email-list",
		"email-reader",
		"compose-modal",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("rendered email page does not include %q", want)
		}
	}
	for _, unwanted := range []string{
		`<a href="/">Home</a>`,
		`<a href="/about">About</a>`,
		`<a href="/contacts">Contacts</a>`,
		`<a href="/documentation">Documentation</a>`,
	} {
		if strings.Contains(output.String(), unwanted) {
			t.Errorf("email page unexpectedly includes shared navigation link %q", unwanted)
		}
	}
}

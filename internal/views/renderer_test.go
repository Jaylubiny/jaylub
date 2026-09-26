package views

import (
	"encoding/json"
	"html/template"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPopulateSEO(t *testing.T) {
	t.Parallel()

	seo := SEOData{
		Title:               "Jaylub",
		Description:         "Projects, chat, and games.",
		CanonicalURL:        "https://jaylub.com/",
		OpenGraphType:       "website",
		ApplicationName:     "Jaylub",
		ApplicationCategory: "WebApplication",
		Features:            []string{"Game", "Chat", "Wiki", "Documentation"},
	}
	data := PageData{Title: "home"}
	if err := populateSEO(&data, seo); err != nil {
		t.Fatalf("populateSEO() error = %v", err)
	}

	if data.MetaTitle != seo.Title || data.MetaDescription != seo.Description {
		t.Fatalf("metadata = (%q, %q), want (%q, %q)", data.MetaTitle, data.MetaDescription, seo.Title, seo.Description)
	}
	if data.CanonicalURL != seo.CanonicalURL || data.OpenGraphURL != seo.CanonicalURL {
		t.Fatalf("canonical/OpenGraph URLs = (%q, %q), want %q", data.CanonicalURL, data.OpenGraphURL, seo.CanonicalURL)
	}
	if !data.Indexable || data.StructuredData == "" {
		t.Fatal("SEO-enabled page should be indexable and include structured data")
	}

	var schema softwareApplicationSchema
	if err := json.Unmarshal([]byte(data.StructuredData), &schema); err != nil {
		t.Fatalf("structured data is invalid JSON: %v", err)
	}
	if schema.Context != "https://schema.org" || schema.Type != "SoftwareApplication" {
		t.Errorf("schema identity = (%q, %q)", schema.Context, schema.Type)
	}
	if schema.Name != seo.ApplicationName || schema.Description != seo.Description || schema.ApplicationCategory != seo.ApplicationCategory {
		t.Errorf("schema metadata does not match SEO configuration: %#v", schema)
	}
	if len(schema.FeatureList) < 4 || len(schema.FeatureList) > 6 {
		t.Errorf("featureList length = %d, want 4-6", len(schema.FeatureList))
	}
}

func TestHomepageTemplateRendersMetadataAndStructuredData(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source")
	}
	templateDir := filepath.Join(filepath.Dir(sourceFile), "..", "..", "web", "templates")
	tmpl, err := template.ParseFiles(
		filepath.Join(templateDir, "layout.html"),
		filepath.Join(templateDir, "home.html"),
	)
	if err != nil {
		t.Fatalf("parse homepage templates: %v", err)
	}

	data := PageData{Title: "home"}
	if err := populateSEO(&data, SEOData{
		Title:               "Jaylub",
		Description:         "Platform description",
		CanonicalURL:        "https://jaylub.com/",
		OpenGraphType:       "website",
		ApplicationName:     "Jaylub",
		ApplicationCategory: "WebApplication",
		Features:            []string{"Game", "Chat", "Wiki", "Docs"},
	}); err != nil {
		t.Fatalf("populate SEO data: %v", err)
	}

	response := httptest.NewRecorder()
	if err := tmpl.ExecuteTemplate(response, "layout.html", data); err != nil {
		t.Fatalf("execute homepage template: %v", err)
	}
	html := response.Body.String()
	for _, fragment := range []string{
		"<title>Jaylub</title>",
		`<meta name="description" content="Platform description">`,
		`<link rel="canonical" href="https://jaylub.com/">`,
		`<meta property="og:title" content="Jaylub">`,
		`<meta property="og:description" content="Platform description">`,
		`<meta property="og:type" content="website">`,
		`<meta property="og:url" content="https://jaylub.com/">`,
		`<meta name="robots" content="index, follow">`,
		`<script type="application/ld+json">{"@context":"https://schema.org","@type":"SoftwareApplication"`,
	} {
		if !strings.Contains(html, fragment) {
			t.Errorf("rendered homepage is missing %q", fragment)
		}
	}
}

func TestPopulateSEOLeavesUnconfiguredPagesUnindexable(t *testing.T) {
	t.Parallel()

	data := PageData{Title: "settings"}
	if err := populateSEO(&data, SEOData{}); err != nil {
		t.Fatalf("populateSEO() error = %v", err)
	}
	if data.Indexable || data.StructuredData != "" {
		t.Fatalf("unconfigured page received indexable SEO metadata: %#v", data)
	}
}

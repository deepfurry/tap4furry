package publicreadcheck

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"github.com/gofiber/fiber/v3"
)

const cache = "no-store"

// Request verifies the actual serialized response, including nested privacy.
func Request(app *fiber.App, path string, status int, target any) error {
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Origin", "https://unrelated.example.invalid")
	r.Header.Set("Cookie", "tap4furry_session=ignored-fixture")
	r.Header.Set("Authorization", "Bearer ignored-fixture")
	r.Header.Set("X-CSRF-Token", "ignored-fixture")
	response, err := app.Test(r, fiber.TestConfig{Timeout: 7 * time.Second})
	if err != nil {
		return errors.New("public HTTP fixture request failed (details withheld)")
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		return errors.New("public HTTP fixture status differs")
	}
	wantCache := "no-store"
	if status == 200 {
		wantCache = cache
	}
	if response.Header.Get("Cache-Control") != wantCache || response.Header.Get("Set-Cookie") != "" {
		return errors.New("public cache/session contract differs")
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return errors.New("public fixture body unavailable")
	}
	var raw any
	if json.Unmarshal(body, &raw) != nil {
		return errors.New("public fixture is not JSON")
	}
	if !privateKeysAbsent(raw) {
		return errors.New("public DTO leaked a private key")
	}
	if target != nil && json.Unmarshal(body, target) != nil {
		return errors.New("public DTO shape differs")
	}
	return nil
}

func privateKeysAbsent(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			switch key {
			case "publication_state", "version", "deleted_at", "rights_status", "created_at", "resource_id":
				return false
			}
			if !privateKeysAbsent(child) {
				return false
			}
		}
	case []any:
		for _, child := range value {
			if !privateKeysAbsent(child) {
				return false
			}
		}
	}
	return true
}

func (f *Fixture) Verify(app *fiber.App) error {
	for _, name := range []string{"draft", "pending", "restricted", "removed", "deleted", "deleted-category"} {
		var body generated.ApiError
		if err := Request(app, "/resources/"+f.Slug(name), 404, &body); err != nil {
			return err
		}
		if body.Code != generated.RESOURCENOTFOUND {
			return errors.New("hidden resource existence exposed")
		}
	}
	for _, query := range []string{"", "?locale=JA", "?locale=fr"} {
		var body generated.ResourceDetail
		if err := Request(app, "/resources/"+f.Slug("public")+query, 200, &body); err != nil {
			return err
		}
		name := "Default resource public"
		category := "Default categories active"
		if query == "?locale=JA" {
			name = "Requested resource public"
			category = "Requested categories active"
		}
		if body.Name != name || body.Category.Name != category || body.Summary == nil || *body.Summary != "Default summary" || body.Description == nil || !strings.Contains(*body.Description, "**community**") || body.DefaultLocale != "en" || len(body.AvailableLocales) != 2 {
			return errors.New("public locale field fallback differs")
		}
		if query == "" && body.RequestedLocale != nil || query == "?locale=JA" && (body.RequestedLocale == nil || *body.RequestedLocale != "ja") {
			return errors.New("public requested locale semantics differ")
		}
		if body.ContentRating != "explicit" || body.Lifecycle != "discontinued" || len(body.Tags) != 2 || len(body.Sources) != 12 || len(body.Relations) != 5 || len(body.ExternalIds) != 1 {
			return errors.New("public graph filtering differs")
		}
		for _, tag := range body.Tags {
			if tag.Id == f.Tags["deleted"].String() {
				return errors.New("deleted tag leaked")
			}
		}
		seen := map[string]bool{}
		for _, source := range body.Sources {
			if !f.VisibleSources[source.Id] {
				return errors.New("hidden source leaked")
			}
			seen[source.Id] = true
		}
		if len(seen) != 12 {
			return errors.New("source matrix incomplete")
		}
		for _, rel := range body.Relations {
			if rel.Resource.Id != f.Resources["other"].String() && rel.Resource.Id != f.Resources["retired-category"].String() {
				return errors.New("hidden relation endpoint leaked")
			}
			if rel.Type == "related_to" && rel.Direction != "symmetric" {
				return errors.New("symmetric relation direction differs")
			}
			if rel.Type == "part_of" && rel.Direction != "outgoing" {
				return errors.New("outgoing direction differs")
			}
			if (rel.Type == "successor_of" || rel.Type == "derived_from") && rel.Direction != "incoming" {
				return errors.New("incoming direction differs")
			}
		}
	}
	var list generated.ResourceList
	if err := Request(app, "/resources?page_size=1", 200, &list); err != nil {
		return err
	}
	if len(list.Items) != 1 || !list.HasNext || list.Page != 1 || list.PageSize != 1 {
		return errors.New("public pagination differs")
	}
	if err := Request(app, "/resources/"+f.Slug("retired-category"), 200, nil); err != nil {
		return err
	}
	for _, kind := range []string{"categories", "tags"} {
		var body struct {
			Items []generated.CategoryItem `json:"items"`
		}
		if err := Request(app, "/"+kind+"?locale=JA", 200, &body); err != nil {
			return err
		}
		found := false
		for _, item := range body.Items {
			if item.Slug == f.Slug("retired") || item.Slug == f.Slug("deleted") {
				return errors.New("non-active taxonomy in browse")
			}
			if item.Slug == f.Slug("active") {
				found = true
				if item.Name != "Requested "+kind+" active" || item.Description == nil || *item.Description != "Default taxonomy description" {
					return errors.New("taxonomy fallback differs")
				}
			}
		}
		if !found {
			return errors.New("active taxonomy missing")
		}
	}
	return nil
}

package contributioncheck

import (
	"errors"
	"fmt"
	admin "github.com/deepfurry/tap4furry/server/internal/transport/admin/generated"
	public "github.com/deepfurry/tap4furry/server/internal/transport/public/generated"
	"net/http"
	"uuid"
)

func runChanges(f *Fixture, a, anon *client, authors [7]*http.Cookie, rid, slug, category string) error {
	var tag admin.EntityID
	if err := a.call("POST", "/tags", map[string]any{"slug": slug, "default_locale": "en", "localization": map[string]string{"name": "Contribution tag"}}, 201, &tag); err != nil {
		return err
	}
	f.Graph.Tags = append(f.Graph.Tags, tag.Id)
	var other admin.ResourceRevision
	if err := a.call("POST", "/resources", map[string]any{"slug": slug + "-other", "default_locale": "en", "category_id": category, "content_rating": "general", "localization": map[string]string{"name": "Other public resource"}}, 201, &other); err != nil {
		return err
	}
	f.Graph.Resources = append(f.Graph.Resources, other.Id)
	if err := a.call("PUT", "/resources/"+other.Id+"/publication?expected_version=1", map[string]string{"state": "published"}, 200, nil); err != nil {
		return err
	}
	kinds := []string{"add_source", "remove_broken_source", "add_tag", "add_relation", "add_translation"}
	sourceID := ""
	for i, kind := range kinds {
		author := &client{ctx: a.ctx, app: anon.app, cookie: authors[i+2], origin: "http://localhost:4321"}
		if err := author.token(); err != nil {
			return err
		}
		path := "/contributions/context/" + slug + "?kind=" + kind
		change := map[string]any{}
		switch kind {
		case "add_source":
			change["source"] = map[string]any{"url": "HTTPS://EXAMPLE.INVALID:443/new-source#fragment", "label": "Contributed source", "source_type": "external"}
		case "remove_broken_source":
			path += "&source_id=" + sourceID
			change["source_id"] = sourceID
		case "add_tag":
			change["tag_ids"] = []string{tag.Id}
		case "add_relation":
			path += "&other_slug=" + slug + "-other&relation_type=part_of&direction=outgoing"
			change["relation"] = map[string]any{"other_resource_id": other.Id, "relation_type": "part_of", "direction": "outgoing"}
		case "add_translation":
			path += "&locale=ja"
			change["translation"] = map[string]any{"locale": "ja", "name": "翻訳リソース", "summary": nil, "description": "**翻訳** <script>text only</script>"}
		}
		var base public.ContributionContext
		if err := author.call("GET", path, nil, 200, &base); err != nil {
			return fmt.Errorf("%s context: %w", kind, err)
		}
		body := map[string]any{"kind": kind, "request_id": uuid.NewV7().String(), "reason": "Independent fixture evidence", "target_resource_id": rid, "base_revision": base.BaseRevision, "change": change}
		var created public.ContributionCreated
		if err := author.call("POST", "/contributions", body, 201, &created); err != nil {
			return fmt.Errorf("%s submit: %w", kind, err)
		}
		f.IDs = append(f.IDs, created.Id)
		var replay public.ContributionCreated
		if err := author.call("POST", "/contributions", body, 201, &replay); err != nil || replay.Id != created.Id {
			return errors.New("extended replay failed")
		}
		var review admin.ContributionDetail
		if err := a.call("GET", "/contributions/"+created.Id, nil, 200, &review); err != nil {
			return err
		}
		if review.Proposed != nil || review.ProposedChange == nil {
			return errors.New("new proposal reused basic content")
		}
		if kind == "add_source" {
			change["source"] = map[string]any{"url": "https://example.invalid/new-source", "label": "Reviewed source", "source_type": "external", "availability_state": "active"}
		}
		if err := a.call("POST", "/contributions/"+created.Id+"/accept", map[string]any{"change": change, "message": "Reviewed fixture evidence", "internal_note": "Private extended review"}, 204, nil); err != nil {
			return fmt.Errorf("%s accept: %w", kind, err)
		}
		var own public.ContributionDetail
		if err := author.call("GET", "/me/contributions/"+created.Id, nil, 200, &own); err != nil {
			return err
		}
		if own.Status != "accepted" || own.ProposedChange == nil || own.AcceptedChange == nil || own.Proposed != nil || len(own.History) != 2 {
			return errors.New("extended author history failed")
		}
		var record public.ResourceDetail
		if err := anon.call("GET", "/resources/"+slug+"?locale=ja", nil, 200, &record); err != nil {
			return err
		}
		switch kind {
		case "add_source":
			for _, s := range record.Sources {
				if s.Url == "https://example.invalid/new-source" {
					sourceID = s.Id
					if s.IsPrimary {
						return errors.New("source primary changed")
					}
				}
			}
			if sourceID == "" {
				return errors.New("accepted source missing")
			}
		case "remove_broken_source":
			for _, s := range record.Sources {
				if s.Id == sourceID {
					return errors.New("removed source publicly listed")
				}
			}
			if own.AcceptedChange.Source != nil {
				return errors.New("removed URL leaked through history")
			}
		case "add_tag":
			if len(record.Tags) != 1 || record.Tags[0].Id != tag.Id {
				return errors.New("accepted tag missing")
			}
		case "add_relation":
			if len(record.Relations) != 1 {
				return errors.New("accepted relation missing")
			}
		case "add_translation":
			if record.Name != "翻訳リソース" || record.Summary != nil {
				return errors.New("accepted translation/fallback wrong")
			}
		}
		if err := a.call("GET", "/contributions/"+created.Id, nil, 200, &review); err != nil {
			return err
		}
		want := 1
		if kind == "add_relation" {
			want = 2
		}
		if review.ResourceChanges == nil || len(*review.ResourceChanges) != want {
			return errors.New("canonical version audit incomplete")
		}
	}
	return nil
}

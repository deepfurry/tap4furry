package contributionhttp

import (
	"github.com/gofiber/fiber/v3"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBoundedStrictJSON(t *testing.T) {
	app := fiber.New(fiber.Config{BodyLimit: 300 * 1024})
	app.Post("/", func(c fiber.Ctx) error {
		var body struct {
			Content struct {
				Name string `json:"name"`
			} `json:"content"`
		}
		fields, err := Decode(c, &body, []string{"content"}, []string{"content"}, nil)
		if err == nil {
			_, err = Object(fields["content"], []string{"name"}, []string{"name"}, nil)
		}
		if err != nil {
			return c.SendStatus(400)
		}
		return c.SendStatus(204)
	})
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"content":{"name":"🦊"}}`, 204}, {`{"Content":{"name":"a"}}`, 400}, {`{"content":{"Name":"a"}}`, 400}, {`{"content":{"name":"a","name":"b"}}`, 400}, {`{"content":null}`, 400}, {`{"content":{"name":null}}`, 400}, {`{"content":{"name":"a"}}{}`, 400}, {`{"content":{"name":"` + strings.Repeat("a", 256*1024) + `"}}`, 400},
	} {
		req := httptest.NewRequest("POST", "/", strings.NewReader(test.body))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != test.status {
			t.Fatalf("strict decoder got %d want %d", res.StatusCode, test.status)
		}
	}
}

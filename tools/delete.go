package main

import (
	"fmt"
	"log"

	"github.com/spf13/pflag"
	goconfluence "github.com/virtomize/confluence-go-api"
)

type Client struct {
	*goconfluence.API
	Cq goconfluence.ContentQuery
}

func main() {
	// Flags
	account := pflag.StringP("account", "a", "", "Account name")
	username := pflag.StringP("username", "u", "", "Username")
	token := pflag.StringP("token", "t", "", "API Token")
	rootPageID := pflag.StringP("pageid", "p", "", "Page ID to delete recursively from")
	spaceKey := pflag.StringP("spacekey", "k", "", "Space Key of the page")
	debug := pflag.Bool("debug", false, "Enable debug prints. VERY noisy. This will probably print secrets to stdout.")

	pflag.Parse()

	confluenceRESTURL := fmt.Sprintf("https://%s.atlassian.net/wiki/rest/api", *account)
	c, err := goconfluence.NewAPI(confluenceRESTURL, *username, *token)
	if err != nil {
		log.Fatal(err)
	}
	client := Client{
		API: c,
		Cq: goconfluence.ContentQuery{
			SpaceKey: *spaceKey,
			Type:     "page",
			Expand:   []string{"space", "body.storage", "version", "ancestors", "descendants"},
		},
	}

	goconfluence.DebugFlag = *debug

	content, err := client.GetContentByID(*rootPageID, client.Cq)
	if err != nil {
		log.Fatalf("failed to get page #%s, %s\n", *rootPageID, err)
	}

	var childContent []*goconfluence.Content
	cc, err := client.getChildContent(content.ID)
	if err != nil {
		log.Fatalf("failed to get child content, %s", err)
	}
	if len(cc) != 0 {
		childContent = append(childContent, cc...)
	}

	if len(childContent) != 0 {
		for _, cc := range childContent {
			_, err := client.DelContent(cc.ID)
			if err != nil {
				log.Fatalf("failed to delete child content, pageid %s: %s,", cc.ID, err)
			}
		}
	}
	_, err = client.DelContent(*rootPageID)
	if err != nil {
		log.Fatal(err)
	}

}

func (c *Client) getChildContent(parentPageID string) ([]*goconfluence.Content, error) {
	var childContent []*goconfluence.Content
	// Grab any child pages from parent
	childPages, err := c.GetChildPages(parentPageID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve list of child pages for pageID %s: %s", parentPageID, err)
	}

	if len(childPages.Results) != 0 {
		for _, page := range childPages.Results {
			if page.Type == "page" {
				content, err := c.GetContentByID(page.ID, c.Cq)
				if err != nil {
					return nil, fmt.Errorf("failed to get content from pageID %s, child of %s: %s", page.ID, parentPageID, err)
				}
				childContent = append(childContent, content)
				subChildContent, err := c.getChildContent(content.ID)
				if err != nil {
					return nil, err
				}
				if len(subChildContent) != 0 {
					childContent = append(childContent, subChildContent...)
				}
			}
		}
	}
	return childContent, nil
}

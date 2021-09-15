package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	goconfluence "github.com/virtomize/confluence-go-api"
)

type Client struct {
	SourceClient       *goconfluence.API
	DestClient         *goconfluence.API
	ConflictSuffix     string
	SourceContentQuery goconfluence.ContentQuery
	DestContentQuery   goconfluence.ContentQuery
}

type Config struct {
	Source         LocationConfig `mapstructure:"source"`
	Destination    LocationConfig `mapstructure:"dest"`
	ConflictSuffix string         `mapstructure:"conflictsuffix"`
}

type LocationConfig struct {
	User     string `mapstructure:"user"`
	Account  string `mapstructure:"account"`
	Token    string `mapstructure:"token"`
	PageId   string `mapstructure:"pageid"`
	SpaceKey string `mapstructure:"spacekey"`
}

type contentNode struct {
	content  *goconfluence.Content
	children []contentNode
}

func main() {
	// Flags
	pflag.String("source-account", "", "Source Confluence account name")
	pflag.String("dest-account", "", "Destination Confluence account name. Can be same as source")
	pflag.String("source-user", "", "Source Confluence Username. Usually email address")
	pflag.String("dest-user", "", "Destination Confluence Username. Usually email address")
	pflag.StringP("source-token", "s", "", "Source Confluence API token")
	pflag.StringP("dest-token", "d", "", "Destination Confluence API token")

	pflag.String("source-pageid", "", "Source Confluence page ID. This is where the export will start to recursively copy pages")
	pflag.String("source-spacekey", "", "Source Confluence Space key")
	pflag.String("dest-pageid", "", "Destination Confluence page ID. Leave blank if top-level")
	pflag.String("dest-spacekey", "", "Destination Confluence Space key")

	recursive := pflag.BoolP("recursive", "r", false, "Enable if you want to recursively copy child pages under source-pageid")
	pflag.String("conflictsuffix", "- import", "String to append to page titles that already exist in the destination space")
	configFile := pflag.StringP("config", "c", "", "YAML Configuration file. Full path and extension")
	debug := pflag.Bool("debug", false, "Enable debug prints. VERY noisy. This will probably print secrets to stdout.")

	pflag.Parse()

	if *configFile != "" {
		viper.SetConfigFile(*configFile)
		err := viper.ReadInConfig()
		if err != nil {
			log.Fatalf("failed to read config file %s, %s", *configFile, err)
		}
	}

	pflag.VisitAll(func(f *pflag.Flag) {
		configName := strings.ReplaceAll(f.Name, "-", ".")
		viper.BindPFlag(configName, f)
	})

	if !viper.IsSet("source.user") {
		log.Println("please provide source username")
		os.Exit(1)
	}
	if !viper.IsSet("source.token") {
		log.Println("please provide source token")
		os.Exit(1)
	}
	if !viper.IsSet("source.pageid") {
		log.Println("please provide source page ID")
		os.Exit(1)
	}

	if !viper.IsSet("source.spacekey") {
		log.Println("please provide source space key")
		os.Exit(1)
	}

	if !viper.IsSet("dest.account") {
		log.Println("please provide destination account. Can be same as source account")
		os.Exit(1)
	}

	if !viper.IsSet("dest.spacekey") {
		log.Println("please provide destination space key")
		os.Exit(1)
	}

	conf := Config{}
	err := viper.Unmarshal(&conf)
	if err != nil {
		log.Fatalf("failed to unmrashal config, %s", err)
	}
	c := Client{
		ConflictSuffix: conf.ConflictSuffix,
	}
	if conf.Source.Account == conf.Destination.Account {
		confluenceRESTURL := fmt.Sprintf("https://%s.atlassian.net/wiki/rest/api", conf.Source.Account)
		sc, err := goconfluence.NewAPI(confluenceRESTURL, conf.Source.User, conf.Source.Token)
		if err != nil {
			log.Fatal(err)
		}
		c.SourceClient = sc
		c.DestClient = sc

	} else {
		var err error
		if conf.Destination.Token == "" {
			log.Println("please provide destination token")
			os.Exit(1)
		}
		if conf.Destination.User == "" {
			log.Println("please provide destination username")
			os.Exit(1)
		}
		sourceRESTURL := fmt.Sprintf("https://%s.atlassian.net/wiki/rest/api", conf.Source.Account)
		destRESTURL := fmt.Sprintf("https://%s.atlassian.net/wiki/rest/api", conf.Destination.Account)

		sc, err := goconfluence.NewAPI(sourceRESTURL, conf.Source.User, conf.Source.Token)
		if err != nil {
			log.Fatalf("failed to create source client, %s\n", err)
		}
		c.SourceClient = sc
		dc, err := goconfluence.NewAPI(destRESTURL, conf.Destination.User, conf.Destination.Token)
		if err != nil {
			log.Fatalf("failed to create destination client, %s\n", err)
		}
		c.DestClient = dc
	}

	goconfluence.DebugFlag = *debug
	c.SourceContentQuery = goconfluence.ContentQuery{
		SpaceKey: conf.Source.SpaceKey,
		Type:     "page",
		Expand:   []string{"space", "body.storage", "version", "ancestors", "descendants"},
	}

	c.DestContentQuery = goconfluence.ContentQuery{
		SpaceKey: conf.Destination.SpaceKey,
		Type:     "page",
		Expand:   []string{"space", "body.storage", "version", "ancestors", "descendants"},
	}

	if conf.Destination.PageId != "" {
		// Get destination page first to make sure it exists
		destContent, err := c.DestClient.GetContentByID(conf.Destination.PageId, c.DestContentQuery)
		if err != nil {
			log.Fatalf("failed to get destination page #%s, %s\n", conf.Destination.PageId, err)
		}
		if destContent.Space.Key != conf.Destination.SpaceKey {
			log.Fatalf("destination page space key (%s) and destination space key (%s) do not match",
				destContent.Space.Key, conf.Destination.SpaceKey)
		}
	}

	// Start grabbing the parent source page
	rootContent, err := c.SourceClient.GetContentByID(conf.Source.PageId, c.SourceContentQuery)
	if err != nil {
		log.Fatal(err)
	}
	rootNode := &contentNode{
		content: rootContent,
	}

	if *recursive {
		err := c.getChildContent(rootNode)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Create parent page in new location
	var parentAncestry []goconfluence.Ancestor
	if conf.Destination.PageId != "" {
		parentAncestry = buildAncestry([]string{conf.Destination.PageId})
	}

	err = c.createContent(rootNode, parentAncestry, conf.Destination.SpaceKey)
	if err != nil {
		log.Fatal(err)
	}
}

func (c *Client) getChildContent(rootNode *contentNode) error {
	// Grab any child pages from parent
	childPages, err := c.SourceClient.GetChildPages(rootNode.content.ID)
	if err != nil {
		return fmt.Errorf("failed to retrieve list of child pages for pageID %s: %w", rootNode.content.ID, err)
	}

	if len(childPages.Results) != 0 {
		for _, page := range childPages.Results {
			if page.Type == "page" {
				content, err := c.SourceClient.GetContentByID(page.ID, c.SourceContentQuery)
				if err != nil {
					return fmt.Errorf("failed to get content from pageID %s, child of %s: %w", page.ID, rootNode.content.ID, err)
				}
				childNode := contentNode{
					content: content,
				}
				err = c.getChildContent(&childNode)
				if err != nil {
					return err
				}
				rootNode.children = append(rootNode.children, childNode)
			}
		}
	}
	return nil
}

func (c *Client) createContent(rootNode *contentNode, parentAncestry []goconfluence.Ancestor, spaceKey string) error {
	newContent := c.generateNewContent(rootNode.content, parentAncestry, spaceKey)
	respContent, err := c.DestClient.CreateContent(newContent)
	if err != nil {
		return fmt.Errorf("failed to create new page %q: %w", rootNode.content.Title, err)
	}

	if len(rootNode.children) != 0 {
		// Create child pages in new location
		for _, cn := range rootNode.children {
			childAncestor := buildAncestry([]string{respContent.ID})
			err := c.createContent(&cn, childAncestor, spaceKey)
			if err != nil {
				return fmt.Errorf("failed to create new child page %q: %w", cn.content.Title, err)
			}
		}
	}
	return nil
}

func (c *Client) generateNewContent(sourceContent *goconfluence.Content, newAncestry []goconfluence.Ancestor, newSpaceKey string) *goconfluence.Content {
	newTitle := sourceContent.Title
	if newSpaceKey == sourceContent.Space.Key {
		newTitle = sourceContent.Title + c.ConflictSuffix
	}

	return &goconfluence.Content{
		ID:        "",
		Type:      sourceContent.Type,
		Status:    sourceContent.Status,
		Title:     newTitle,
		Ancestors: newAncestry,
		Body:      sourceContent.Body,
		Space: goconfluence.Space{
			Key: newSpaceKey,
		},
	}
}

func buildAncestry(ancestorIDs []string) []goconfluence.Ancestor {
	var a []goconfluence.Ancestor

	for _, ancestorID := range ancestorIDs {
		a = append(a, goconfluence.Ancestor{
			ID: ancestorID,
		})
	}
	return a
}

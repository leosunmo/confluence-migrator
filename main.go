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
	SourceClient   *goconfluence.API
	DestClient     *goconfluence.API
	ConflictSuffix string
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
	sourceCQ := goconfluence.ContentQuery{
		SpaceKey: conf.Source.SpaceKey,
		Type:     "page",
		Expand:   []string{"space", "body.storage", "version", "ancestors", "descendants"},
	}

	destCQ := goconfluence.ContentQuery{
		SpaceKey: conf.Destination.SpaceKey,
		Type:     "page",
		Expand:   []string{"space", "body.storage", "version", "ancestors", "descendants"},
	}

	if conf.Destination.PageId != "" {
		// Get destination page first to make sure it exists, and to grab its
		// ancestry for later.
		destContent, err := c.DestClient.GetContentByID(conf.Destination.PageId, destCQ)
		if err != nil {
			log.Fatalf("failed to get destination page #%s, %s\n", conf.Destination.PageId, err)
		}
		if destContent.Space.Key != conf.Destination.SpaceKey {
			log.Fatalf("destination page space key (%s) and destination space key (%s) do not match",
				destContent.Space.Key, conf.Destination.SpaceKey)
		}
	}

	// Start grabbing the parent source page
	content, err := c.SourceClient.GetContentByID(conf.Source.PageId, sourceCQ)
	if err != nil {
		log.Fatal(err)
	}

	var childContent []*goconfluence.Content
	if *recursive {
		// Grab any child pages from parent
		childPages, err := c.SourceClient.GetChildPages(conf.Source.PageId)
		if err != nil {
			log.Fatal(err)
		}

		if len(childPages.Results) != 0 {
			for _, page := range childPages.Results {
				if page.Type == "page" {
					content, err := c.SourceClient.GetContentByID(page.ID, sourceCQ)
					if err != nil {
						log.Fatalf("failed to get child content, pageID #%s, %s\n", page.ID, err)
					}
					childContent = append(childContent, content)
				}
			}
		}
	}

	// Create parent page in new location
	var parentAncestry []goconfluence.Ancestor
	if conf.Destination.PageId != "" {
		parentAncestry = buildAncestry([]string{conf.Destination.PageId})
	}

	newContent := c.generateNewContent(content, parentAncestry, conf.Destination.SpaceKey)
	respContent, err := c.DestClient.CreateContent(newContent)
	if err != nil {
		log.Fatalf("failed to create new parent page, %s\n", err)
	}

	if len(childContent) != 0 {
		// Create child pages in new location
		for _, cc := range childContent {
			childAncestor := buildAncestry([]string{respContent.ID})
			_, err := c.DestClient.CreateContent(c.generateNewContent(cc, childAncestor, conf.Destination.SpaceKey))
			if err != nil {
				log.Fatalf("failed to create new child page #%s: %s\n", cc.ID, err)
			}
		}
	}

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

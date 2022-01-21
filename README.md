# confluence-migrator
Confluence page migration/copying tool.
Copy pages (and their children) to a new parent page, new Space, or even a new Confluence account.

## Why
Confluence's built in Export tool (to XML) works well enough, as you can select which pages you want to export. I cannot for the life of me find a nice way to _IMPORT_ these XML files again. The only option seems to be a full "restore from backup" procedure which seems overkill. So instead I went overkill and wrote this mess in anger.

## Usage
Build it and use flags or config file to move pages.

```
go build .
```

```
Usage of ./confluence-migrator:
  -c, --config string            YAML Configuration file. Full path and extension
      --conflictsuffix string    String to append to page titles that already exist in the destination space (default "- import")
      --debug                    Enable debug prints. VERY noisy. This will probably print secrets to stdout.
      --dest-account string      Destination Confluence account name. Can be same as source
      --dest-pageid string       Destination Confluence page ID. Leave blank if top-level
      --dest-spacekey string     Destination Confluence Space key
  -d, --dest-token string        Destination Confluence API token
      --dest-user string         Destination Confluence Username. Usually email address
  -r, --recursive                Enable if you want to recursively copy child pages under source-pageid
      --source-account string    Source Confluence account name
      --source-pageid string     Source Confluence page ID. This is where the export will start to recursively copy pages
      --source-spacekey string   Source Confluence Space key
  -s, --source-token string      Source Confluence API token
      --source-user string       Source Confluence Username. Usually email address
```

ConflictSuffix is required as a page title has to be unique within a Space. So if you move a page somewhere in the same Space, it will have this string appended to its title.

Account is the name of your account as seen in the Confluence subdomain, https://mycompany.atlassian.net/.

### Examples
Copying a page and all its sub-pages to a different page (making them sub-pages to the new page) in the same space:
```
# config.yaml
source:
  user: leo.sunmo@company.com
  account: mycompany
  pageid: 123456
  spacekey: IT
dest:
  account: mycompany
  pageid: 678890
  spaceKey: IT
conflictsuffix: "- import"
```

`$ ./confluence-migrator -s my-secret-token123 -r -c config.yaml`


Copying a page, ignore its sub-pages to the root of a different space in the same account:
```
# config.yaml
source:
  user: leo.sunmo@company.com
  account: mycompany
  pageid: 123456
  spacekey: IT
dest:
  account: mycompany
  spaceKey: MARKETING
```
`$ ./confluence-migrator -s my-secret-token123 -c config.yaml`


Copying a page and all its sub-pages to the root of a space in a new account:
```
# config.yaml
source:
  user: leo.sunmo@company.com
  account: mycompany
  pageid: 123456
  spacekey: IT
dest:
  user: leo.sunmo@newcompany.com
  account: newcompany
  spaceKey: OPS
```
`$ ./confluence-migrator -s my-secret-token123 -d my-other-token456 -r -c config.yaml`

## TODO
* Copy attachments properly.
* Figure out if we can migrate user that created the page in source.  
  This is a bit tricky as Atlassian changed how the User API works and it now requires some new global Atlassian User ID, which none of the users I tested with had.
* Maintain comments and history.  
  This is totally possible with the current state of the REST API. Without solving the user issue above first, this will just be a mess however.

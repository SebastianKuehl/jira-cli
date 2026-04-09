# JIRA CLI

This CLI fetches Jira tickets to markdown files.
Markdown tickets are grouped in folders (sprints).
REST API v2 is used.

## Tech stack
- Go
- kong
- stdlib HTTP/JSON
- GoReleaser

## Authentication
The CLI looks for required fields JIRA_BASE_URL and JIRA_TOKEN in order:
- command arguments
- environment variables
- .env file

## Configuration
Configuration is stored in the user's home path in the folder `.jira/config`.
The user can configure the cli:
- project: if no project is configured, each command has to prompt the user to interactively select from a list of available jira projects. If a project is configured, no prompt occurs
- board: a board specific to the selected project. If not configured the user will be prompted whereever required to specify a board from a list of boards for the given project
- project base path: an optional directory where folders and files will be written to for the configured project. Without an base path configured folders and files will be written to the command invokation folder

### Flow
The config
- does not exist:
    - prompt the user for project and project base path
- exists:
    - print the actual config path
    - print the actual config content nicely formatted (no json)
    - prompt yes (y) or no (n) to overwrite the existing config

## Commands
```sh
jira --help
jira test # checks if the environment variables can be used to reach the jira tickets
jira config # select a default project from a list of available projects, set a project base path to write the project jira tickets to
jira rm config # removes the config
jira rm sprint [sprint] # removes the local sprint folder and all of its local markdown tickets
jira rm ticket [id] # removes the local markdown file for id
jira fetch # fetches all jira tickets and groups them by sprint
jira fetch [sprint|id] # fetches all jira tickets for the specified sprint or a specific ticket and places that in the corresponding folder
jira fetch [sprint|id] --year [yyyy] # limits fetch results to sprints in the specified four digit year
jira ls # lists all sprints
jira ls [sprint] [--verbose|-v] # lists all tickets of sprint [sprint]
jira cat [sprint|id] # outputs the sprint's goal OR stdouts the ticket properties and body to console
jira move [id] # prints the ticket's workflow state and allows the user to select the next state from a list of available states
jira assign [id] [user] # assigns the ticket to user. user is optional: left empty, the invoking user will be assigned
jira unassign [id] # unassigns the ticket
```

Specifics for the command `jira ls [sprint]`:
- prints the list of jira tickets in the format: <id> <title>
- with flag --verbose or -v:
    - prints a line with <id><title> 
    - prints: <assignee> and <reporter>
    - prints the <workflow state>
    - optionally prints a link to a pull request


## Markdown file
- file name: the ticket id
- file structure:
    - frontmatter properties of the jira ticket and a backlink to the jira ticket
    - jira ticket body

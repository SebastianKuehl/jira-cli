package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/sebastian/jira-cli/internal/config"
	"github.com/sebastian/jira-cli/internal/env"
	"github.com/sebastian/jira-cli/internal/jira"
)

type App struct {
	cli CLI
}

func New() *App {
	return &App{}
}

func (a *App) Run(args []string) error {
	parser, err := kong.New(&a.cli,
		kong.Name("jira"),
		kong.Description("Jira CLI to fetch and sync tickets to markdown."),
	)
	if err != nil {
		return err
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		return err
	}
	if err := a.cli.bootstrap(); err != nil {
		return err
	}
	return ctx.Run(&Context{
		CLI: &a.cli,
	})
}

type CLI struct {
	BaseURL string `help:"Jira base URL." env:"JIRA_BASE_URL"`
	Token   string `help:"Jira token." env:"JIRA_TOKEN"`

	Test      TestCmd      `cmd:"" help:"Checks if Jira credentials can reach Jira."`
	Configure ConfigureCmd `cmd:"" help:"Configure default project and base path."`
	Fetch     FetchCmd     `cmd:"" help:"Fetch tickets and group by sprint."`
	Ls        LsCmd        `cmd:"" help:"List sprints or tickets in a sprint."`
	Cat       CatCmd       `cmd:"" help:"Print sprint goal or ticket content."`
	Move      MoveCmd      `cmd:"" help:"Move ticket to next workflow state."`
	Assign    AssignCmd    `cmd:"" help:"Assign ticket to user."`
	Unassign  UnassignCmd  `cmd:"" help:"Unassign ticket."`

	Config config.Config `kong:"-"`
}

func (c *CLI) bootstrap() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c.Config = cfg

	dotenvPath := ".env"
	if _, err := os.Stat(dotenvPath); err == nil {
		m, err := env.LoadFile(dotenvPath)
		if err != nil {
			return err
		}
		if c.BaseURL == "" {
			c.BaseURL = m["JIRA_BASE_URL"]
		}
		if c.Token == "" {
			c.Token = m["JIRA_TOKEN"]
		}
	}
	return nil
}

type Context struct {
	CLI *CLI
}

func (c *Context) JiraClient() (*jira.Client, error) {
	if c.CLI.BaseURL == "" || c.CLI.Token == "" {
		return nil, errors.New("missing Jira credentials: provide args/env/.env for JIRA_BASE_URL and JIRA_TOKEN")
	}
	return jira.NewClient(c.CLI.BaseURL, c.CLI.Token), nil
}

func (c *Context) ProjectPath() (string, error) {
	if c.CLI.Config.BasePath == "" {
		return os.Getwd()
	}
	return filepath.Abs(c.CLI.Config.BasePath)
}

type TestCmd struct{}

func (t *TestCmd) Run(ctx *Context) error {
	client, err := ctx.JiraClient()
	if err != nil {
		return err
	}
	if err := client.TestConnection(context.Background()); err != nil {
		return err
	}
	fmt.Println("jira test: ok")
	return nil
}

type ConfigureCmd struct {
	Project  string `help:"Default Jira project key."`
	BasePath string `help:"Optional base path to write markdown files."`
}

func (c *ConfigureCmd) Run(ctx *Context) error {
	cfg := ctx.CLI.Config
	if c.Project != "" {
		cfg.Project = c.Project
	}
	if c.BasePath != "" {
		p, err := filepath.Abs(c.BasePath)
		if err != nil {
			return err
		}
		cfg.BasePath = p
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Println("configuration saved")
	return nil
}

type FetchCmd struct {
	Sprint string `arg:"" optional:"" help:"Sprint name to fetch."`
	Ticket string `name:"ticket" help:"Fetch one ticket by ID."`
}

func (c *FetchCmd) Run(ctx *Context) error {
	_, err := ctx.ProjectPath()
	if err != nil {
		return err
	}
	return jira.ErrNotImplemented
}

type LsCmd struct {
	Sprint  string `arg:"" optional:"" help:"Sprint name."`
	Verbose bool   `short:"v" help:"Verbose output."`
}

func (c *LsCmd) Run(ctx *Context) error {
	_ = ctx
	return jira.ErrNotImplemented
}

type CatCmd struct {
	Target string `arg:"" help:"Sprint name or ticket ID."`
}

func (c *CatCmd) Run(ctx *Context) error {
	_ = ctx
	return jira.ErrNotImplemented
}

type MoveCmd struct {
	ID string `arg:"" help:"Ticket ID."`
}

func (c *MoveCmd) Run(ctx *Context) error {
	_ = ctx
	return jira.ErrNotImplemented
}

type AssignCmd struct {
	ID   string `arg:"" help:"Ticket ID."`
	User string `arg:"" optional:"" help:"User; defaults to invoking user."`
}

func (c *AssignCmd) Run(ctx *Context) error {
	_ = ctx
	return jira.ErrNotImplemented
}

type UnassignCmd struct {
	ID string `arg:"" help:"Ticket ID."`
}

func (c *UnassignCmd) Run(ctx *Context) error {
	_ = ctx
	return jira.ErrNotImplemented
}

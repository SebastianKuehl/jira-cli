package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

	Test   TestCmd   `cmd:"" help:"Checks if Jira credentials can reach Jira."`
	Config ConfigCmd `cmd:"config" help:"Configure default project and base path."`
	Rm     RmCmd     `cmd:"rm" help:"Remove config, sprint folder, or ticket file."`
	Fetch  FetchCmd  `cmd:"" help:"Fetch tickets and group by sprint."`
	Ls        LsCmd        `cmd:"" help:"List sprints or tickets in a sprint."`
	Cat       CatCmd       `cmd:"" help:"Print sprint goal or ticket content."`
	Move      MoveCmd      `cmd:"" help:"Move ticket to next workflow state."`
	Assign    AssignCmd    `cmd:"" help:"Assign ticket to user."`
	Unassign  UnassignCmd  `cmd:"" help:"Unassign ticket."`

	Cfg config.Config `kong:"-"`
}

func (c *CLI) bootstrap() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c.Cfg = cfg

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
	if c.CLI.Cfg.BasePath == "" {
		return os.Getwd()
	}
	return filepath.Abs(c.CLI.Cfg.BasePath)
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

type ConfigCmd struct {
	Info bool `name:"info" help:"Print config file path and contents."`
}

func (c *ConfigCmd) Run(ctx *Context) error {
	if c.Info {
		path, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Println("Config file:", path)
		exists, err := config.Exists()
		if err != nil {
			return err
		}
		if !exists {
			fmt.Println("No config file found.")
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}

	exists, err := config.Exists()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	isInteractive := stdinIsTerminal()

	if exists {
		if !isInteractive {
			return errors.New("config already exists; re-run interactively to replace it")
		}
		fmt.Print("A config already exists. Create a new one? [y/N]: ")
		answer, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	client, err := ctx.JiraClient()
	if err != nil {
		return err
	}
	projects, err := client.ListProjects(context.Background())
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return errors.New("no Jira projects available for current credentials")
	}

	if !isInteractive {
		return errors.New("project must be selected interactively; stdin is not a terminal")
	}

	fmt.Println("Select a default Jira project:")
	for i, p := range projects {
		fmt.Printf("  %d) %s - %s\n", i+1, p.Key, p.Name)
	}
	fmt.Print("Project number: ")
	raw, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	selected, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || selected < 1 || selected > len(projects) {
		return errors.New("invalid project selection")
	}

	cfg := config.Config{
		Project: projects[selected-1].Key,
	}

	fmt.Print("Project base path (leave empty for current directory): ")
	rawBasePath, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	basePath := strings.TrimSpace(rawBasePath)
	if basePath != "" {
		p, err := expandPath(basePath)
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

// RmCmd groups removal subcommands.
type RmCmd struct {
	Config RmConfigCmd `cmd:"config" help:"Remove the config file."`
	Sprint RmSprintCmd `cmd:"sprint" help:"Remove a local sprint folder and its tickets."`
	Ticket RmTicketCmd `cmd:"ticket" help:"Remove a local markdown file for a ticket."`
}

type RmConfigCmd struct{}

func (c *RmConfigCmd) Run(ctx *Context) error {
	_ = ctx
	return jira.ErrNotImplemented
}

type RmSprintCmd struct {
	Sprint string `arg:"" help:"Sprint name."`
}

func (c *RmSprintCmd) Run(ctx *Context) error {
	_ = ctx
	return jira.ErrNotImplemented
}

type RmTicketCmd struct {
	ID string `arg:"" help:"Ticket ID."`
}

func (c *RmTicketCmd) Run(ctx *Context) error {
	_ = ctx
	return jira.ErrNotImplemented
}

func expandPath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		p = filepath.Join(home, p[2:])
	}
	return filepath.Abs(p)
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
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

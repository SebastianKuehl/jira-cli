package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

	Test     TestCmd     `cmd:"" help:"Checks if Jira credentials can reach Jira."`
	Config   ConfigCmd   `cmd:"config" help:"Configure default project and base path."`
	Rm       RmCmd       `cmd:"rm" help:"Remove config, sprint folder, or ticket file."`
	Fetch    FetchCmd    `cmd:"" help:"Fetch tickets and group by sprint."`
	Ls       LsCmd       `cmd:"" help:"List sprints or tickets in a sprint."`
	Cat      CatCmd      `cmd:"" help:"Print sprint goal or ticket content."`
	Move     MoveCmd     `cmd:"" help:"Move ticket to next workflow state."`
	Assign   AssignCmd   `cmd:"" help:"Assign ticket to user."`
	Unassign UnassignCmd `cmd:"" help:"Unassign ticket."`

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
	return expandPath(c.CLI.Cfg.BasePath)
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

type ConfigCmd struct{}

func (c *ConfigCmd) Run(ctx *Context) error {
	exists, err := config.Exists()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	isInteractive := stdinIsTerminal()

	if exists {
		path, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Println("Config file:", path)
		cfg, loadErr := config.Load()
		if loadErr != nil {
			fmt.Println("  (config file exists but could not be read:", loadErr, ")")
		} else {
			fmt.Println("  Project:   ", cfg.Project)
			if cfg.Project != "" && cfg.BoardByProject != nil && cfg.BoardByProject[cfg.Project] > 0 {
				fmt.Println("  Board ID:  ", cfg.BoardByProject[cfg.Project])
				if cfg.BoardNameByProject != nil && strings.TrimSpace(cfg.BoardNameByProject[cfg.Project]) != "" {
					fmt.Println("  Board:     ", cfg.BoardNameByProject[cfg.Project])
				}
			} else if cfg.BoardID > 0 {
				fmt.Println("  Board ID:  ", cfg.BoardID)
			} else {
				fmt.Println("  Board ID:   (not configured)")
			}
			if cfg.BasePath != "" {
				fmt.Println("  Base path: ", cfg.BasePath)
			} else {
				fmt.Println("  Base path:  (current directory)")
			}
		}
		fmt.Println()

		if !isInteractive {
			return nil
		}
		fmt.Print("Overwrite with a new config? [y/N]: ")
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
	fmt.Println("Fetching Jira projects...")
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

	fmt.Printf("Fetching Jira boards for project %s...\n", cfg.Project)
	boards, err := client.ListBoards(context.Background(), cfg.Project)
	if err != nil {
		fmt.Println("Warning: unable to list boards, skipping board selection:", err)
	} else if len(boards) > 0 {
		fmt.Printf("Select a default board for project %s:\n", cfg.Project)
		for i, b := range boards {
			fmt.Printf("  %d) %d - %s\n", i+1, b.ID, b.Name)
		}
		fmt.Print("Board number (leave empty to skip): ")
		rawBoard, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		rawBoard = strings.TrimSpace(rawBoard)
		if rawBoard != "" {
			selectedBoard, err := strconv.Atoi(rawBoard)
			if err != nil || selectedBoard < 1 || selectedBoard > len(boards) {
				return errors.New("invalid board selection")
			}
			cfg.BoardID = boards[selectedBoard-1].ID
			cfg.BoardByProject = map[string]int{
				cfg.Project: cfg.BoardID,
			}
			cfg.BoardNameByProject = map[string]string{
				cfg.Project: boards[selectedBoard-1].Name,
			}
		}
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
	return config.Remove()
}

type RmSprintCmd struct {
	Sprint string `arg:"" help:"Sprint name."`
}

func (c *RmSprintCmd) Run(ctx *Context) error {
	if strings.TrimSpace(c.Sprint) == "" || c.Sprint == "." {
		return fmt.Errorf("invalid sprint %q", c.Sprint)
	}
	basePath, err := ctx.ProjectPath()
	if err != nil {
		return err
	}
	baseAbs, err := filepath.Abs(basePath)
	if err != nil {
		return err
	}
	baseResolved, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return err
	}
	target := filepath.Join(baseAbs, c.Sprint)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	targetResolved, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(baseResolved, targetResolved)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid sprint path %q", c.Sprint)
	}
	info, err := os.Stat(targetResolved)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("sprint %q is not a directory", c.Sprint)
	}
	return os.RemoveAll(targetResolved)
}

type RmTicketCmd struct {
	ID string `arg:"" help:"Ticket ID."`
}

func (c *RmTicketCmd) Run(ctx *Context) error {
	id := strings.TrimSpace(c.ID)
	if !isTicketID(id) {
		return fmt.Errorf("invalid ticket id %q", c.ID)
	}
	basePath, err := ctx.ProjectPath()
	if err != nil {
		return err
	}
	wantNames := map[string]struct{}{
		id:         {},
		id + ".md": {},
	}
	found := make([]string, 0)
	err = filepath.WalkDir(basePath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}
		if strings.Count(rel, string(filepath.Separator)) < 1 {
			return nil
		}
		if _, ok := wantNames[d.Name()]; ok {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return fmt.Errorf("ticket %q not found", id)
	}
	for _, f := range found {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	return nil
}

var ticketIDPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-[0-9]+$`)

func isTicketID(id string) bool {
	return ticketIDPattern.MatchString(strings.TrimSpace(id))
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
	client, err := ctx.JiraClient()
	if err != nil {
		return err
	}
	projectKey, err := ensureProject(ctx, client)
	if err != nil {
		return err
	}
	boardID, err := ensureBoard(ctx, client, projectKey)
	if err != nil {
		return err
	}
	sprints, err := client.ListSprints(context.Background(), boardID)
	if err != nil {
		return err
	}

	if c.Sprint == "" {
		for _, sprint := range sprints {
			fmt.Println(sprint.Name)
		}
		return nil
	}
	var selected *jira.Sprint
	for i := range sprints {
		if strings.EqualFold(sprints[i].Name, c.Sprint) || strconv.Itoa(sprints[i].ID) == c.Sprint {
			selected = &sprints[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("sprint %q not found", c.Sprint)
	}
	list, err := client.ListSprintTickets(context.Background(), boardID, selected.ID)
	if err != nil {
		return err
	}
	for i, ticket := range list {
		fmt.Printf("%s %s\n", ticket.ID, ticket.Title)
		if !c.Verbose {
			continue
		}
		fmt.Printf("  assignee: %s | reporter: %s\n", emptyFallback(ticket.Assignee), emptyFallback(ticket.Reporter))
		fmt.Printf("  state: %s\n", emptyFallback(ticket.State))
		if ticket.PRLink != "" {
			fmt.Printf("  pr: %s\n", ticket.PRLink)
		}
		if i < len(list)-1 {
			fmt.Println()
		}
	}
	return nil
}

func ensureProject(ctx *Context, client *jira.Client) (string, error) {
	if strings.TrimSpace(ctx.CLI.Cfg.Project) != "" {
		return ctx.CLI.Cfg.Project, nil
	}
	if !stdinIsTerminal() {
		return "", errors.New("no project configured; run jira configure or configure in interactive mode")
	}
	projects, err := client.ListProjects(context.Background())
	if err != nil {
		return "", err
	}
	if len(projects) == 0 {
		return "", errors.New("no Jira projects available")
	}
	fmt.Println("Select project:")
	for i, p := range projects {
		fmt.Printf("  %d) %s - %s\n", i+1, p.Key, p.Name)
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Project number: ")
	raw, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	selected, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || selected < 1 || selected > len(projects) {
		return "", errors.New("invalid project selection")
	}
	cfg := ctx.CLI.Cfg
	cfg.Project = projects[selected-1].Key
	if err := config.Save(cfg); err != nil {
		return "", err
	}
	ctx.CLI.Cfg = cfg
	return cfg.Project, nil
}

func ensureBoard(ctx *Context, client *jira.Client, projectKey string) (int, error) {
	boards, err := client.ListBoards(context.Background(), projectKey)
	if err != nil {
		return 0, err
	}
	isValidBoard := func(id int) bool {
		for _, board := range boards {
			if board.ID == id {
				return true
			}
		}
		return false
	}
	if ctx.CLI.Cfg.BoardByProject != nil && ctx.CLI.Cfg.BoardByProject[projectKey] > 0 {
		id := ctx.CLI.Cfg.BoardByProject[projectKey]
		if isValidBoard(id) {
			if ctx.CLI.Cfg.BoardNameByProject == nil || strings.TrimSpace(ctx.CLI.Cfg.BoardNameByProject[projectKey]) == "" {
				cfg := ctx.CLI.Cfg
				if cfg.BoardNameByProject == nil {
					cfg.BoardNameByProject = map[string]string{}
				}
				cfg.BoardNameByProject[projectKey] = boardNameByID(boards, id)
				if err := config.Save(cfg); err != nil {
					return 0, err
				}
				ctx.CLI.Cfg = cfg
			}
			return id, nil
		}
		cfg := ctx.CLI.Cfg
		delete(cfg.BoardByProject, projectKey)
		if cfg.BoardNameByProject != nil {
			delete(cfg.BoardNameByProject, projectKey)
		}
		if err := config.Save(cfg); err != nil {
			return 0, err
		}
		ctx.CLI.Cfg = cfg
	}
	if ctx.CLI.Cfg.BoardID > 0 && isValidBoard(ctx.CLI.Cfg.BoardID) {
		cfg := ctx.CLI.Cfg
		if cfg.BoardByProject == nil {
			cfg.BoardByProject = map[string]int{}
		}
		cfg.BoardByProject[projectKey] = cfg.BoardID
		if cfg.BoardNameByProject == nil {
			cfg.BoardNameByProject = map[string]string{}
		}
		cfg.BoardNameByProject[projectKey] = boardNameByID(boards, cfg.BoardID)
		if err := config.Save(cfg); err != nil {
			return 0, err
		}
		ctx.CLI.Cfg = cfg
		return cfg.BoardID, nil
	}

	if len(boards) == 0 {
		return 0, fmt.Errorf("no Jira boards found for project %s", projectKey)
	}
	if !stdinIsTerminal() {
		return 0, errors.New("no board configured; run jira configure --board-id <id> or use interactive mode")
	}
	fmt.Printf("Select board for project %s:\n", projectKey)
	for i, board := range boards {
		fmt.Printf("  %d) %d - %s\n", i+1, board.ID, board.Name)
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Board number: ")
	raw, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	selected, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || selected < 1 || selected > len(boards) {
		return 0, errors.New("invalid board selection")
	}
	cfg := ctx.CLI.Cfg
	cfg.BoardID = boards[selected-1].ID
	if cfg.BoardByProject == nil {
		cfg.BoardByProject = map[string]int{}
	}
	cfg.BoardByProject[projectKey] = cfg.BoardID
	if cfg.BoardNameByProject == nil {
		cfg.BoardNameByProject = map[string]string{}
	}
	cfg.BoardNameByProject[projectKey] = boards[selected-1].Name
	if err := config.Save(cfg); err != nil {
		return 0, err
	}
	ctx.CLI.Cfg = cfg
	return cfg.BoardID, nil
}

func boardNameByID(boards []jira.Board, id int) string {
	for _, board := range boards {
		if board.ID == id {
			return board.Name
		}
	}
	return ""
}

func emptyFallback(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
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
	client, err := ctx.JiraClient()
	if err != nil {
		return err
	}
	ticket, err := client.GetTicket(context.Background(), c.ID)
	if err != nil {
		return err
	}
	fmt.Printf("Ticket:   %s\n", ticket.ID)
	fmt.Printf("State:    %s\n", emptyFallback(ticket.State))
	fmt.Printf("Assignee: %s\n\n", emptyFallback(ticket.Assignee))

	transitions, err := client.GetTransitions(context.Background(), c.ID)
	if err != nil {
		return err
	}
	if len(transitions) == 0 {
		return fmt.Errorf("no transitions available for ticket %s", c.ID)
	}

	fmt.Printf("Available transitions for %s:\n", c.ID)
	for i, t := range transitions {
		fmt.Printf("  %d) %s\n", i+1, t.Name)
	}

	if !stdinIsTerminal() {
		return errors.New("transition must be selected interactively; stdin is not a terminal")
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Select transition: ")
	raw, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	selected, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || selected < 1 || selected > len(transitions) {
		return errors.New("invalid transition selection")
	}

	chosen := transitions[selected-1]
	if err := client.DoTransition(context.Background(), c.ID, chosen.ID); err != nil {
		return err
	}
	fmt.Printf("%s moved to %q\n", c.ID, chosen.Name)
	return nil
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

package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

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
	parser, err := newParser(&a.cli)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		ctx, err := kong.Trace(parser, args)
		if err != nil {
			return err
		}
		return ctx.PrintUsage(false)
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

func newParser(cli *CLI) (*kong.Kong, error) {
	return kong.New(cli,
		kong.Name("jira"),
		kong.Description("Jira CLI to fetch and sync tickets to markdown."),
		kong.Help(normalizedHelpPrinter(kong.DefaultHelpPrinter)),
		kong.ShortHelp(normalizedHelpPrinter(kong.DefaultShortHelpPrinter)),
	)
}

func normalizedHelpPrinter(printer kong.HelpPrinter) kong.HelpPrinter {
	return func(options kong.HelpOptions, ctx *kong.Context) error {
		origStdout := ctx.Stdout
		var buf bytes.Buffer
		ctx.Stdout = &buf
		defer func() {
			ctx.Stdout = origStdout
		}()

		if err := printer(options, ctx); err != nil {
			return err
		}

		_, err := io.WriteString(origStdout, normalizeHelpOutput(buf.String(), ctx.Model.Name))
		return err
	}
}

func normalizeHelpOutput(output, appName string) string {
	lines := strings.Split(output, "\n")
	rootUsagePrefix := "Usage: " + appName + " <command>"

	for i, line := range lines {
		line = strings.ReplaceAll(line, "[<", "<")
		line = strings.ReplaceAll(line, ">]", ">")
		if strings.HasPrefix(line, "  ") {
			line = strings.ReplaceAll(line, " [flags]", "")
		}
		if strings.HasPrefix(line, "Usage: "+appName+" ") && !strings.HasPrefix(line, rootUsagePrefix) {
			line = strings.ReplaceAll(line, " [flags]", "")
		}
		lines[i] = line
	}

	return strings.Join(lines, "\n")
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

func (c *Context) JiraClientForUpdateCache(update bool) (*jira.Client, error) {
	client, err := c.JiraClient()
	if err != nil {
		return nil, err
	}
	client.RefreshCache = update
	return client, nil
}

func (c *Context) ProjectPath() (string, error) {
	if c.CLI.Cfg.BasePath == "" {
		return os.Getwd()
	}
	return expandPath(c.CLI.Cfg.BasePath)
}

type TestCmd struct{}

func (t *TestCmd) Run(ctx *Context) error {
	client, err := ctx.JiraClientForUpdateCache(true)
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
	Target string `arg:"" optional:"" help:"Config, sprint name, or ticket ID."`
	Config bool   `help:"Remove the config file."`
	Sprint bool   `help:"Treat target as a sprint name."`
	Ticket bool   `help:"Treat target as a ticket ID."`
}

func (c *RmCmd) Run(ctx *Context) error {
	target := strings.TrimSpace(c.Target)
	explicit := 0
	if c.Config {
		explicit++
	}
	if c.Sprint {
		explicit++
	}
	if c.Ticket {
		explicit++
	}
	if explicit > 1 {
		return errors.New("choose only one of --config, --sprint, or --ticket")
	}
	if c.Config {
		if target != "" && !strings.EqualFold(target, "config") {
			return errors.New("--config does not accept a sprint or ticket target")
		}
		return (&RmConfigCmd{}).Run(ctx)
	}
	if target == "" {
		return errors.New("provide a sprint name or ticket ID")
	}
	if (target == "." || target == "..") && !c.Config && !c.Ticket {
		return fmt.Errorf("invalid sprint %q", target)
	}
	basePath, err := ctx.ProjectPath()
	if err != nil {
		return err
	}
	isTicket := isTicketID(target)
	if c.Sprint {
		sprintPath, _, err := resolveSprintRemovalSelection(basePath, target, bufio.NewReader(os.Stdin), os.Stdout, stdinIsTerminal())
		if err != nil {
			return err
		}
		if sprintPath == "" {
			return fmt.Errorf("sprint %q not found", target)
		}
		return removeSprintPath(basePath, target, sprintPath)
	}
	if c.Ticket {
		return (&RmTicketCmd{ID: target}).Run(ctx)
	}
	if strings.EqualFold(target, "config") {
		if sprintExists(basePath, target) {
			kind, err := resolveRemovalTarget(target, []string{"config", "sprint"}, bufio.NewReader(os.Stdin), os.Stdout, stdinIsTerminal())
			if err != nil {
				return err
			}
			if kind == "sprint" {
				sprintPath, _, err := resolveSprintRemovalSelection(basePath, target, bufio.NewReader(os.Stdin), os.Stdout, stdinIsTerminal())
				if err != nil {
					return err
				}
				if sprintPath == "" {
					return fmt.Errorf("sprint %q not found", target)
				}
				return removeSprintPath(basePath, target, sprintPath)
			}
		}
		return (&RmConfigCmd{}).Run(ctx)
	}
	if isTicket {
		sprintPath, _, err := resolveExactSprintRemovalSelection(basePath, target)
		if err != nil {
			return err
		}
		if sprintPath != "" {
			kind, err := resolveRemovalTarget(target, []string{"sprint", "ticket"}, bufio.NewReader(os.Stdin), os.Stdout, stdinIsTerminal())
			if err != nil {
				return err
			}
			if kind == "ticket" {
				return (&RmTicketCmd{ID: target}).Run(ctx)
			}
			return removeSprintPath(basePath, target, sprintPath)
		}
		return (&RmTicketCmd{ID: target}).Run(ctx)
	}
	sprintPath, _, err := resolveSprintRemovalSelection(basePath, target, bufio.NewReader(os.Stdin), os.Stdout, stdinIsTerminal())
	if err != nil {
		return err
	}
	if sprintPath == "" {
		return fmt.Errorf("sprint %q not found", target)
	}
	return removeSprintPath(basePath, target, sprintPath)
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
	sprintPath, _, err := resolveSprintRemovalSelection(basePath, c.Sprint, bufio.NewReader(os.Stdin), os.Stdout, stdinIsTerminal())
	if err != nil {
		return err
	}
	if sprintPath == "" {
		return fmt.Errorf("sprint %q not found", c.Sprint)
	}
	return removeSprintPath(basePath, c.Sprint, sprintPath)
}

func removeSprintPath(basePath, target, sprintPath string) error {
	baseAbs, err := filepath.Abs(basePath)
	if err != nil {
		return err
	}
	baseResolved, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(sprintPath)
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
		return fmt.Errorf("invalid sprint path %q", target)
	}
	info, err := os.Stat(targetResolved)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("sprint %q is not a directory", target)
	}
	return os.RemoveAll(targetResolved)
}

type RmTicketCmd struct {
	ID string `arg:"" help:"Ticket ID."`
}

func (c *RmTicketCmd) Run(ctx *Context) error {
	id := strings.ToUpper(strings.TrimSpace(c.ID))
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

func sprintExists(basePath, sprint string) bool {
	path, _, err := resolveExactSprintRemovalSelection(basePath, sprint)
	if err != nil {
		return false
	}
	return path != ""
}

func resolveRemovalTarget(target string, options []string, reader *bufio.Reader, out io.Writer, interactive bool) (string, error) {
	if !interactive {
		return "", fmt.Errorf("target %q is ambiguous; rerun with an explicit removal flag", target)
	}
	fmt.Fprintf(out, "Target %q is ambiguous:\n", target)
	labels := map[string]string{
		"config": "Remove config file",
		"sprint": "Remove sprint folder",
		"ticket": "Remove ticket file",
	}
	for i, option := range options {
		fmt.Fprintf(out, "  %d) %s\n", i+1, labels[option])
	}
	fmt.Fprint(out, "Selection: ")
	raw, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	selected, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || selected < 1 || selected > len(options) {
		return "", errors.New("invalid removal selection")
	}
	return options[selected-1], nil
}

func resolveSprintRemovalSelection(basePath, target string, reader *bufio.Reader, out io.Writer, interactive bool) (string, []string, error) {
	matches, err := findSprintRemovalMatches(basePath, target)
	if err != nil {
		return "", nil, err
	}
	if len(matches) == 0 {
		return "", nil, nil
	}
	if len(matches) == 1 {
		return matches[0].Path, []string{matches[0].Name}, nil
	}
	if !interactive {
		names := sprintRemovalMatchNames(matches)
		return "", names, fmt.Errorf("sprint %q is ambiguous; matches: %s", strings.TrimSpace(target), strings.Join(names, ", "))
	}
	selected, err := pickSprintRemovalMatch(matches, reader, out, target)
	if err != nil {
		return "", nil, err
	}
	return selected.Path, sprintRemovalMatchNames(matches), nil
}

func resolveExactSprintRemovalSelection(basePath, target string) (string, []string, error) {
	matches, err := findExactSprintRemovalMatches(basePath, target)
	if err != nil {
		return "", nil, err
	}
	if len(matches) == 0 {
		return "", nil, nil
	}
	return matches[0].Path, sprintRemovalMatchNames(matches), nil
}

type sprintRemovalMatch struct {
	Name string
	Path string
}

func findSprintRemovalMatches(basePath, target string) ([]sprintRemovalMatch, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, err
	}
	exact := make([]sprintRemovalMatch, 0, 1)
	approximate := make([]sprintRemovalMatch, 0)
	lowerTarget := strings.ToLower(target)
	normalizedTarget := normalizeSprintLookup(target)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(basePath, name)
		match := sprintRemovalMatch{Name: name, Path: path}
		if strings.EqualFold(name, target) {
			exact = append(exact, match)
			continue
		}
		if strings.Contains(strings.ToLower(name), lowerTarget) || (normalizedTarget != "" && strings.Contains(normalizeSprintLookup(name), normalizedTarget)) {
			approximate = append(approximate, match)
		}
	}
	if len(exact) > 0 {
		return exact, nil
	}
	sort.SliceStable(approximate, func(i, j int) bool {
		return strings.ToLower(approximate[i].Name) < strings.ToLower(approximate[j].Name)
	})
	return approximate, nil
}

func findExactSprintRemovalMatches(basePath, target string) ([]sprintRemovalMatch, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, err
	}
	exact := make([]sprintRemovalMatch, 0, 1)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !strings.EqualFold(entry.Name(), target) {
			continue
		}
		exact = append(exact, sprintRemovalMatch{
			Name: entry.Name(),
			Path: filepath.Join(basePath, entry.Name()),
		})
	}
	return exact, nil
}

func sprintRemovalMatchNames(matches []sprintRemovalMatch) []string {
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match.Name)
	}
	return names
}

func pickSprintRemovalMatch(matches []sprintRemovalMatch, reader *bufio.Reader, out io.Writer, target string) (sprintRemovalMatch, error) {
	filtered := matches
	query := ""
	for {
		if query != "" {
			filtered = filterSprintRemovalMatches(matches, query)
			if len(filtered) == 0 {
				fmt.Fprintf(out, "  (no sprint matches for %q)\n", query)
				query = ""
				filtered = matches
				continue
			}
		}
		fmt.Fprintf(out, "Sprint %q matches multiple folders:\n", strings.TrimSpace(target))
		for i, match := range filtered {
			fmt.Fprintf(out, "  %d) %s\n", i+1, match.Name)
		}
		fmt.Fprint(out, "Enter number to select, or type a search term to filter (empty to cancel): ")
		raw, err := reader.ReadString('\n')
		if err != nil {
			return sprintRemovalMatch{}, err
		}
		input := strings.TrimSpace(raw)
		if input == "" {
			return sprintRemovalMatch{}, errors.New("removal cancelled")
		}
		if n, err := strconv.Atoi(input); err == nil {
			if n < 1 || n > len(filtered) {
				fmt.Fprintf(out, "  (invalid number, valid range is 1-%d)\n", len(filtered))
				continue
			}
			return filtered[n-1], nil
		}
		query = input
	}
}

func filterSprintRemovalMatches(matches []sprintRemovalMatch, query string) []sprintRemovalMatch {
	query = strings.TrimSpace(query)
	if query == "" {
		return matches
	}
	lowerQuery := strings.ToLower(query)
	normalizedQuery := normalizeSprintLookup(query)
	filtered := make([]sprintRemovalMatch, 0, len(matches))
	for _, match := range matches {
		if strings.Contains(strings.ToLower(match.Name), lowerQuery) || (normalizedQuery != "" && strings.Contains(normalizeSprintLookup(match.Name), normalizedQuery)) {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

var ticketIDPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-[0-9]+$`)
var sprintNumberPattern = regexp.MustCompile(`\d+`)
var sprintDisplayPattern = regexp.MustCompile(`^(.*)\((\d+)\)\s*$`)

func isTicketID(id string) bool {
	return ticketIDPattern.MatchString(strings.ToUpper(strings.TrimSpace(id)))
}

func findSprint(sprints []jira.Sprint, input string) (*jira.Sprint, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}
	for i := range sprints {
		if strings.EqualFold(sprints[i].Name, trimmed) || strconv.Itoa(sprints[i].ID) == trimmed {
			return &sprints[i], nil
		}
	}
	if sprint := findDisplayedSprint(sprints, trimmed); sprint != nil {
		return sprint, nil
	}
	if !isDigits(trimmed) {
		return nil, nil
	}
	matches := make([]string, 0, 1)
	var selected *jira.Sprint
	for i := range sprints {
		for _, part := range sprintNumberPattern.FindAllString(sprints[i].Name, -1) {
			if part == trimmed {
				matches = append(matches, sprints[i].Name)
				selected = &sprints[i]
				break
			}
		}
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("sprint %q is ambiguous; matches: %s", trimmed, strings.Join(matches, ", "))
	}
	return selected, nil
}

func resolveSprintSelection(sprints []jira.Sprint, input string, reader *bufio.Reader, out io.Writer, interactive bool) (*jira.Sprint, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}
	if selected := findExactSprint(sprints, trimmed); selected != nil {
		return selected, nil
	}
	matches := findApproximateSprints(sprints, trimmed)
	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	}
	if !interactive {
		names := make([]string, 0, len(matches))
		for _, sprint := range matches {
			names = append(names, sprint.Name)
		}
		return nil, fmt.Errorf("sprint %q is ambiguous; matches: %s", trimmed, strings.Join(names, ", "))
	}
	return pickSprintMatch(matches, reader, out, trimmed)
}

func pickSprintMatch(matches []jira.Sprint, reader *bufio.Reader, out io.Writer, target string) (*jira.Sprint, error) {
	filtered := matches
	query := ""
	for {
		if query != "" {
			filtered = filterSprintMatches(matches, query)
			if len(filtered) == 0 {
				fmt.Fprintf(out, "  (no sprint matches for %q)\n", query)
				query = ""
				filtered = matches
				continue
			}
		}
		fmt.Fprintf(out, "Sprint %q matches multiple sprints:\n", strings.TrimSpace(target))
		for i, sprint := range filtered {
			fmt.Fprintf(out, "  %d) %s (%d)\n", i+1, sprint.Name, sprint.ID)
		}
		fmt.Fprint(out, "Enter number to select, or type a search term to filter (empty to cancel): ")
		raw, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		input := strings.TrimSpace(raw)
		if input == "" {
			return nil, errors.New("selection cancelled")
		}
		if n, err := strconv.Atoi(input); err == nil {
			if n < 1 || n > len(filtered) {
				fmt.Fprintf(out, "  (invalid number, valid range is 1-%d)\n", len(filtered))
				continue
			}
			return &filtered[n-1], nil
		}
		query = input
	}
}

func filterSprintMatches(matches []jira.Sprint, query string) []jira.Sprint {
	query = strings.TrimSpace(query)
	if query == "" {
		return matches
	}
	lowerQuery := strings.ToLower(query)
	normalizedQuery := normalizeSprintLookup(query)
	filtered := make([]jira.Sprint, 0, len(matches))
	for _, sprint := range matches {
		if strings.Contains(strings.ToLower(sprint.Name), lowerQuery) || (normalizedQuery != "" && strings.Contains(normalizeSprintLookup(sprint.Name), normalizedQuery)) {
			filtered = append(filtered, sprint)
		}
	}
	return filtered
}

func isDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
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

var stdinIsTerminalFunc = stdinIsTerminal

type FetchCmd struct {
	Target string `arg:"" optional:"" help:"Sprint name or ticket ID to fetch."`
	Ticket string `name:"ticket" help:"Explicitly fetch one ticket by ID."`
	Sprint string `name:"sprint" help:"Explicitly fetch one sprint by name or ID."`
	Year   int    `name:"year" help:"Restrict fetch to sprints in the given four digit year."`
}

func (c *FetchCmd) Run(ctx *Context) error {
	if selectedFetchModes(c) > 1 {
		return errors.New("provide only one of positional target, --ticket, or --sprint")
	}
	if c.Year != 0 && (c.Year < 1000 || c.Year > 9999) {
		return fmt.Errorf("invalid year %d: use a four digit year", c.Year)
	}
	basePath, err := ctx.ProjectPath()
	if err != nil {
		return err
	}
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
	boardID, sprints, err := resolveBoardSprints(ctx, client, projectKey, boardID)
	if err != nil {
		return err
	}
	sprints = filterSprintsByYear(sprints, c.Year)
	if len(sprints) == 0 {
		if c.Year != 0 {
			return fmt.Errorf("no sprints found for year %d", c.Year)
		}
		return errors.New("no sprints found")
	}
	targets, ticketID, err := c.resolveFetchTargets(sprints)
	if err != nil {
		return err
	}
	return c.fetchResolvedTargets(basePath, client, boardID, targets, ticketID)
}

func selectedFetchModes(c *FetchCmd) int {
	count := 0
	if strings.TrimSpace(c.Target) != "" {
		count++
	}
	if strings.TrimSpace(c.Ticket) != "" {
		count++
	}
	if strings.TrimSpace(c.Sprint) != "" {
		count++
	}
	return count
}

func (c *FetchCmd) resolveFetchTargets(sprints []jira.Sprint) ([]jira.Sprint, string, error) {
	if ticketID := strings.TrimSpace(c.Ticket); ticketID != "" {
		return sprints, strings.ToUpper(ticketID), nil
	}
	if sprintTarget := strings.TrimSpace(c.Sprint); sprintTarget != "" {
		selected, err := resolveFetchSprintSelection(sprints, sprintTarget, bufio.NewReader(os.Stdin), os.Stdout, stdinIsTerminalFunc())
		if err != nil {
			return nil, "", err
		}
		if selected == nil {
			return nil, "", fmt.Errorf("sprint %q not found", sprintTarget)
		}
		return []jira.Sprint{*selected}, "", nil
	}

	target := strings.TrimSpace(c.Target)
	if target == "" {
		selected, err := selectedSprints(sprints, "")
		return selected, "", err
	}
	if sprint, err := findSprint(sprints, target); err != nil {
		return nil, "", err
	} else if sprint != nil && isTicketID(target) {
		return nil, "", fmt.Errorf("target %q is ambiguous; use --ticket or --sprint", target)
	}
	if isTicketID(target) {
		return sprints, strings.ToUpper(target), nil
	}
	selected, err := resolveFetchSprintSelection(sprints, target, bufio.NewReader(os.Stdin), os.Stdout, stdinIsTerminalFunc())
	if err != nil {
		return nil, "", err
	}
	if selected == nil {
		return nil, "", fmt.Errorf("sprint %q not found", target)
	}
	return []jira.Sprint{*selected}, "", nil
}

func (c *FetchCmd) fetchResolvedTargets(basePath string, client *jira.Client, boardID int, targets []jira.Sprint, ticketID string) error {
	if ticketID != "" {
		fmt.Printf("retrieving data for ticket %s\n", ticketID)
	}
	fmt.Printf("retrieving data for sprints: %s\n", joinSprintNames(targets))
	sprintTickets, err := collectSprintTickets(context.Background(), client, boardID, targets)
	if err != nil {
		return err
	}
	ticketToSprints := mapTicketsToSprints(targets, sprintTickets)
	if ticketID != "" {
		if _, ok := ticketToSprints[ticketID]; !ok {
			return fmt.Errorf("ticket %q not found in configured board sprints", ticketID)
		}
	}
	tickets, err := client.SearchTicketsBySprintIDs(context.Background(), sprintIDs(targets))
	if err != nil {
		return err
	}
	byID := make(map[string]jira.IssueTicket, len(tickets))
	for _, ticket := range tickets {
		byID[strings.ToUpper(ticket.ID)] = ticket
	}
	if ticketID != "" {
		ticket, ok := byID[ticketID]
		if !ok {
			return fmt.Errorf("ticket %q not found", ticketID)
		}
		for _, sprint := range ticketToSprints[ticketID] {
			fmt.Printf("writing markdown files for sprint %s\n", sprint.Name)
			if err := writeFetchedTicket(basePath, sprint, ticket); err != nil {
				return err
			}
		}
		folders := make([]string, 0, len(ticketToSprints[ticketID]))
		for _, sprint := range ticketToSprints[ticketID] {
			folders = append(folders, sprintFolderName(sprint))
		}
		fmt.Printf("fetched %s into %s\n", ticket.ID, strings.Join(folders, ", "))
		return nil
	}

	fetched := 0
	for _, sprint := range targets {
		items := sprintTickets[sprint.ID]
		fmt.Printf("writing markdown files for sprint %s\n", sprint.Name)
		for _, item := range items {
			ticket, ok := byID[strings.ToUpper(item.ID)]
			if !ok {
				return fmt.Errorf("ticket %q not found", item.ID)
			}
			if err := writeFetchedTicket(basePath, sprint, ticket); err != nil {
				return err
			}
			fetched++
		}
		fmt.Printf("fetched sprint %s (%d ticket(s))\n", sprint.Name, len(items))
	}
	if len(targets) == 1 {
		fmt.Printf("fetched %d ticket(s) for %s\n", fetched, targets[0].Name)
		return nil
	}
	fmt.Printf("fetched %d ticket(s) across %d sprint(s)\n", fetched, len(targets))
	return nil
}

func resolveFetchSprintSelection(sprints []jira.Sprint, input string, reader *bufio.Reader, out io.Writer, interactive bool) (*jira.Sprint, error) {
	targets, err := selectedSprints(sprints, input)
	if err == nil {
		if len(targets) == 0 {
			return nil, nil
		}
		return &targets[0], nil
	}
	if !interactive || !strings.Contains(err.Error(), "ambiguous") {
		return nil, err
	}

	trimmed := strings.TrimSpace(input)
	matches := findApproximateSprints(latestSprints(sprints, 10), trimmed)
	if len(matches) <= 1 {
		matches = findApproximateSprints(sprints, trimmed)
	}
	if len(matches) <= 1 {
		return nil, err
	}
	return pickSprintMatch(matches, reader, out, trimmed)
}

func findExactSprint(sprints []jira.Sprint, query string) *jira.Sprint {
	query = strings.TrimSpace(query)
	for i := range sprints {
		if strings.EqualFold(sprints[i].Name, query) || strconv.Itoa(sprints[i].ID) == query {
			return &sprints[i]
		}
	}
	return findDisplayedSprint(sprints, query)
}

func findDisplayedSprint(sprints []jira.Sprint, query string) *jira.Sprint {
	namePart, idPart, ok := parseSprintDisplay(query)
	if !ok {
		return nil
	}
	for i := range sprints {
		if strconv.Itoa(sprints[i].ID) == idPart && strings.EqualFold(strings.TrimSpace(sprints[i].Name), namePart) {
			return &sprints[i]
		}
	}
	return nil
}

func selectedSprints(sprints []jira.Sprint, query string) ([]jira.Sprint, error) {
	if strings.TrimSpace(query) == "" {
		return sprints, nil
	}
	if sprint := findExactSprint(sprints, query); sprint != nil {
		return []jira.Sprint{*sprint}, nil
	}
	recent := latestSprints(sprints, 10)
	if matches := findApproximateSprints(recent, query); len(matches) == 1 {
		return []jira.Sprint{matches[0]}, nil
	} else if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, sprint := range matches {
			names = append(names, sprint.Name)
		}
		return nil, fmt.Errorf("sprint %q is ambiguous; matches: %s", query, strings.Join(names, ", "))
	}
	if matches := findApproximateSprints(sprints, query); len(matches) == 1 {
		return []jira.Sprint{matches[0]}, nil
	} else if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, sprint := range matches {
			names = append(names, sprint.Name)
		}
		return nil, fmt.Errorf("sprint %q is ambiguous; matches: %s", query, strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("sprint %q not found", query)
}

func latestSprints(sprints []jira.Sprint, limit int) []jira.Sprint {
	if limit <= 0 || len(sprints) <= limit {
		out := append([]jira.Sprint(nil), sprints...)
		sort.SliceStable(out, func(i, j int) bool {
			return sprintSortDate(out[i]).After(sprintSortDate(out[j]))
		})
		return out
	}
	out := append([]jira.Sprint(nil), sprints...)
	sort.SliceStable(out, func(i, j int) bool {
		return sprintSortDate(out[i]).After(sprintSortDate(out[j]))
	})
	return out[:limit]
}

func sprintSortDate(sprint jira.Sprint) time.Time {
	for _, value := range []time.Time{sprint.CompleteDate, sprint.EndDate, sprint.StartDate, sprint.CreatedDate} {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func filterSprintsByYear(sprints []jira.Sprint, year int) []jira.Sprint {
	if year == 0 {
		return sprints
	}
	filtered := make([]jira.Sprint, 0, len(sprints))
	for _, sprint := range sprints {
		if sprintMatchesYear(sprint, year) {
			filtered = append(filtered, sprint)
		}
	}
	return filtered
}

func sprintMatchesYear(sprint jira.Sprint, year int) bool {
	for _, value := range []time.Time{sprint.StartDate, sprint.EndDate, sprint.CompleteDate} {
		if !value.IsZero() && value.Year() == year {
			return true
		}
	}
	return false
}

func findApproximateSprints(sprints []jira.Sprint, query string) []jira.Sprint {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	lowerQuery := strings.ToLower(query)
	normalizedQuery := normalizeSprintLookup(query)
	matches := make([]jira.Sprint, 0)
	seen := make(map[int]struct{})
	for _, sprint := range sprints {
		nameLower := strings.ToLower(sprint.Name)
		normalizedName := normalizeSprintLookup(sprint.Name)
		if strings.Contains(nameLower, lowerQuery) || (normalizedQuery != "" && strings.Contains(normalizedName, normalizedQuery)) {
			if _, ok := seen[sprint.ID]; ok {
				continue
			}
			seen[sprint.ID] = struct{}{}
			matches = append(matches, sprint)
		}
	}
	return matches
}

func normalizeSprintLookup(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func parseSprintDisplay(value string) (string, string, bool) {
	matches := sprintDisplayPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 3 {
		return "", "", false
	}
	return strings.TrimSpace(matches[1]), matches[2], true
}

func collectSprintTickets(ctx context.Context, client *jira.Client, boardID int, sprints []jira.Sprint) (map[int][]jira.IssueTicket, error) {
	collected := make(map[int][]jira.IssueTicket, len(sprints))
	for _, sprint := range sprints {
		items, err := client.ListSprintTickets(ctx, boardID, sprint.ID)
		if err != nil {
			return nil, err
		}
		collected[sprint.ID] = items
	}
	return collected, nil
}

func joinSprintNames(sprints []jira.Sprint) string {
	names := make([]string, 0, len(sprints))
	for _, sprint := range sprints {
		names = append(names, sprint.Name)
	}
	return strings.Join(names, ", ")
}

func mapTicketsToSprints(sprints []jira.Sprint, sprintTickets map[int][]jira.IssueTicket) map[string][]jira.Sprint {
	sprintByID := make(map[int]jira.Sprint, len(sprints))
	for _, sprint := range sprints {
		sprintByID[sprint.ID] = sprint
	}
	out := make(map[string][]jira.Sprint)
	for sprintID, items := range sprintTickets {
		sprint := sprintByID[sprintID]
		for _, item := range items {
			key := strings.ToUpper(item.ID)
			out[key] = append(out[key], sprint)
		}
	}
	return out
}

func sprintIDs(sprints []jira.Sprint) []int {
	ids := make([]int, 0, len(sprints))
	for _, sprint := range sprints {
		ids = append(ids, sprint.ID)
	}
	return ids
}

func sprintFolderName(sprint jira.Sprint) string {
	name := strings.TrimSpace(sprint.Name)
	replacer := strings.NewReplacer("/", "-", "\\", "-")
	name = strings.TrimSpace(replacer.Replace(name))
	if name == "" || name == "." || name == ".." {
		return fmt.Sprintf("Sprint-%d", sprint.ID)
	}
	return name
}

func writeFetchedTicket(basePath string, sprint jira.Sprint, ticket jira.IssueTicket) error {
	dir, err := sprintOutputDir(basePath, sprint)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	targetPath := filepath.Join(dir, ticket.ID+".md")
	if info, err := os.Lstat(targetPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("ticket path %q must not be a symlink", ticket.ID)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("ticket path %q is not a regular file", ticket.ID)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	body := renderTicketMarkdown(ticket)
	return os.WriteFile(targetPath, []byte(body), 0o644)
}

func sprintOutputDir(basePath string, sprint jira.Sprint) (string, error) {
	baseAbs, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(baseAbs, 0o755); err != nil {
		return "", err
	}
	baseResolved, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return "", err
	}
	targetAbs := filepath.Join(baseResolved, sprintFolderName(sprint))
	if info, err := os.Lstat(targetAbs); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("sprint path %q must not be a symlink", sprint.Name)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("sprint path %q is not a directory", sprint.Name)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	rel, err := filepath.Rel(baseResolved, targetAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid sprint path %q", sprint.Name)
	}
	return targetAbs, nil
}

func renderTicketMarkdown(ticket jira.IssueTicket) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("id: %s\n", ticket.ID))
	b.WriteString(fmt.Sprintf("title: %s\n", ticket.Title))
	if ticket.Assignee != "" {
		b.WriteString(fmt.Sprintf("assignee: %s\n", ticket.Assignee))
	}
	if ticket.Reporter != "" {
		b.WriteString(fmt.Sprintf("reporter: %s\n", ticket.Reporter))
	}
	if ticket.State != "" {
		b.WriteString(fmt.Sprintf("workflow_state: %s\n", ticket.State))
	}
	if ticket.Priority != "" {
		b.WriteString(fmt.Sprintf("priority: %s\n", ticket.Priority))
	}
	if len(ticket.Labels) > 0 {
		b.WriteString(fmt.Sprintf("labels: %s\n", strings.Join(ticket.Labels, ", ")))
	}
	if ticket.PRLink != "" {
		b.WriteString(fmt.Sprintf("pull_request_url: %s\n", ticket.PRLink))
	}
	if ticket.URL != "" {
		b.WriteString(fmt.Sprintf("url: %s\n", ticket.URL))
	}
	b.WriteString("---\n")
	if strings.TrimSpace(ticket.Description) != "" {
		b.WriteString("\n")
		b.WriteString(ticket.Description)
		if !strings.HasSuffix(ticket.Description, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

type LsCmd struct {
	Sprint      string `arg:"" optional:"" help:"Sprint name."`
	Verbose     bool   `short:"v" help:"Verbose output."`
	UpdateCache bool   `help:"Refresh cached output for this command target."`
}

func (c *LsCmd) Run(ctx *Context) error {
	basePath, err := ctx.ProjectPath()
	if err != nil {
		return err
	}
	client, err := ctx.JiraClientForUpdateCache(c.UpdateCache)
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
	boardID, sprints, err := resolveBoardSprints(ctx, client, projectKey, boardID)
	if err != nil {
		return err
	}

	if c.Sprint == "" {
		output := renderSprintListOutput(basePath, sprints)
		fmt.Print(output)
		return nil
	}
	selected, err := resolveSprintSelection(sprints, c.Sprint, bufio.NewReader(os.Stdin), os.Stdout, stdinIsTerminalFunc())
	if err != nil {
		return err
	}
	if selected == nil {
		return fmt.Errorf("sprint %q not found", c.Sprint)
	}
	list, err := client.ListSprintTickets(context.Background(), boardID, selected.ID)
	if err != nil {
		return err
	}
	output := renderLsOutput(*selected, list, c.Verbose)
	fmt.Print(output)
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
	if ctx.CLI.Cfg.BoardByProject != nil && ctx.CLI.Cfg.BoardByProject[projectKey] > 0 {
		return ctx.CLI.Cfg.BoardByProject[projectKey], nil
	}
	if ctx.CLI.Cfg.BoardID > 0 {
		cfg := ctx.CLI.Cfg
		if cfg.BoardByProject == nil {
			cfg.BoardByProject = map[string]int{}
		}
		cfg.BoardByProject[projectKey] = cfg.BoardID
		ctx.CLI.Cfg = cfg
		return cfg.BoardID, nil
	}

	boards, err := client.ListBoards(context.Background(), projectKey)
	if err != nil {
		return 0, err
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

func resolveBoardSprints(ctx *Context, client *jira.Client, projectKey string, boardID int) (int, []jira.Sprint, error) {
	sprints, err := client.ListSprints(context.Background(), boardID)
	if err == nil {
		return boardID, sprints, nil
	}
	if !looksLikeNotFound(err) || !configuredBoardForProject(ctx, projectKey, boardID) {
		return 0, nil, err
	}
	if clearConfiguredBoard(ctx, projectKey, boardID) != nil {
		return 0, nil, err
	}
	boardID, retryErr := ensureBoard(ctx, client, projectKey)
	if retryErr != nil {
		return 0, nil, retryErr
	}
	sprints, retryErr = client.ListSprints(context.Background(), boardID)
	if retryErr != nil {
		return 0, nil, retryErr
	}
	return boardID, sprints, nil
}

func configuredBoardForProject(ctx *Context, projectKey string, boardID int) bool {
	if ctx == nil || ctx.CLI == nil {
		return false
	}
	if ctx.CLI.Cfg.BoardByProject != nil && ctx.CLI.Cfg.BoardByProject[projectKey] == boardID && boardID > 0 {
		return true
	}
	return ctx.CLI.Cfg.BoardID == boardID && boardID > 0
}

func clearConfiguredBoard(ctx *Context, projectKey string, boardID int) error {
	if ctx == nil || ctx.CLI == nil {
		return nil
	}
	cfg := ctx.CLI.Cfg
	changed := false
	if cfg.BoardByProject != nil && cfg.BoardByProject[projectKey] == boardID {
		delete(cfg.BoardByProject, projectKey)
		changed = true
	}
	if cfg.BoardNameByProject != nil {
		delete(cfg.BoardNameByProject, projectKey)
	}
	if cfg.BoardID == boardID {
		cfg.BoardID = 0
		changed = true
	}
	if !changed {
		ctx.CLI.Cfg = cfg
		return nil
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	ctx.CLI.Cfg = cfg
	return nil
}

func looksLikeNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "404") || strings.Contains(msg, "not found")
}

func emptyFallback(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

type CatCmd struct {
	Target      string `arg:"" help:"Sprint name or ticket ID."`
	UpdateCache bool   `help:"Refresh cached output for this command target."`
}

func (c *CatCmd) Run(ctx *Context) error {
	client, err := ctx.JiraClientForUpdateCache(c.UpdateCache)
	if err != nil {
		return err
	}

	if isTicketID(c.Target) {
		ticket, err := client.GetTicket(context.Background(), c.Target)
		if err != nil {
			return err
		}
		output := renderTicket(ticket)
		fmt.Print(output)
		return nil
	}

	// Treat target as a sprint name — fetch the sprint goal from Jira.
	projectKey, err := ensureProject(ctx, client)
	if err != nil {
		return err
	}
	boardID, err := ensureBoard(ctx, client, projectKey)
	if err != nil {
		return err
	}
	_, sprints, err := resolveBoardSprints(ctx, client, projectKey, boardID)
	if err != nil {
		return err
	}
	sprint, err := findSprint(sprints, c.Target)
	if err != nil {
		return err
	}
	if sprint != nil {
		var output string
		if strings.TrimSpace(sprint.Goal) == "" {
			output = "(no goal set for this sprint)\n"
		} else {
			output = sprint.Goal
			if !strings.HasSuffix(output, "\n") {
				output += "\n"
			}
		}
		fmt.Print(output)
		return nil
	}
	return fmt.Errorf("sprint %q not found", c.Target)
}

func renderTicket(t jira.IssueTicket) string {
	var b strings.Builder
	fmt.Fprintf(&b, "id:       %s\n", t.ID)
	fmt.Fprintf(&b, "title:    %s\n", t.Title)
	fmt.Fprintf(&b, "state:    %s\n", emptyFallback(t.State))
	fmt.Fprintf(&b, "assignee: %s\n", emptyFallback(t.Assignee))
	fmt.Fprintf(&b, "reporter: %s\n", emptyFallback(t.Reporter))
	if t.Priority != "" {
		fmt.Fprintf(&b, "priority: %s\n", t.Priority)
	}
	if len(t.Labels) > 0 {
		fmt.Fprintf(&b, "labels:   %s\n", strings.Join(t.Labels, ", "))
	}
	if t.URL != "" {
		fmt.Fprintf(&b, "url:      %s\n", t.URL)
	}
	if strings.TrimSpace(t.Description) != "" {
		b.WriteByte('\n')
		b.WriteString(t.Description)
		if !strings.HasSuffix(t.Description, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func renderLsOutput(sprint jira.Sprint, list []jira.IssueTicket, verbose bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d)\n", sprint.Name, sprint.ID)
	for i, ticket := range list {
		fmt.Fprintf(&b, "%s %s\n", ticket.ID, ticket.Title)
		if !verbose {
			continue
		}
		fmt.Fprintf(&b, "  assignee: %s\n", emptyFallback(ticket.Assignee))
		fmt.Fprintf(&b, "  reporter: %s\n", emptyFallback(ticket.Reporter))
		fmt.Fprintf(&b, "  state: %s\n", emptyFallback(ticket.State))
		if ticket.PRLink != "" {
			fmt.Fprintf(&b, "  pr: %s\n", ticket.PRLink)
		}
		if i < len(list)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func localSprintNote(basePath string, sprint jira.Sprint) string {
	dir, err := expectedSprintDir(basePath, sprint)
	if err != nil {
		return ""
	}
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return " ✅ local"
	}
	return ""
}

func expectedSprintDir(basePath string, sprint jira.Sprint) (string, error) {
	baseAbs, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(baseAbs, sprintFolderName(sprint)), nil
}

func renderSprintListOutput(basePath string, sprints []jira.Sprint) string {
	var b strings.Builder
	for _, sprint := range sprints {
		fmt.Fprintf(&b, "%s (%d)", sprint.Name, sprint.ID)
		b.WriteString(localSprintNote(basePath, sprint))
		b.WriteByte('\n')
	}
	return b.String()
}

// isTicketID returns true when s looks like a Jira ticket key (e.g. PROJ-123 or proj-123).

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

// pickAssignableUser shows an interactive, searchable list of users assignable to issueKey.
// The user can type a number to select or a search term to re-filter the list.
func pickAssignableUser(client *jira.Client, issueKey string) (jira.User, error) {
	reader := bufio.NewReader(os.Stdin)
	query := ""
	for {
		users, err := client.SearchAssignableUsers(context.Background(), issueKey, query)
		if err != nil {
			return jira.User{}, fmt.Errorf("could not list assignable users: %w", err)
		}
		if len(users) == 0 {
			if query == "" {
				return jira.User{}, fmt.Errorf("no assignable users found for %s", issueKey)
			}
			fmt.Printf("  (no results for %q)\n", query)
			query = ""
			continue
		}
		if query != "" {
			fmt.Printf("Results for %q:\n", query)
		} else {
			fmt.Printf("Assignable users for %s:\n", issueKey)
		}
		for i, u := range users {
			fmt.Printf("  %d) %s\n", i+1, u.DisplayName)
		}
		fmt.Print("Enter number to select, or type a name to search (empty to cancel): ")
		raw, err := reader.ReadString('\n')
		if err != nil {
			return jira.User{}, err
		}
		input := strings.TrimSpace(raw)
		if input == "" {
			return jira.User{}, errors.New("assignment cancelled")
		}
		if n, err := strconv.Atoi(input); err == nil {
			if n < 1 || n > len(users) {
				fmt.Printf("  (invalid number, valid range is 1–%d)\n", len(users))
				continue
			}
			return users[n-1], nil
		}
		// Treat as a new search query.
		query = input
	}
}

type AssignCmd struct {
	ID   string `arg:"" help:"Ticket ID."`
	User string `arg:"" optional:"" help:"User to assign; omit to pick from a list."`
}

func (c *AssignCmd) Run(ctx *Context) error {
	client, err := ctx.JiraClient()
	if err != nil {
		return err
	}

	var user jira.User
	if c.User == "" {
		if !stdinIsTerminal() {
			return errors.New("no user specified; provide a user argument or run interactively to pick from a list")
		}
		user, err = pickAssignableUser(client, c.ID)
		if err != nil {
			return err
		}
	} else {
		users, err := client.SearchAssignableUsers(context.Background(), c.ID, c.User)
		if err != nil {
			return fmt.Errorf("user search failed: %w", err)
		}
		if len(users) == 0 {
			return fmt.Errorf("no users found matching %q", c.User)
		}
		if len(users) == 1 {
			user = users[0]
		} else {
			// Try to find an exact match before prompting.
			var exact []jira.User
			q := strings.ToLower(c.User)
			for _, u := range users {
				if strings.ToLower(u.Name) == q ||
					strings.ToLower(u.EmailAddr) == q ||
					strings.ToLower(u.DisplayName) == q {
					exact = append(exact, u)
				}
			}
			if len(exact) == 1 {
				user = exact[0]
			} else {
				if !stdinIsTerminal() {
					return fmt.Errorf("multiple users found matching %q; run interactively to select one", c.User)
				}
				fmt.Printf("Multiple users found for %q:\n", c.User)
				for i, u := range users {
					fmt.Printf("  %d) %s (%s)\n", i+1, u.DisplayName, u.EmailAddr)
				}
				reader := bufio.NewReader(os.Stdin)
				fmt.Print("Select user number: ")
				raw, err := reader.ReadString('\n')
				if err != nil {
					return err
				}
				selected, err := strconv.Atoi(strings.TrimSpace(raw))
				if err != nil || selected < 1 || selected > len(users) {
					return errors.New("invalid user selection")
				}
				user = users[selected-1]
			}
		}
	}

	if err := client.AssignTicket(context.Background(), c.ID, &user); err != nil {
		return err
	}
	fmt.Printf("Assigned %s to %s\n", c.ID, user.DisplayName)
	return nil
}

type UnassignCmd struct {
	ID string `arg:"" help:"Ticket ID."`
}

func (c *UnassignCmd) Run(ctx *Context) error {
	client, err := ctx.JiraClient()
	if err != nil {
		return err
	}
	if err := client.UnassignTicket(context.Background(), c.ID); err != nil {
		return err
	}
	fmt.Printf("%s unassigned\n", c.ID)
	return nil
}

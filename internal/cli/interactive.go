package cli

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sbenjam1n/gamsync/internal/region"
	"github.com/spf13/cobra"
)

var interactiveCmd = &cobra.Command{
	Use:     "interactive",
	Aliases: []string{"i"},
	Short:   "Interactive TUI for browsing regions, concepts, and syncs",
	RunE: func(cmd *cobra.Command, args []string) error {
		root := projectRoot()

		gamignore := region.ParseGamignore(root)
		markers, warnings, err := region.ScanDirectory(root, gamignore)
		if err != nil {
			return fmt.Errorf("scan directory: %w", err)
		}

		archEntries, _ := region.ParseArchMdEntries(root)

		// Build tree
		tree := region.BuildTree(markers)

		// Try to load DB data (optional — TUI works without DB)
		var regions []dbRegionInfo
		var concepts []dbConceptInfo
		var syncs []dbSyncInfo
		ctx := context.Background()
		pool, poolErr := connectDB(ctx)
		if poolErr == nil {
			defer pool.Close()
			regions = loadDBRegions(ctx, pool)
			concepts = loadDBConcepts(ctx, pool)
			syncs = loadDBSyncs(ctx, pool)
		}

		m := newInteractiveModel(tree, archEntries, markers, warnings, regions, concepts, syncs)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return err
		}
		return nil
	},
}

// --- Data types ---

type dbRegionInfo struct {
	Path     string
	State    string
	Concepts string
}

type dbConceptInfo struct {
	Name    string
	Purpose string
}

type dbSyncInfo struct {
	Name        string
	Description string
	Enabled     bool
}

// --- Styles ---

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("15"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	pathStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
)

// --- View modes ---

type viewMode int

const (
	viewRegions  viewMode = iota
	viewConcepts
	viewSyncs
	viewDetail
)

// --- Tree item ---

type treeItem struct {
	name      string
	fullPath  string
	file      string
	startLine int
	endLine   int
	depth     int
	expanded  bool
	children  []*treeItem
	comment   string // from arch.md
}

// --- Model ---

type interactiveModel struct {
	tree         *region.TreeNode
	archEntries  []region.ArchEntry
	markers      []*region.RegionMarker
	warnings     []string
	dbRegions    []dbRegionInfo
	dbConcepts   []dbConceptInfo
	dbSyncs      []dbSyncInfo
	items        []*treeItem
	cursor       int
	viewMode     viewMode
	width        int
	height       int
	searchMode   bool
	searchBuffer string
	detailPath   string
}

func newInteractiveModel(
	tree *region.TreeNode,
	archEntries []region.ArchEntry,
	markers []*region.RegionMarker,
	warnings []string,
	dbRegions []dbRegionInfo,
	dbConcepts []dbConceptInfo,
	dbSyncs []dbSyncInfo,
) interactiveModel {
	archComments := make(map[string]string)
	for _, e := range archEntries {
		archComments[e.Path] = e.Comment
	}

	items := buildTreeItems(tree, archComments, 0)
	// Expand top-level by default
	for _, item := range items {
		item.expanded = true
	}

	return interactiveModel{
		tree:        tree,
		archEntries: archEntries,
		markers:     markers,
		warnings:    warnings,
		dbRegions:   dbRegions,
		dbConcepts:  dbConcepts,
		dbSyncs:     dbSyncs,
		items:       items,
		width:       80,
		height:      24,
	}
}

func buildTreeItems(node *region.TreeNode, comments map[string]string, depth int) []*treeItem {
	var items []*treeItem
	for _, child := range node.Children {
		item := &treeItem{
			name:      child.Name,
			fullPath:  child.FullPath,
			file:      child.File,
			startLine: child.Start,
			endLine:   child.End,
			depth:     depth,
			comment:   comments[child.FullPath],
			children:  buildTreeItems(child, comments, depth+1),
		}
		items = append(items, item)
	}
	return items
}

func (m interactiveModel) Init() tea.Cmd {
	return nil
}

func (m interactiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.searchMode {
			return m.handleSearchKey(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "j", "down":
			visible := m.visibleItems()
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "g":
			m.cursor = 0
		case "G":
			visible := m.visibleItems()
			m.cursor = len(visible) - 1

		case "enter", " ", "l", "right":
			visible := m.visibleItems()
			if m.cursor < len(visible) {
				item := visible[m.cursor]
				if len(item.children) > 0 {
					item.expanded = !item.expanded
				} else {
					// Show detail for leaf nodes
					m.viewMode = viewDetail
					m.detailPath = item.fullPath
				}
			}
		case "h", "left":
			visible := m.visibleItems()
			if m.cursor < len(visible) {
				item := visible[m.cursor]
				if item.expanded {
					item.expanded = false
				}
			}

		case "1":
			m.viewMode = viewRegions
			m.cursor = 0
		case "2":
			m.viewMode = viewConcepts
			m.cursor = 0
		case "3":
			m.viewMode = viewSyncs
			m.cursor = 0

		case "/":
			m.searchMode = true
			m.searchBuffer = ""

		case "esc":
			if m.viewMode == viewDetail {
				m.viewMode = viewRegions
			}
		}
	}
	return m, nil
}

func (m *interactiveModel) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.searchMode = false
		// Find first matching item
		if m.searchBuffer != "" {
			visible := m.visibleItems()
			for i, item := range visible {
				if strings.Contains(strings.ToLower(item.fullPath), strings.ToLower(m.searchBuffer)) {
					m.cursor = i
					break
				}
			}
		}
	case "esc":
		m.searchMode = false
		m.searchBuffer = ""
	case "backspace":
		if len(m.searchBuffer) > 0 {
			m.searchBuffer = m.searchBuffer[:len(m.searchBuffer)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.searchBuffer += msg.String()
		}
	}
	return m, nil
}

func (m interactiveModel) visibleItems() []*treeItem {
	var result []*treeItem
	var collect func(items []*treeItem)
	collect = func(items []*treeItem) {
		for _, item := range items {
			result = append(result, item)
			if item.expanded && len(item.children) > 0 {
				collect(item.children)
			}
		}
	}
	collect(m.items)
	return result
}

func (m interactiveModel) View() string {
	var b strings.Builder

	// Header
	header := titleStyle.Render("GAM+Sync Interactive Browser")
	tabs := ""
	switch m.viewMode {
	case viewRegions:
		tabs = headerStyle.Render("[1:Regions]") + " " + dimStyle.Render("2:Concepts") + " " + dimStyle.Render("3:Syncs")
	case viewConcepts:
		tabs = dimStyle.Render("1:Regions") + " " + headerStyle.Render("[2:Concepts]") + " " + dimStyle.Render("3:Syncs")
	case viewSyncs:
		tabs = dimStyle.Render("1:Regions") + " " + dimStyle.Render("2:Concepts") + " " + headerStyle.Render("[3:Syncs]")
	case viewDetail:
		tabs = dimStyle.Render("1:Regions") + " " + dimStyle.Render("2:Concepts") + " " + dimStyle.Render("3:Syncs") + " " + headerStyle.Render("[Detail]")
	}

	b.WriteString(header + "  " + tabs + "\n")
	b.WriteString(strings.Repeat("─", min(m.width, 80)) + "\n")

	contentHeight := m.height - 5 // header + tabs + divider + help + search

	switch m.viewMode {
	case viewRegions:
		m.renderRegionTree(&b, contentHeight)
	case viewConcepts:
		m.renderConceptList(&b, contentHeight)
	case viewSyncs:
		m.renderSyncList(&b, contentHeight)
	case viewDetail:
		m.renderDetail(&b, contentHeight)
	}

	// Search bar
	if m.searchMode {
		b.WriteString("\n/" + m.searchBuffer + "█")
	}

	// Help
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("j/k:navigate  enter:expand/detail  1/2/3:tabs  /:search  q:quit"))

	return b.String()
}

func (m interactiveModel) renderRegionTree(b *strings.Builder, maxLines int) {
	visible := m.visibleItems()

	// Scroll offset
	scrollOffset := 0
	if m.cursor >= maxLines {
		scrollOffset = m.cursor - maxLines + 1
	}

	lines := 0
	for i := scrollOffset; i < len(visible) && lines < maxLines; i++ {
		item := visible[i]
		indent := strings.Repeat("  ", item.depth)
		prefix := "  "
		if len(item.children) > 0 {
			if item.expanded {
				prefix = "▼ "
			} else {
				prefix = "▶ "
			}
		}

		line := indent + prefix + item.name
		if item.comment != "" {
			line += "  " + dimStyle.Render("# "+item.comment)
		}
		if item.file != "" {
			line += "  " + dimStyle.Render(fmt.Sprintf("[%s:%d]", item.file, item.startLine))
		}

		if i == m.cursor {
			b.WriteString(selectedStyle.Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
		lines++
	}

	// Warnings
	if len(m.warnings) > 0 && lines < maxLines {
		b.WriteString("\n")
		lines++
		for _, w := range m.warnings {
			if lines >= maxLines {
				break
			}
			b.WriteString(warnStyle.Render("⚠ "+w) + "\n")
			lines++
		}
	}
}

func (m interactiveModel) renderConceptList(b *strings.Builder, maxLines int) {
	if len(m.dbConcepts) == 0 {
		b.WriteString(dimStyle.Render("  No concepts found (database not connected or empty)") + "\n")
		return
	}

	lines := 0
	for i, c := range m.dbConcepts {
		if lines >= maxLines {
			break
		}
		line := fmt.Sprintf("  %-25s %s", c.Name, dimStyle.Render(c.Purpose))
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
		lines++
	}
}

func (m interactiveModel) renderSyncList(b *strings.Builder, maxLines int) {
	if len(m.dbSyncs) == 0 {
		b.WriteString(dimStyle.Render("  No syncs found (database not connected or empty)") + "\n")
		return
	}

	lines := 0
	for i, s := range m.dbSyncs {
		if lines >= maxLines {
			break
		}
		enabled := headerStyle.Render("●")
		if !s.Enabled {
			enabled = dimStyle.Render("○")
		}
		line := fmt.Sprintf("  %s %-25s %s", enabled, s.Name, dimStyle.Render(s.Description))
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
		lines++
	}
}

func (m interactiveModel) renderDetail(b *strings.Builder, maxLines int) {
	b.WriteString(pathStyle.Render("Region: "+m.detailPath) + "\n")
	b.WriteString(strings.Repeat("─", 40) + "\n")
	lines := 2

	// Find in arch entries
	for _, e := range m.archEntries {
		if e.Path == m.detailPath && lines < maxLines {
			if e.Comment != "" {
				b.WriteString("  Comment: " + e.Comment + "\n")
				lines++
			}
			b.WriteString(fmt.Sprintf("  arch.md line: %d\n", e.Line))
			lines++
		}
	}

	// Find in markers
	for _, mk := range m.markers {
		if mk.Path == m.detailPath && lines < maxLines {
			b.WriteString(fmt.Sprintf("  Source: %s:%d-%d\n", mk.File, mk.StartLine, mk.EndLine))
			lines++
		}
	}

	// Find in DB regions
	for _, r := range m.dbRegions {
		if r.Path == m.detailPath && lines < maxLines {
			b.WriteString(fmt.Sprintf("  State: %s\n", r.State))
			lines++
			if r.Concepts != "" {
				b.WriteString(fmt.Sprintf("  Concepts: %s\n", r.Concepts))
				lines++
			}
		}
	}

	if lines < maxLines {
		b.WriteString("\n" + helpStyle.Render("Press ESC to return") + "\n")
	}
}

// --- DB loading helpers (use *pgxpool.Pool) ---

func loadDBRegions(ctx context.Context, pool *pgxpool.Pool) []dbRegionInfo {
	rows, err := pool.Query(ctx, `
		SELECT r.path, r.lifecycle_state,
		       COALESCE(string_agg(c.name, ', '), '') as concepts
		FROM regions r
		LEFT JOIN concept_region_assignments cra ON cra.region_id = r.id
		LEFT JOIN concepts c ON c.id = cra.concept_id
		GROUP BY r.id, r.path, r.lifecycle_state
		ORDER BY r.path
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []dbRegionInfo
	for rows.Next() {
		var r dbRegionInfo
		rows.Scan(&r.Path, &r.State, &r.Concepts)
		result = append(result, r)
	}
	return result
}

func loadDBConcepts(ctx context.Context, pool *pgxpool.Pool) []dbConceptInfo {
	rows, err := pool.Query(ctx, `SELECT name, purpose FROM concepts ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []dbConceptInfo
	for rows.Next() {
		var c dbConceptInfo
		rows.Scan(&c.Name, &c.Purpose)
		result = append(result, c)
	}
	return result
}

func loadDBSyncs(ctx context.Context, pool *pgxpool.Pool) []dbSyncInfo {
	rows, err := pool.Query(ctx, `SELECT name, description, enabled FROM synchronizations ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []dbSyncInfo
	for rows.Next() {
		var s dbSyncInfo
		rows.Scan(&s.Name, &s.Description, &s.Enabled)
		result = append(result, s)
	}
	return result
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}

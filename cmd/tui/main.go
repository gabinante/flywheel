// TUI client for Flywheel. Uses the generated REST client.
// Optional: FLYWHEEL_BASE_URL (default http://localhost:8080).
// On start: log in with GitHub (browser), then select an org. No JWT or org ID required in env.
// Token is cached in ~/.config/flywheel/token (or platform config dir) with 0600 permissions.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"
	"github.com/gabinante/flywheel/api/client"
	"github.com/gabinante/flywheel/cmd/tui/components"
)

const (
	baseURLDefault = "http://localhost:8080"
)

// tokenCachePath returns the path to the token cache file.
func tokenCachePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "flywheel", "token"), nil
}

type tokenCache struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

func readTokenCache(baseURL string) (string, error) {
	path, err := tokenCachePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var c tokenCache
	if err := json.Unmarshal(data, &c); err != nil {
		return "", nil
	}
	if c.BaseURL != baseURL || c.Token == "" {
		return "", nil
	}
	return c.Token, nil
}

func writeTokenCache(baseURL, token string) error {
	path, err := tokenCachePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	c := tokenCache{BaseURL: baseURL, Token: token}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func clearTokenCache() error {
	path, err := tokenCachePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func achievementsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "flywheel", "achievements.json"), nil
}

type achievementsStore struct {
	Unlocked []string `json:"unlocked"`
}

func loadAchievements() ([]string, error) {
	path, err := achievementsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s achievementsStore
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s.Unlocked, nil
}

func saveAchievements(unlocked []string) error {
	path, err := achievementsPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	s := achievementsStore{Unlocked: unlocked}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// achievementDef defines an achievement and how to unlock it.
type achievementDef struct {
	ID   string
	Name string
	Check func(stats *client.MeStats, history *client.MeStatsHistory) bool
}

var achievementDefs = []achievementDef{
	{ID: "first_ticket", Name: "First ticket", Check: func(s *client.MeStats, h *client.MeStatsHistory) bool {
		return s != nil && s.TicketsCreated >= 1
	}},
	{ID: "10_reviews", Name: "10 reviews", Check: func(s *client.MeStats, h *client.MeStatsHistory) bool {
		return s != nil && (s.ReviewsApproved+s.ReviewsRejected) >= 10
	}},
	{ID: "week_streak", Name: "Week streak", Check: func(s *client.MeStats, h *client.MeStatsHistory) bool {
		return computeStreak(h) >= 7
	}},
	{ID: "50_tickets", Name: "50 tickets", Check: func(s *client.MeStats, h *client.MeStatsHistory) bool {
		return s != nil && s.TicketsCreated >= 50
	}},
}

func (m *model) updateAchievements() {
	if m.meStats == nil && m.meStatsHistory == nil {
		return
	}
	current := m.achievementsUnlocked
	if current == nil {
		var err error
		current, err = loadAchievements()
		if err != nil {
			return
		}
	}
	unlocked, changed := checkAndUnlockAchievements(m.meStats, m.meStatsHistory, current)
	m.achievementsUnlocked = unlocked
	if changed {
		_ = saveAchievements(unlocked)
	}
}

func checkAndUnlockAchievements(stats *client.MeStats, history *client.MeStatsHistory, current []string) ([]string, bool) {
	unlocked := make(map[string]bool)
	for _, id := range current {
		unlocked[id] = true
	}
	changed := false
	for _, def := range achievementDefs {
		if unlocked[def.ID] {
			continue
		}
		if def.Check(stats, history) {
			unlocked[def.ID] = true
			changed = true
		}
	}
	if !changed {
		return current, false
	}
	out := make([]string, 0, len(unlocked))
	for _, def := range achievementDefs {
		if unlocked[def.ID] {
			out = append(out, def.ID)
		}
	}
	return out, true
}

func renderHeader(m model) string {
	s := components.Primary.Render("Flywheel")
	if m.orgID != "" {
		name := ""
		for _, o := range m.orgs {
			if o.Id != nil && *o.Id == m.orgID {
				if o.Name != nil {
					name = *o.Name
				} else if o.Slug != nil {
					name = *o.Slug
				}
				break
			}
		}
		if name == "" {
			name = m.orgID
		}
		s += components.Muted.Render(" · "+name)
	}
	if m.projectID != "" {
		slug := m.projectID
		for _, p := range m.projects {
			if p.Id != nil && *p.Id == m.projectID {
				if p.Slug != nil {
					slug = *p.Slug
				}
				break
			}
		}
		s += components.Muted.Render(" · "+slug)
	}
	if len(m.achievementsUnlocked) > 0 {
		s += components.Muted.Render(fmt.Sprintf(" · 🏆 %d", len(m.achievementsUnlocked)))
	}
	if m.celebrationMsg != "" {
		s += components.Success.Render(m.celebrationMsg) + "\n"
	}
	s += "\n"
	// Lifetime stats: one bordered block, three columns (label + number + small bar), aligned to content width
	if m.meStats != nil {
		w := contentWidth(m)
		// Simple milestone bar: 10 chars, no ANSI so width is predictable
		bar := func(pct float64) string {
			if pct > 1 {
				pct = 1
			}
			n := int(pct * 10)
			if n > 10 {
				n = 10
			}
			return strings.Repeat("█", n) + strings.Repeat("░", 10-n)
		}
		nextMilestone := func(n int) int {
			if n <= 0 {
				return 10
			}
			return ((n / 10) + 1) * 10
		}
		milestoneT := nextMilestone(m.meStats.TicketsCreated)
		milestoneA := nextMilestone(m.meStats.ReviewsApproved)
		milestoneR := nextMilestone(m.meStats.ReviewsRejected)
		if milestoneR < 10 {
			milestoneR = 10
		}
		pctT := float64(m.meStats.TicketsCreated) / float64(milestoneT)
		pctA := float64(m.meStats.ReviewsApproved) / float64(milestoneA)
		pctR := float64(m.meStats.ReviewsRejected) / float64(milestoneR)
		// Badge for milestones: First ticket (1), 10 tickets, 50 tickets, etc.
		badge := func(n int, labels map[int]string) string {
			for _, m := range []int{500, 250, 100, 50, 25, 10, 1} {
				if n >= m && labels[m] != "" {
					return " " + components.Muted.Render("★ "+labels[m])
				}
			}
			return ""
		}
		ticketBadges := map[int]string{1: "First ticket", 10: "10 tickets", 50: "50 tickets", 100: "100 tickets"}
		reviewBadges := map[int]string{1: "First review", 10: "10 reviews", 50: "50 reviews", 100: "100 reviews"}
		// Bar = progress toward next round-number milestone; label so it's clear
		col1 := components.Muted.Render("Tickets created") + "\n" + components.Primary.Render(fmt.Sprintf("%d", m.meStats.TicketsCreated)) + badge(m.meStats.TicketsCreated, ticketBadges) + "\n" + components.Muted.Render(bar(pctT)+" "+fmt.Sprintf("%d/%d", m.meStats.TicketsCreated, milestoneT))
		col2 := components.Muted.Render("Reviews approved") + "\n" + components.Primary.Render(fmt.Sprintf("%d", m.meStats.ReviewsApproved)) + badge(m.meStats.ReviewsApproved, reviewBadges) + "\n" + components.Muted.Render(bar(pctA)+" "+fmt.Sprintf("%d/%d", m.meStats.ReviewsApproved, milestoneA))
		col3 := components.Muted.Render("Reviews rejected") + "\n" + components.Primary.Render(fmt.Sprintf("%d", m.meStats.ReviewsRejected)) + badge(m.meStats.ReviewsRejected, reviewBadges) + "\n" + components.Muted.Render(bar(pctR)+" "+fmt.Sprintf("%d/%d", m.meStats.ReviewsRejected, milestoneR))
		statsRow := lipgloss.JoinHorizontal(lipgloss.Top, col1, "    ", col2, "    ", col3)
		statsInner := components.Primary.Render("Your impact") + "\n\n" + statsRow
		if m.meStatsHistory != nil {
			if streak := computeStreak(m.meStatsHistory); streak > 0 {
				statsInner += "\n\n" + components.Muted.Render(fmt.Sprintf("🔥 %d-day streak", streak))
			}
		}
		if m.meStatsHistory != nil && len(m.meStatsHistory.TicketsCreated) > 0 {
			tickets := intsToFloats(m.meStatsHistory.TicketsCreated)
			n := len(m.meStatsHistory.TicketsCreated)
			reviews := make([]float64, n)
			approved := m.meStatsHistory.ReviewsApproved
			rejected := m.meStatsHistory.ReviewsRejected
			for i := 0; i < n; i++ {
				a, r := 0, 0
				if i < len(approved) {
					a = approved[i]
				}
				if i < len(rejected) {
					r = rejected[i]
				}
				reviews[i] = float64(a + r)
			}
			graphWidth := 50
			if w > 0 && w < 80 {
				graphWidth = w - 4
			}
			if graphWidth > 20 {
				plot := asciigraph.PlotMany([][]float64{tickets, reviews},
					asciigraph.Height(5),
					asciigraph.Width(graphWidth),
					asciigraph.Precision(0),
					asciigraph.SeriesLegends("tickets", "reviews"),
					asciigraph.Caption("Activity (last 14 days)"))
				statsInner += "\n\n" + components.Muted.Render(plot)
			}
		}
		// Same border and padding as content panel so stats and content align
		s += components.Border.Width(w).Padding(1, 2).Render(statsInner) + "\n\n"
	}
	if m.projectID != "" {
		if m.screen == screenTickets && len(m.tickets) > 0 {
			s += formatTicketStatsLine(m.tickets) + "\n"
		} else if m.screen == screenPendingReviews {
			s += components.Muted.Render(fmt.Sprintf("Pending reviews: %d", len(m.reviews))) + "\n"
		}
	}
	return s
}

// contentWidth returns width to use for content panel and cards. Uses full terminal width when set; default 120 when unknown; max 200.
func contentWidth(m model) int {
	w := m.width
	if w <= 0 {
		return 120
	}
	if w > 200 {
		return 200
	}
	return w
}

func contentHeight(m model) int {
	h := m.height - 12
	if h < 8 {
		return 8
	}
	return h
}

func renderContentPanel(content string, width int) string {
	if width <= 0 {
		width = 120
	}
	if width > 200 {
		width = 200
	}
	return components.Border.Width(width).Padding(1, 2).Render(content)
}

func renderHelpBar(hints []string) string {
	return components.KeyHintBar(hints) + "\n"
}

func ticketCountsByState(tickets []client.Ticket) map[string]int {
	counts := make(map[string]int)
	for _, t := range tickets {
		if t.State != nil {
			counts[string(*t.State)]++
		}
	}
	return counts
}

func intsToFloats(a []int) []float64 {
	out := make([]float64, len(a))
	for i, v := range a {
		out[i] = float64(v)
	}
	return out
}

// computeStreak returns consecutive days with activity (tickets + reviews), from most recent backward.
func computeStreak(h *client.MeStatsHistory) int {
	if h == nil || len(h.TicketsCreated) == 0 {
		return 0
	}
	n := len(h.TicketsCreated)
	approved := h.ReviewsApproved
	rejected := h.ReviewsRejected
	streak := 0
	for i := n - 1; i >= 0; i-- {
		activity := h.TicketsCreated[i]
		if i < len(approved) {
			activity += approved[i]
		}
		if i < len(rejected) {
			activity += rejected[i]
		}
		if activity > 0 {
			streak++
		} else {
			break
		}
	}
	return streak
}

func formatTicketStatsLine(tickets []client.Ticket) string {
	counts := ticketCountsByState(tickets)
	order := []string{"pending", "claimed", "executing", "awaiting_review", "done", "blocked", "needs_human", "failed"}
	var parts []string
	for _, s := range order {
		if c := counts[s]; c > 0 {
			parts = append(parts, components.StateStyle(s).Render(fmt.Sprintf("%s: %d", s, c)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "  ")
}

func main() {
	baseURL := os.Getenv("FLYWHEEL_BASE_URL")
	if baseURL == "" {
		baseURL = baseURLDefault
	}
	token, _ := readTokenCache(baseURL)
	m := newModel(baseURL, token)
	if token != "" {
		m.screen = screenOrgSelect
		m.loading = true
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}
}

type screen int

const (
	screenLogin screen = iota
	screenOrgSelect
	screenProjects
	screenProjectMenu
	screenTickets
	screenTicketDetail
	screenPendingReviews
	screenReviewDecision
	screenGitNotesLog
	screenGitNoteDetail
	screenProjectSettings
	screenCreateProject
	screenProjectFilter
	screenTicketFilter
	screenWorkStreams
)

type model struct {
	baseURL string
	token   string
	api     *client.ClientWithResponses

	orgID    string
	orgs     []client.Org
	projects []client.Project
	tickets     []client.Ticket
	reviews      []client.Ticket
	reviewTicket *client.Ticket        // ticket currently being reviewed
	reviewTrace  *client.ExecutionTrace // execution trace (loaded when entering review screen)
	detailTicket *client.Ticket        // ticket shown in detail view
	detailTrace  *client.ExecutionTrace // trace for detail view
	projectStatus string               // active, closed, all for project list filter
	meStats           *client.MeStats       // lifetime stats (loaded on org select screen)
	meStatsHistory    *client.MeStatsHistory // daily counts for activity graph
	achievementsUnlocked []string             // achievement IDs from ~/.config/flywheel/achievements.json

	screen    screen
	selected  int
	projectID string
	ticketID  string
	err          string
	loading      bool
	confirmReject bool
	successMsg    string // e.g. "✓ Approved" after review
	celebrationMsg string // e.g. "🎉 50 reviews approved!" when crossing milestone
	width        int
	height    int

	// Git notes
	gitNotesLog     []client.GitNotesLogEntry
	gitNotesErr     string
	gitNoteDetail   *gitNoteDetail
	noteTypeFilter  string // "decision", "trace", "intent"

	// Project settings (work streams + git integration)
	settingsGitRemotes    []string // "Off" + remote names from git remote
	settingsGitRemotesErr string
	settingsRepoPath      string // resolved repo path for git commands

	// Create project
	createProjectNameInput textinput.Model

	// Project settings (name, slug, git remote)
	settingsNameInput       textinput.Model
	settingsSlugInput       textinput.Model
	settingsDefaultBranchInput textinput.Model
	settingsFocus           int // 0=name, 1=slug, 2=default branch, 3=remote list

	// Ticket list filter (work stream status + work stream + state)
	workStreams                   []client.WorkStream
	workStreamsErr                 string
	ticketWorkStreamStatusFilter   string // "active", "closed", "all" - which work streams to show
	ticketWorkStreamFilter         string // when set, filter tickets by this work stream ID
	ticketStateFilter              string // "active" (default), "all", or specific state
	ticketFilterFocus              int    // 0 = work stream status, 1 = work stream, 2 = state
	ticketFilterStatusSelected    int    // 0=active, 1=closed, 2=all
	ticketFilterWorkStreamSelected int
	ticketFilterStateSelected      int

	// Work streams management screen
	workStreamsList     []client.WorkStream
	workStreamStatusFilter string // "active", "closed", "all"

	spinner    spinner.Model
	ticketList list.Model
	detailVP   viewport.Model
}

// ticketItem implements list.DefaultItem for the tickets list.
type ticketItem struct {
	title string
	desc  string
}

func (t ticketItem) Title() string       { return t.title }
func (t ticketItem) Description() string { return t.desc }
func (t ticketItem) FilterValue() string { return t.title }

type gitNoteDetail struct {
	CommitSHA string
	Type      string
	Message   string
	AgentID   string
	TicketID  string
	CreatedAt string
	Body      string
}

func newModel(baseURL, token string) model {
	m := model{baseURL: baseURL, token: token}
	if token != "" {
		m.api, _ = client.NewClientWithResponses(baseURL, m.authEditor())
	}
	m.spinner = spinner.New(spinner.WithStyle(components.Primary))
	return m
}

// spinnerTickCmd returns a Cmd that sends a spinner tick (Tick returns Msg, we need Cmd).
func (m model) spinnerTickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond, func(t time.Time) tea.Msg {
		return m.spinnerTickCmd()
	})
}

func (m *model) authEditor() client.ClientOption {
	token := m.token
	return client.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	})
}

func (m model) Init() tea.Cmd {
	if m.token == "" {
		return nil
	}
	if m.orgID == "" {
		return tea.Batch(loadOrgs(m.api), loadStats(m.api), loadStatsHistory(m.api, 14), m.spinnerTickCmd())
	}
	return tea.Batch(loadProjects(m.api, m.orgID, m.projectStatus), m.spinnerTickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.screen == screenTickets && len(m.tickets) > 0 {
			w := contentWidth(m)
			if w <= 0 {
				w = 120
			}
			h := m.height - 12
			if h < 4 {
				h = 4
			}
			m.ticketList.SetSize(w, h)
		}
		if (m.screen == screenTicketDetail || m.screen == screenReviewDecision) && m.detailVP.Height > 0 {
			w, h := contentWidth(m), contentHeight(m)
			m.detailVP.Width = w
			m.detailVP.Height = h
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loading {
			return m, cmd
		}
		return m, nil
	case tea.KeyMsg:
		if m.screen == screenCreateProject {
			switch msg.String() {
			case "enter":
				name := strings.TrimSpace(m.createProjectNameInput.Value())
				if name == "" {
					name = "My Project"
				}
				m.loading = true
				return m, tea.Batch(createProjectWithName(m.api, m.orgID, name), m.spinnerTickCmd())
			case "esc":
				return m.handleBack()
			default:
				var cmd tea.Cmd
				m.createProjectNameInput, cmd = m.createProjectNameInput.Update(msg)
				return m, cmd
			}
		}
		if m.screen == screenProjectSettings {
			switch msg.String() {
			case "tab":
				m.settingsFocus = (m.settingsFocus + 1) % 4
				if m.settingsFocus == 0 {
					m.settingsNameInput.Focus()
					m.settingsSlugInput.Blur()
					m.settingsDefaultBranchInput.Blur()
				} else if m.settingsFocus == 1 {
					m.settingsSlugInput.Focus()
					m.settingsNameInput.Blur()
					m.settingsDefaultBranchInput.Blur()
				} else if m.settingsFocus == 2 {
					m.settingsDefaultBranchInput.Focus()
					m.settingsNameInput.Blur()
					m.settingsSlugInput.Blur()
				} else {
					m.settingsNameInput.Blur()
					m.settingsSlugInput.Blur()
					m.settingsDefaultBranchInput.Blur()
				}
				return m, textinput.Blink
			case "shift+tab":
				m.settingsFocus = (m.settingsFocus + 3) % 4
				if m.settingsFocus == 0 {
					m.settingsNameInput.Focus()
					m.settingsSlugInput.Blur()
					m.settingsDefaultBranchInput.Blur()
				} else if m.settingsFocus == 1 {
					m.settingsSlugInput.Focus()
					m.settingsNameInput.Blur()
					m.settingsDefaultBranchInput.Blur()
				} else if m.settingsFocus == 2 {
					m.settingsDefaultBranchInput.Focus()
					m.settingsNameInput.Blur()
					m.settingsSlugInput.Blur()
				} else {
					m.settingsNameInput.Blur()
					m.settingsSlugInput.Blur()
					m.settingsDefaultBranchInput.Blur()
				}
				return m, textinput.Blink
			case "enter":
				m.loading = true
				return m, tea.Batch(saveProjectSettings(m.api, m.projectID, m.settingsRepoPath, m.settingsGitRemotes, m.selected, strings.TrimSpace(m.settingsNameInput.Value()), strings.TrimSpace(m.settingsSlugInput.Value()), strings.TrimSpace(m.settingsDefaultBranchInput.Value())), m.spinnerTickCmd())
			case "up", "k":
				if m.settingsFocus == 3 {
					if m.selected > 0 {
						m.selected--
					}
					return m, nil
				}
			case "down", "j":
				if m.settingsFocus == 3 {
					if m.selected < len(m.settingsGitRemotes)-1 {
						m.selected++
					}
					return m, nil
				}
			case "esc":
				return m.handleBack()
			default:
				if m.settingsFocus == 0 {
					var cmd tea.Cmd
					m.settingsNameInput, cmd = m.settingsNameInput.Update(msg)
					return m, cmd
				}
				if m.settingsFocus == 1 {
					var cmd tea.Cmd
					m.settingsSlugInput, cmd = m.settingsSlugInput.Update(msg)
					return m, cmd
				}
				if m.settingsFocus == 2 {
					var cmd tea.Cmd
					m.settingsDefaultBranchInput, cmd = m.settingsDefaultBranchInput.Update(msg)
					return m, cmd
				}
			}
		}
		if m.celebrationMsg != "" {
			m.celebrationMsg = ""
		}
		// Delegate viewport keys when on ticket detail or review (not when confirming reject)
		if (m.screen == screenTicketDetail || (m.screen == screenReviewDecision && !m.confirmReject)) && m.detailVP.Height > 0 {
			switch msg.String() {
			case "up", "k", "down", "j", "pgup", "pgdown":
				var cmd tea.Cmd
				m.detailVP, cmd = m.detailVP.Update(msg)
				return m, cmd
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.screen == screenTicketFilter {
				switch m.ticketFilterFocus {
				case 0:
					if m.ticketFilterStatusSelected > 0 {
						m.ticketFilterStatusSelected--
					}
				case 1:
					if m.ticketFilterWorkStreamSelected > 0 {
						m.ticketFilterWorkStreamSelected--
					}
				case 2:
					if m.ticketFilterStateSelected > 0 {
						m.ticketFilterStateSelected--
					}
				}
				return m, nil
			}
			if m.screen == screenTickets && len(m.tickets) > 0 {
				var cmd tea.Cmd
				m.ticketList, cmd = m.ticketList.Update(msg)
				m.selected = m.ticketList.Index()
				return m, cmd
			}
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.screen == screenTicketFilter {
				switch m.ticketFilterFocus {
				case 0:
					if m.ticketFilterStatusSelected < 2 {
						m.ticketFilterStatusSelected++
					}
				case 1:
					max := 1 + len(m.workStreams)
					if m.ticketFilterWorkStreamSelected < max-1 {
						m.ticketFilterWorkStreamSelected++
					}
				case 2:
					if m.ticketFilterStateSelected < 9 {
						m.ticketFilterStateSelected++
					}
				}
				return m, nil
			}
			if m.screen == screenTickets && len(m.tickets) > 0 {
				var cmd tea.Cmd
				m.ticketList, cmd = m.ticketList.Update(msg)
				m.selected = m.ticketList.Index()
				return m, cmd
			}
			max := m.listLen()
			if m.selected < max-1 {
				m.selected++
			}
			return m, nil
		case "tab":
			if m.screen == screenTicketFilter {
				m.ticketFilterFocus = (m.ticketFilterFocus + 1) % 3
				return m, nil
			}
		case "enter":
			if m.screen == screenTicketFilter {
				statusOptions := []string{"active", "closed", "all"}
				stateOptions := []string{"active", "all", "pending", "awaiting_review", "done", "blocked", "needs_human", "claimed", "executing", "failed"}
				switch m.ticketFilterFocus {
				case 0:
					// Work stream status: set filter, reload work streams
					if m.ticketFilterStatusSelected >= 0 && m.ticketFilterStatusSelected < len(statusOptions) {
						m.ticketWorkStreamStatusFilter = statusOptions[m.ticketFilterStatusSelected]
						m.ticketFilterWorkStreamSelected = 0
						m.ticketWorkStreamFilter = ""
						return m, loadWorkStreams(m.api, m.projectID, m.ticketWorkStreamStatusFilter)
					}
					return m, nil
				case 1:
					// Work stream: set filter, stay in modal
					if m.ticketFilterWorkStreamSelected == 0 {
						m.ticketWorkStreamFilter = ""
					} else if m.ticketFilterWorkStreamSelected > 0 && m.ticketFilterWorkStreamSelected <= len(m.workStreams) && m.workStreams[m.ticketFilterWorkStreamSelected-1].Id != nil {
						m.ticketWorkStreamFilter = *m.workStreams[m.ticketFilterWorkStreamSelected-1].Id
					}
					return m, nil
				case 2:
					// State: set filter, close, reload
					if m.ticketFilterStateSelected >= 0 && m.ticketFilterStateSelected < len(stateOptions) {
						m.ticketStateFilter = stateOptions[m.ticketFilterStateSelected]
						m.screen = screenTickets
						m.selected = 0
						m.loading = true
						return m, tea.Batch(loadTickets(m.api, m.projectID, m.ticketWorkStreamFilter, m.ticketStateFilter), m.spinnerTickCmd())
					}
					return m, nil
				}
				return m, nil
			}
			if m.screen == screenProjectFilter {
				statuses := []string{"active", "closed", "all"}
				if m.selected >= 0 && m.selected < len(statuses) {
					m.projectStatus = statuses[m.selected]
					m.screen = screenProjects
					m.selected = 0
					m.loading = true
					return m, tea.Batch(loadProjects(m.api, m.orgID, m.projectStatus), m.spinnerTickCmd())
				}
				return m, nil
			}
			return m.handleEnter()
		case "f":
			if m.screen == screenTickets {
				m.screen = screenTicketFilter
				m.ticketFilterFocus = 0
				// Pre-select work stream status
				switch m.ticketWorkStreamStatusFilter {
				case "closed":
					m.ticketFilterStatusSelected = 1
				case "all":
					m.ticketFilterStatusSelected = 2
				default:
					m.ticketFilterStatusSelected = 0
				}
				// Pre-select work stream
				m.ticketFilterWorkStreamSelected = 0
				for i, ws := range m.workStreams {
					if ws.Id != nil && *ws.Id == m.ticketWorkStreamFilter {
						m.ticketFilterWorkStreamSelected = i + 1
						break
					}
				}
				// Pre-select state
				stateOpts := []string{"active", "all", "pending", "awaiting_review", "done", "blocked", "needs_human", "claimed", "executing", "failed"}
				m.ticketFilterStateSelected = 0
				for i, s := range stateOpts {
					if s == m.ticketStateFilter {
						m.ticketFilterStateSelected = i
						break
					}
				}
				// Load work streams if not yet loaded (or always, to respect status filter)
				status := m.ticketWorkStreamStatusFilter
				if status == "" {
					status = "active"
				}
				return m, tea.Batch(loadWorkStreams(m.api, m.projectID, status), m.spinnerTickCmd())
			}
			if m.screen == screenProjects {
				m.screen = screenProjectFilter
				// Pre-select current filter
				switch m.projectStatus {
				case "closed":
					m.selected = 1
				case "all":
					m.selected = 2
				default:
					m.selected = 0
				}
				return m, nil
			}
			if m.screen == screenWorkStreams {
				// Cycle filter: active -> closed -> all -> active
				switch m.workStreamStatusFilter {
				case "active":
					m.workStreamStatusFilter = "closed"
				case "closed":
					m.workStreamStatusFilter = "all"
				case "all":
					m.workStreamStatusFilter = "active"
				default:
					m.workStreamStatusFilter = "active"
				}
				m.loading = true
				m.selected = 0
				return m, tea.Batch(loadWorkStreamsForList(m.api, m.projectID, m.workStreamStatusFilter), m.spinnerTickCmd())
			}
		case "b", "esc":
			// Esc cancels filter modals without applying: exit with current filter values.
			// Only Enter applies selections; unconfirmed UI state is discarded.
			if m.screen == screenTicketFilter {
				m.screen = screenTickets
				m.loading = true
				return m, tea.Batch(loadTickets(m.api, m.projectID, m.ticketWorkStreamFilter, m.ticketStateFilter), m.spinnerTickCmd())
			}
			if m.screen == screenProjectFilter {
				m.screen = screenProjects
				return m, nil
			}
			if m.screen == screenReviewDecision && m.confirmReject {
				m.confirmReject = false
				return m, nil
			}
			return m.handleBack()
		case "a":
			if m.screen == screenReviewDecision && !m.confirmReject {
				return m, submitReview(m.api, m.ticketID, "approved")
			}
		case "r":
			if m.screen == screenReviewDecision {
				if m.confirmReject {
					// already in confirm; r does nothing
				} else {
					m.confirmReject = true
					return m, nil
				}
			}
		case "y":
			if m.screen == screenReviewDecision && m.confirmReject {
				m.confirmReject = false
				return m, submitReview(m.api, m.ticketID, "rejected")
			}
		case "n":
			if m.screen == screenReviewDecision && m.confirmReject {
				m.confirmReject = false
				return m, nil
			}
		}
		return m, nil
	case tokenMsg:
		m.token = msg.token
		m.err = msg.err
		if msg.err != "" {
			return m, nil
		}
		_ = writeTokenCache(m.baseURL, m.token)
		var err error
		m.api, err = client.NewClientWithResponses(m.baseURL, m.authEditor())
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.screen = screenOrgSelect
		m.loading = true
		return m, tea.Batch(loadOrgs(m.api), loadStats(m.api), loadStatsHistory(m.api, 14), m.spinnerTickCmd())
	case statsMsg:
		m.meStats = msg.stats
		if msg.stats != nil {
			for _, milestone := range []int{10, 25, 50, 100, 250, 500} {
				if msg.stats.TicketsCreated == milestone {
					m.celebrationMsg = fmt.Sprintf("🎉 %d tickets created!", milestone)
					break
				}
				if msg.stats.ReviewsApproved == milestone {
					m.celebrationMsg = fmt.Sprintf("🎉 %d reviews approved!", milestone)
					break
				}
				if msg.stats.ReviewsRejected == milestone {
					m.celebrationMsg = fmt.Sprintf("🎉 %d reviews rejected!", milestone)
					break
				}
			}
		}
		m.updateAchievements()
		return m, nil
	case statsHistoryMsg:
		m.meStatsHistory = msg.history
		m.updateAchievements()
		return m, nil
	case orgsMsg:
		m.loading = false
		m.orgs = msg.orgs
		m.err = msg.err
		if msg.err != "" {
			_ = clearTokenCache()
			m.token = ""
			m.api = nil
			m.screen = screenLogin
		} else {
			m.screen = screenOrgSelect
			if len(m.orgs) > 0 {
				m.selected = 0
			}
		}
		return m, nil
	case orgSelectedMsg:
		m.orgID = msg.orgID
		m.screen = screenProjects
		m.selected = 0
		m.loading = true
		return m, tea.Batch(loadProjects(m.api, m.orgID, m.projectStatus), loadStats(m.api), loadStatsHistory(m.api, 14), m.spinnerTickCmd())
	case projectsMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == "" {
			m.projects = msg.projects
			m.screen = screenProjects
			if len(m.projects) > 0 {
				m.selected = 0
			}
			if m.projectStatus == "" {
				m.projectStatus = "active"
			}
		}
		return m, nil
	case gitRemotesMsg:
		m.loading = false
		m.settingsGitRemotes = msg.remotes
		m.settingsGitRemotesErr = msg.err
		if len(m.settingsGitRemotes) > 0 {
			m.selected = msg.selectedIndex
		}
		return m, nil
	case workStreamsMsg:
		m.workStreams = msg.workStreams
		m.workStreamsErr = msg.err
		if m.screen == screenTickets && len(m.tickets) > 0 {
			m.ticketList.SetItems(buildTicketListItems(m.tickets, m.workStreams, m.ticketWorkStreamFilter))
		}
		return m, nil
	case workStreamsListMsg:
		m.loading = false
		m.workStreamsList = msg.workStreams
		m.workStreamsErr = msg.err
		return m, nil
	case workStreamUpdatedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == "" {
			m.err = ""
			m.loading = true
			wsStatus := m.ticketWorkStreamStatusFilter
			if wsStatus == "" {
				wsStatus = "active"
			}
			return m, tea.Batch(
				loadWorkStreamsForList(m.api, m.projectID, m.workStreamStatusFilter),
				loadWorkStreams(m.api, m.projectID, wsStatus), // refresh ticket filter
				m.spinnerTickCmd(),
			)
		}
		return m, nil
	case projectUpdatedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == "" {
			if m.screen == screenProjectSettings {
				m.screen = screenProjectMenu
				m.selected = 0
			}
			m.loading = true
			// Preserve current filter
			status := m.projectStatus
			if status == "" {
				status = "active"
			}
			return m, tea.Batch(loadProjects(m.api, m.orgID, status), m.spinnerTickCmd())
		}
		return m, nil
	case projectCreatedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == "" {
			m.successMsg = "✓ Project created"
			m.screen = screenProjects
			m.selected = 0
			m.loading = true
			return m, tea.Batch(loadProjects(m.api, m.orgID, m.projectStatus), m.spinnerTickCmd())
		}
		return m, nil
	case ticketsMsg:
		m.loading = false
		m.tickets = msg.tickets
		if m.ticketStateFilter == "active" {
			active := []client.Ticket{}
			for _, t := range m.tickets {
				s := ""
				if t.State != nil {
					s = string(*t.State)
				}
				switch s {
				case "pending", "claimed", "executing", "awaiting_review":
					active = append(active, t)
				}
			}
			m.tickets = active
		}
		m.err = msg.err
		m.screen = screenTickets
		items := buildTicketListItems(m.tickets, m.workStreams, m.ticketWorkStreamFilter)
		w := contentWidth(m)
		if w <= 0 {
			w = 120
		}
		h := m.height - 12
		if h < 4 {
			h = 4
		}
		m.ticketList = list.New(items, list.NewDefaultDelegate(), w, h)
		m.ticketList.SetShowStatusBar(false)
		m.ticketList.SetShowFilter(false)
		m.ticketList.SetShowHelp(false)
		m.ticketList.SetShowTitle(false)
		m.ticketList.SetShowPagination(false)
		if len(m.tickets) > 0 {
			m.selected = 0
		}
		return m, nil
	case reviewsMsg:
		m.loading = false
		m.reviews = msg.tickets
		m.err = msg.err
		m.screen = screenPendingReviews
		if len(m.reviews) > 0 {
			m.selected = 0
		}
		return m, nil
	case reviewDoneMsg:
		m.err = msg.err
		if msg.err == "" {
			m.reviewTicket = nil
			m.reviewTrace = nil
			m.detailVP = viewport.Model{}
			m.screen = screenPendingReviews
			m.loading = true
			if msg.decision == "approved" {
				m.successMsg = "✓ Approved"
			} else if msg.decision == "rejected" {
				m.successMsg = "✓ Rejected"
			}
			// Terminal bell for immediate feedback (platform-appropriate)
			fmt.Fprint(os.Stderr, "\a")
			return m, tea.Batch(loadPendingReviews(m.api, m.projectID), m.spinnerTickCmd())
		}
		return m, nil
	case traceMsg:
		m.loading = false
		if msg.err != "" {
			m.err = "trace: " + msg.err
			return m, nil
		}
		if m.screen == screenTicketDetail && m.detailTicket != nil && m.detailTicket.Id != nil && *m.detailTicket.Id == msg.ticketID {
			m.detailTrace = msg.trace
			w, h := contentWidth(m), contentHeight(m)
			m.detailVP = viewport.New(w, h)
			m.detailVP.SetContent(components.CardRender("Ticket detail", formatTicketBody(m.detailTicket, m.detailTrace, false, w), w))
			m.detailVP.GotoTop()
			return m, nil
		}
		if m.screen == screenReviewDecision && m.ticketID == msg.ticketID {
			m.reviewTrace = msg.trace
			w, h := contentWidth(m), contentHeight(m)
			m.detailVP = viewport.New(w, h)
			m.detailVP.SetContent(components.CardRender("Review ticket", formatTicketBody(m.reviewTicket, m.reviewTrace, true, w), w))
			m.detailVP.GotoTop()
			return m, nil
		}
		return m, nil
	case gitNotesLogMsg:
		m.loading = false
		m.gitNotesErr = msg.err
		if msg.err == "" {
			m.gitNotesLog = msg.entries
			m.selected = 0
		}
		return m, nil
	}
	return m, nil
}

func (m model) listLen() int {
	switch m.screen {
	case screenLogin:
		return 0
	case screenOrgSelect:
		return len(m.orgs)
	case screenProjects:
		// +1 for "Create project" option
		return len(m.projects) + 1
	case screenProjectMenu:
		return 7
	case screenTickets:
		return len(m.tickets)
	case screenTicketDetail:
		return 0
	case screenPendingReviews:
		return len(m.reviews)
	case screenReviewDecision:
		return 0
	case screenGitNotesLog:
		return len(m.gitNotesLog)
	case screenGitNoteDetail:
		return 0
	case screenProjectSettings:
		if m.settingsFocus == 3 {
			return len(m.settingsGitRemotes)
		}
		return 0
	case screenCreateProject:
		return 0
	case screenProjectFilter:
		return 3
	case screenTicketFilter:
		switch m.ticketFilterFocus {
		case 0:
			return 3 // Active only, Closed only, All
		case 1:
			return 1 + len(m.workStreams)
		case 2:
			return 9
		}
		return 0
	case screenWorkStreams:
		return len(m.workStreamsList)
	}
	return 0
}

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenLogin:
		return m, startLoginFlow(m.baseURL)
	case screenOrgSelect:
		if m.selected >= 0 && m.selected < len(m.orgs) {
			if id := m.orgs[m.selected].Id; id != nil {
				return m, selectOrg(*id)
			}
		}
		return m, nil
	case screenProjects:
		if m.selected == len(m.projects) {
			// "+ Create project" selected
			m.screen = screenCreateProject
			ti := textinput.New()
			ti.Placeholder = "My Project"
			ti.Width = 40
			ti.PromptStyle = components.Primary
			ti.TextStyle = components.Secondary
			ti.Focus()
			m.createProjectNameInput = ti
			return m, textinput.Blink
		}
		if m.selected >= 0 && m.selected < len(m.projects) {
			if id := m.projects[m.selected].Id; id != nil {
				m.projectID = *id
			}
			m.screen = screenProjectMenu
			m.selected = 0
			m.successMsg = ""
		}
		return m, nil
	case screenProjectMenu:
		switch m.selected {
		case 0:
			m.loading = true
			status := m.ticketWorkStreamStatusFilter
			if status == "" {
				status = "active"
			}
			if m.ticketStateFilter == "" {
				m.ticketStateFilter = "active"
			}
			return m, tea.Batch(loadTickets(m.api, m.projectID, m.ticketWorkStreamFilter, m.ticketStateFilter), loadWorkStreams(m.api, m.projectID, status))
		case 1:
			m.loading = true
			return m, tea.Batch(loadPendingReviews(m.api, m.projectID), m.spinnerTickCmd())
		case 2:
			// Agent decisions (git notes)
			slug := projectSlug(m)
			repoPath := resolveRepoPath(slug)
			m.gitNotesLog = nil
			m.gitNotesErr = ""
			m.noteTypeFilter = "decision"
			m.screen = screenGitNotesLog
			m.loading = true
			return m, tea.Batch(loadGitNotesLog(m.api, m.orgID, m.projectID, repoPath, m.noteTypeFilter, 20), m.spinnerTickCmd())
		case 3:
			// Work streams (manage, close/reopen)
			m.workStreamsList = nil
			m.workStreamsErr = ""
			m.workStreamStatusFilter = "active"
			m.screen = screenWorkStreams
			m.selected = 0
			m.loading = true
			return m, tea.Batch(loadWorkStreamsForList(m.api, m.projectID, m.workStreamStatusFilter), m.spinnerTickCmd())
		case 4:
			// Project settings (name, slug, work streams + git integration)
			m.screen = screenProjectSettings
			m.settingsGitRemotes = nil
			m.settingsGitRemotesErr = ""
			m.settingsRepoPath = resolveRepoPath(projectSlug(m))
			name, slug, defaultBranch := "", "", "main"
			for _, p := range m.projects {
				if p.Id != nil && *p.Id == m.projectID {
					if p.Name != nil {
						name = *p.Name
					}
					if p.Slug != nil {
						slug = *p.Slug
					}
					if p.DefaultBranch != nil && *p.DefaultBranch != "" {
						defaultBranch = *p.DefaultBranch
					}
					break
				}
			}
			tiName := textinput.New()
			tiName.Placeholder = "Project name"
			tiName.Width = 40
			tiName.SetValue(name)
			tiName.PromptStyle = components.Primary
			tiName.TextStyle = components.Secondary
			tiSlug := textinput.New()
			tiSlug.Placeholder = "project-slug"
			tiSlug.Width = 40
			tiSlug.SetValue(slug)
			tiSlug.PromptStyle = components.Primary
			tiSlug.TextStyle = components.Secondary
			tiDefaultBranch := textinput.New()
			tiDefaultBranch.Placeholder = "main"
			tiDefaultBranch.Width = 20
			tiDefaultBranch.SetValue(defaultBranch)
			tiDefaultBranch.PromptStyle = components.Primary
			tiDefaultBranch.TextStyle = components.Secondary
			tiName.Focus()
			tiSlug.Blur()
			tiDefaultBranch.Blur()
			m.settingsNameInput = tiName
			m.settingsSlugInput = tiSlug
			m.settingsDefaultBranchInput = tiDefaultBranch
			m.settingsFocus = 0
			m.loading = true
			repoURL := ""
			for _, p := range m.projects {
				if p.Id != nil && *p.Id == m.projectID && p.RepoUrl != nil {
					repoURL = *p.RepoUrl
					break
				}
			}
			return m, tea.Batch(textinput.Blink, loadGitRemotes(m.settingsRepoPath, repoURL), m.spinnerTickCmd())
		case 5:
			// Close or Reopen project
			for _, p := range m.projects {
				if p.Id != nil && *p.Id == m.projectID {
					if p.Status != nil && *p.Status == client.ProjectStatusClosed {
						m.loading = true
						return m, tea.Batch(updateProject(m.api, m.projectID, string(client.UpdateProjectRequestStatusActive)), m.spinnerTickCmd())
					}
					m.loading = true
					return m, tea.Batch(updateProject(m.api, m.projectID, string(client.UpdateProjectRequestStatusClosed)), m.spinnerTickCmd())
				}
			}
			return m, nil
		case 6:
			m.screen = screenProjects
			m.selected = 0
			m.loading = true
			status := m.projectStatus
			if status == "" {
				status = "active"
			}
			return m, tea.Batch(loadProjects(m.api, m.orgID, status), m.spinnerTickCmd())
		}
		return m, nil
	case screenWorkStreams:
		if m.selected >= 0 && m.selected < len(m.workStreamsList) {
			ws := m.workStreamsList[m.selected]
			if ws.Id == nil {
				return m, nil
			}
			// Toggle close/reopen
			status := "closed"
			if ws.Status != nil && string(*ws.Status) == "closed" {
				status = "active"
			}
			m.loading = true
			return m, tea.Batch(updateWorkStream(m.api, m.projectID, *ws.Id, status), m.spinnerTickCmd())
		}
		return m, nil
	case screenGitNotesLog:
		if m.selected >= 0 && m.selected < len(m.gitNotesLog) {
			e := m.gitNotesLog[m.selected]
			commitSHA := ""
			if e.CommitSha != nil {
				commitSHA = *e.CommitSha
			}
			body := ""
			if e.Body != nil {
				body = *e.Body
			}
			m.gitNoteDetail = parseGitNoteDetail(commitSHA, body)
			m.screen = screenGitNoteDetail
		}
		return m, nil
	case screenTickets:
		if m.selected >= 0 && m.selected < len(m.tickets) {
			t := &m.tickets[m.selected]
			if id := t.Id; id != nil {
				m.detailTicket = t
				m.detailTrace = nil
				m.screen = screenTicketDetail
				m.loading = true
				return m, tea.Batch(loadTrace(m.api, *id), m.spinnerTickCmd())
			}
		}
		return m, nil
	case screenPendingReviews:
		if m.selected >= 0 && m.selected < len(m.reviews) {
			t := &m.reviews[m.selected]
			if id := t.Id; id != nil {
				m.ticketID = *id
				m.reviewTicket = t
				m.reviewTrace = nil
				m.screen = screenReviewDecision
				m.loading = true
				return m, tea.Batch(loadTrace(m.api, *id), m.spinnerTickCmd())
			}
		}
		return m, nil
	}
	return m, nil
}

func (m model) handleBack() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenOrgSelect:
		_ = clearTokenCache()
		m.screen = screenLogin
		m.token = ""
		m.api = nil
		m.orgID = ""
		return m, nil
	case screenProjectMenu:
		m.screen = screenProjects
		m.selected = 0
		m.loading = true
		status := m.projectStatus
		if status == "" {
			status = "active"
		}
		return m, tea.Batch(loadProjects(m.api, m.orgID, status), m.spinnerTickCmd())
	case screenProjectSettings:
		m.screen = screenProjectMenu
		m.selected = 0
		return m, nil
	case screenCreateProject:
		m.screen = screenProjects
		m.selected = 0
		return m, nil
	case screenGitNotesLog:
		m.screen = screenProjectMenu
		m.selected = 0
		m.gitNotesLog = nil
		m.gitNotesErr = ""
		return m, nil
	case screenGitNoteDetail:
		m.screen = screenGitNotesLog
		m.gitNoteDetail = nil
		return m, nil
	case screenWorkStreams:
		m.screen = screenProjectMenu
		m.selected = 0
		m.workStreamsList = nil
		m.workStreamsErr = ""
		return m, nil
	case screenTicketDetail:
		m.screen = screenTickets
		m.detailTicket = nil
		m.detailTrace = nil
		m.detailVP = viewport.Model{}
		return m, nil
	case screenTickets, screenPendingReviews:
		m.screen = screenProjectMenu
		m.selected = 0
		m.successMsg = ""
		return m, nil
	case screenReviewDecision:
		m.screen = screenPendingReviews
		m.reviewTicket = nil
		m.reviewTrace = nil
		m.confirmReject = false
		m.detailVP = viewport.Model{}
		return m, nil
	case screenProjects:
		m.screen = screenOrgSelect
		m.projectID = ""
		m.successMsg = ""
		return m, tea.Batch(loadStats(m.api), loadStatsHistory(m.api, 14))
	}
	return m, tea.Quit
}

func (m model) View() string {
	var b strings.Builder
	if m.err != "" {
		w := contentWidth(m)
		b.WriteString(components.Border.Width(w).Padding(1, 2).Render(components.Error.Render(m.err)) + "\n\n")
	}
	switch m.screen {
	case screenLogin:
		b.WriteString(components.Primary.Render("Flywheel") + "\n")
		b.WriteString(components.Muted.Render("Agent work tracking & review") + "\n\n")
		b.WriteString("Not logged in.\n\n")
		b.WriteString(components.Border.Width(40).Padding(0, 1).Render(components.Primary.Render("Log in with GitHub") + " (opens browser)") + "\n\n")
		b.WriteString(components.Muted.Render("Press Enter to open the browser and sign in. You'll be redirected back here."))
	case screenOrgSelect:
		if m.loading {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Loading…"))
			break
		}
		items := make([]string, 0, len(m.orgs))
		for _, o := range m.orgs {
			items = append(items, str(o.Name)+" ("+str(o.Slug)+")")
		}
		list := components.SelectList{Items: items, Selected: m.selected, EmptyMessage: "Sign in with GitHub to see your orgs."}
		b.WriteString(components.Primary.Render("Select organization") + "\n\n")
		b.WriteString(list.Render(m.width))
	case screenProjects:
		if m.loading {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Loading…"))
			break
		}
		if m.successMsg != "" {
			b.WriteString(components.Success.Render(m.successMsg) + "\n\n")
		}
		items := make([]string, 0, len(m.projects)+1)
		for _, p := range m.projects {
			items = append(items, str(p.Name)+" ("+str(p.Slug)+")")
		}
		if len(m.projects) == 0 {
			items = append(items, "+ Create your first project")
		} else {
			items = append(items, "+ Create project")
		}
		list := components.SelectList{Items: items, Selected: m.selected, EmptyMessage: ""}
		b.WriteString(components.Primary.Render("Projects") + "\n\n")
		b.WriteString(list.Render(m.width))
	case screenProjectMenu:
		closeReopen := "Close project"
		for _, p := range m.projects {
			if p.Id != nil && *p.Id == m.projectID && p.Status != nil && *p.Status == client.ProjectStatusClosed {
				closeReopen = "Reopen project"
				break
			}
		}
		menus := []string{"List tickets", "Pending reviews", "Agent decisions", "Work streams", "Project settings", closeReopen, "Back to projects"}
		list := components.SelectList{Items: menus, Selected: m.selected, EmptyMessage: ""}
		projectLabel := m.projectID
		for _, p := range m.projects {
			if p.Id != nil && *p.Id == m.projectID {
				if p.Name != nil && p.Slug != nil {
					projectLabel = *p.Name + " (" + *p.Slug + ")"
				} else if p.Name != nil {
					projectLabel = *p.Name
				} else if p.Slug != nil {
					projectLabel = *p.Slug
				}
				break
			}
		}
		b.WriteString(components.Primary.Render("Project: "+projectLabel) + "\n\n")
		b.WriteString(list.Render(m.width))
	case screenTickets:
		if m.loading {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Loading…"))
			break
		}
		b.WriteString(components.Primary.Render("Tickets") + "\n\n")
		if line := formatTicketStatsLine(m.tickets); line != "" {
			b.WriteString(line + "\n\n")
		}
		if len(m.tickets) == 0 {
			b.WriteString(components.Muted.Render("No tickets yet — create one via MCP or the API to get started."))
		} else {
			b.WriteString(m.ticketList.View())
		}
	case screenPendingReviews:
		if m.loading {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Loading…"))
			break
		}
		if m.successMsg != "" {
			b.WriteString(components.Success.Render(m.successMsg) + "\n\n")
		}
		items := make([]string, 0, len(m.reviews))
		for _, t := range m.reviews {
			items = append(items, str(t.Id)+" "+str(t.Title))
		}
		emptyMsg := "Great work! Queue is clear.\n\nTickets move here when an agent marks them \"awaiting_review\".\nCreate tickets, claim and complete work, then submit for review."
		list := components.SelectList{Items: items, Selected: m.selected, EmptyMessage: emptyMsg}
		b.WriteString(components.Primary.Render("Pending reviews") + "\n\n")
		b.WriteString(components.Muted.Render(fmt.Sprintf("%d ticket(s) awaiting review", len(m.reviews))) + "\n\n")
		b.WriteString(list.Render(m.width))
	case screenTicketDetail:
		if m.loading && m.detailTrace == nil {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Loading trace…"))
			break
		}
		if m.detailTicket != nil {
			if m.detailVP.Height > 0 {
				b.WriteString(m.detailVP.View())
			} else {
				w := contentWidth(m)
				body := formatTicketBody(m.detailTicket, m.detailTrace, false, w)
				b.WriteString(components.CardRender("Ticket detail", body, w))
			}
		}
		b.WriteString("\n  [b] Back\n")
	case screenReviewDecision:
		if m.confirmReject {
			b.WriteString(components.Muted.Render("Reject this ticket? [y/N]"))
			break
		}
		if m.loading {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Loading…"))
			break
		}
		if m.reviewTicket != nil {
			if m.detailVP.Height > 0 {
				b.WriteString(m.detailVP.View())
			} else {
				w := contentWidth(m)
				body := formatTicketBody(m.reviewTicket, m.reviewTrace, true, w)
				b.WriteString(components.CardRender("Review ticket", body, w))
			}
		}
		b.WriteString("\n" + components.Muted.Render("Look for: objective met, clear outputs, sensible trace.") + "\n")
		b.WriteString("\n  [a] Approve  [r] Reject  [b] Back\n")
	case screenGitNotesLog:
		if m.loading {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Loading…"))
			break
		}
		if m.gitNotesErr != "" {
			b.WriteString(components.Error.Render(m.gitNotesErr) + "\n\n")
			b.WriteString(components.Muted.Render("Set FLYWHEEL_REPO_PATH or run from project root. Or use: flywheel-git note log -t decision -n 20"))
			break
		}
		items := make([]string, 0, len(m.gitNotesLog))
		for _, e := range m.gitNotesLog {
			sha := ""
			if e.CommitSha != nil {
				s := *e.CommitSha
				if len(s) > 7 {
					sha = s[:7]
				} else {
					sha = s
				}
			}
			noteType := "decision"
			if e.Ref != nil {
				ref := *e.Ref
				if idx := strings.LastIndex(ref, "/"); idx >= 0 {
					noteType = ref[idx+1:]
				}
			}
			msg := ""
			if e.Body != nil {
				var v struct {
					Message string `json:"message"`
				}
				if json.Unmarshal([]byte(*e.Body), &v) == nil && v.Message != "" {
					msg = v.Message
					if len(msg) > 60 {
						msg = msg[:57] + "..."
					}
				}
			}
			items = append(items, fmt.Sprintf("%s %s %s", sha, noteType, msg))
		}
		emptyMsg := "No agent decisions yet. Agents add notes when they complete work via submit_ticket."
		list := components.SelectList{Items: items, Selected: m.selected, EmptyMessage: emptyMsg}
		b.WriteString(components.Primary.Render("Agent decisions") + "\n\n")
		b.WriteString(list.Render(m.width))
	case screenGitNoteDetail:
		if m.gitNoteDetail != nil {
			w := contentWidth(m)
			var sb strings.Builder
			sb.WriteString("Commit: " + m.gitNoteDetail.CommitSHA + "\n")
			if m.gitNoteDetail.Type != "" {
				sb.WriteString("Type: " + m.gitNoteDetail.Type + "\n")
			}
			sb.WriteString("\n" + m.gitNoteDetail.Message + "\n")
			if m.gitNoteDetail.AgentID != "" {
				sb.WriteString("\nAgent: " + m.gitNoteDetail.AgentID + "\n")
			}
			if m.gitNoteDetail.TicketID != "" {
				sb.WriteString("Ticket: " + m.gitNoteDetail.TicketID + "\n")
			}
			if m.gitNoteDetail.CreatedAt != "" {
				sb.WriteString("Created: " + m.gitNoteDetail.CreatedAt + "\n")
			}
			b.WriteString(components.CardRender("Note detail", sb.String(), w))
		}
		b.WriteString("\n  [b] Back\n")
	case screenWorkStreams:
		if m.loading {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Loading…"))
			break
		}
		if m.workStreamsErr != "" {
			b.WriteString(components.Error.Render(m.workStreamsErr))
			break
		}
		items := make([]string, 0, len(m.workStreamsList))
		for _, ws := range m.workStreamsList {
			name := ""
			if ws.Name != nil {
				name = *ws.Name
			}
			branch := ""
			if ws.Branch != nil && *ws.Branch != "" {
				branch = " [" + *ws.Branch + "]"
			}
			status := ""
			if ws.Status != nil {
				status = " " + string(*ws.Status)
			}
			items = append(items, name+branch+components.Muted.Render(status))
		}
		filterLabel := m.workStreamStatusFilter
		if filterLabel == "" {
			filterLabel = "active"
		}
		emptyMsg := "No work streams. Create them via MCP (create_work_stream). Work streams group tickets and can be tied to branches."
		list := components.SelectList{Items: items, Selected: m.selected, EmptyMessage: emptyMsg}
		b.WriteString(components.Primary.Render("Work streams") + " " + components.Muted.Render("(filter: "+filterLabel+")") + "\n\n")
		b.WriteString(components.Muted.Render("Enter to close/reopen · closed work streams disappear from ticket filter") + "\n\n")
		b.WriteString(list.Render(m.width))
	case screenProjectSettings:
		if m.loading && len(m.settingsGitRemotes) == 0 {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Loading…"))
			break
		}
		if m.loading {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Saving…"))
			break
		}
		b.WriteString(components.Primary.Render("Project settings") + "\n\n")
		b.WriteString(components.Muted.Render("Name:") + "\n")
		b.WriteString(m.settingsNameInput.View())
		b.WriteString("\n\n" + components.Muted.Render("Slug:") + "\n")
		b.WriteString(m.settingsSlugInput.View())
		b.WriteString("\n\n" + components.Muted.Render("Default branch (when closing work stream):") + "\n")
		b.WriteString(m.settingsDefaultBranchInput.View())
		b.WriteString("\n\n" + components.Muted.Render("Work streams + git integration (remote URL):") + "\n\n")
		if m.settingsGitRemotesErr != "" {
			b.WriteString(components.Error.Render(m.settingsGitRemotesErr) + "\n\n")
		}
		if len(m.settingsGitRemotes) > 0 {
			items := make([]string, len(m.settingsGitRemotes))
			for i, r := range m.settingsGitRemotes {
				if r == "Off" {
					items[i] = "Off"
				} else {
					items[i] = r
				}
			}
			list := components.SelectList{Items: items, Selected: m.selected, EmptyMessage: ""}
			b.WriteString(list.Render(m.width))
		}
		b.WriteString("\n\n" + components.Muted.Render("Tab to switch · Enter to save · esc to cancel"))
	case screenCreateProject:
		if m.loading {
			b.WriteString(m.spinner.View() + " " + components.Muted.Render("Creating…"))
			break
		}
		b.WriteString(components.Primary.Render("Create project") + "\n\n")
		b.WriteString(components.Muted.Render("Project name:") + "\n\n")
		b.WriteString(m.createProjectNameInput.View())
		b.WriteString("\n\n" + components.Muted.Render("Enter to create · esc to cancel"))
	case screenProjectFilter:
		items := []string{"Active only", "Closed only", "All"}
		list := components.SelectList{Items: items, Selected: m.selected, EmptyMessage: ""}
		b.WriteString(components.Primary.Render("Filter projects") + "\n\n")
		b.WriteString(components.Muted.Render("Show:") + "\n\n")
		b.WriteString(list.Render(m.width))
		b.WriteString("\n\n" + components.Muted.Render("Enter to apply · esc to cancel"))
	case screenTicketFilter:
		if m.workStreamsErr != "" {
			b.WriteString(components.Error.Render(m.workStreamsErr) + "\n\n")
		}
		statusItems := []string{"Active only", "Closed only", "All"}
		wsItems := []string{"All"}
		for _, ws := range m.workStreams {
			name := ""
			if ws.Name != nil {
				name = *ws.Name
			}
			slug := ""
			if ws.Slug != nil {
				slug = *ws.Slug
			}
			branch := ""
			if ws.Branch != nil && *ws.Branch != "" {
				branch = " [" + *ws.Branch + "]"
			}
			wsItems = append(wsItems, name+" ("+slug+")"+branch)
		}
		stateItems := []string{"Active", "All", "Pending", "Awaiting review", "Done", "Blocked", "Needs human", "Claimed", "Executing", "Failed"}
		form := components.FilterForm{
			Section1Label:    "Work stream status:",
			Section1Items:    statusItems,
			Section1Selected: m.ticketFilterStatusSelected,
			Section2Label:    "Work stream:",
			Section2Items:    wsItems,
			Section2Selected: m.ticketFilterWorkStreamSelected,
			Section3Label:    "State:",
			Section3Items:    stateItems,
			Section3Selected: m.ticketFilterStateSelected,
			Focus:            m.ticketFilterFocus,
		}
		b.WriteString(components.Primary.Render("Filter tickets") + "\n\n")
		b.WriteString(form.Render(m.width))
		b.WriteString("\n\n" + components.Muted.Render("tab switch section · enter to set · esc to cancel"))
	}
	body := b.String()
	w := contentWidth(m)
	var contentArea string
	if m.screen == screenTicketDetail || m.screen == screenReviewDecision || m.screen == screenGitNoteDetail || m.screen == screenProjectSettings || m.screen == screenCreateProject || m.screen == screenProjectFilter || m.screen == screenTicketFilter {
		// Single card, no outer panel (avoids double border and uses full width)
		contentArea = body
	} else {
		contentArea = renderContentPanel(body, w)
	}
	helpHints := []string{"↑/k ↓/j select", "enter choose", "b/esc back", "q quit"}
	if m.screen == screenReviewDecision && m.confirmReject {
		helpHints = []string{"y confirm", "n/Esc cancel"}
	} else if m.screen == screenReviewDecision {
		if m.detailVP.Height > 0 {
			helpHints = []string{"↑/↓ pgup/pgdn scroll", "a approve", "r reject", "b back"}
		} else {
			helpHints = []string{"a approve", "r reject", "b back"}
		}
	} else if m.screen == screenTicketDetail {
		if m.detailVP.Height > 0 {
			helpHints = []string{"↑/↓ pgup/pgdn scroll", "b back"}
		} else {
			helpHints = []string{"b back"}
		}
	} else if m.screen == screenGitNoteDetail {
		helpHints = []string{"b back"}
	} else if m.screen == screenProjectSettings {
		helpHints = []string{"tab switch", "enter save", "esc cancel"}
	} else if m.screen == screenCreateProject {
		helpHints = []string{"enter create", "esc cancel"}
	} else if m.screen == screenProjectFilter || m.screen == screenTicketFilter {
		h := "↑/k ↓/j select"
		if m.screen == screenTicketFilter {
			h += " · tab switch"
		}
		helpHints = []string{h, "enter apply", "esc cancel"}
	} else if m.screen == screenGitNotesLog {
		helpHints = []string{"↑/k ↓/j select", "enter view", "b/esc back", "q quit"}
	} else if m.screen == screenWorkStreams {
		filterLabel := m.workStreamStatusFilter
		if filterLabel == "" {
			filterLabel = "active"
		}
		helpHints = []string{"↑/k ↓/j select", "enter close/reopen", "f filter (" + filterLabel + ")", "b/esc back", "q quit"}
	} else if m.screen == screenProjects {
		filterLabel := "active"
		if m.projectStatus != "" {
			filterLabel = m.projectStatus
		}
		helpHints = []string{"↑/k ↓/j select", "enter choose", "f filter (" + filterLabel + ")", "b/esc back", "q quit"}
	} else if m.screen == screenTickets {
		statusLabel := m.ticketWorkStreamStatusFilter
		if statusLabel == "" {
			statusLabel = "active"
		}
		wsLabel := "all"
		if m.ticketWorkStreamFilter != "" {
			for _, ws := range m.workStreams {
				if ws.Id != nil && *ws.Id == m.ticketWorkStreamFilter && ws.Name != nil {
					wsLabel = *ws.Name
					break
				}
			}
		}
		stateLabel := m.ticketStateFilter
		if stateLabel == "" {
			stateLabel = "active"
		}
		filterLabel := statusLabel + " / " + wsLabel + " / " + stateLabel
		helpHints = []string{"↑/k ↓/j select", "enter choose", "f filter (" + filterLabel + ")", "b/esc back", "q quit"}
	}
	return renderHeader(m) + contentArea + "\n" + renderHelpBar(helpHints)
}

type tokenMsg struct {
	token string
	err   string
}
type orgsMsg struct {
	orgs []client.Org
	err  string
}
type orgSelectedMsg struct {
	orgID string
}
type projectsMsg struct {
	projects []client.Project
	err      string
}
type ticketsMsg struct {
	tickets []client.Ticket
	err     string
}
type workStreamsMsg struct {
	workStreams []client.WorkStream
	err         string
}
type workStreamsListMsg struct {
	workStreams []client.WorkStream
	err         string
}
type reviewsMsg struct {
	tickets []client.Ticket
	err     string
}
type reviewDoneMsg struct {
	err      string
	decision string // "approved" or "rejected" when err == ""
}
type traceMsg struct {
	ticketID string
	trace    *client.ExecutionTrace
	err      string
}
type projectUpdatedMsg struct {
	err string
}
type projectCreatedMsg struct {
	err string
}
type statsMsg struct {
	stats *client.MeStats
}
type statsHistoryMsg struct {
	history *client.MeStatsHistory
}
type gitRemotesMsg struct {
	remotes       []string
	selectedIndex int
	err           string
}

type gitNotesLogMsg struct {
	entries []client.GitNotesLogEntry
	err     string
}

func startLoginFlow(baseURL string) tea.Cmd {
	return func() tea.Msg {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return tokenMsg{err: "could not start callback server: " + err.Error()}
		}
		port := listener.Addr().(*net.TCPAddr).Port
		callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
		loginURL := baseURL + "/auth/github?redirect_uri=" + url.QueryEscape(callbackURL)

		// Pre-flight: hit login URL before opening browser. Server must return 302 to GitHub.
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := client.Get(loginURL)
		if err != nil {
			return tokenMsg{err: "server unreachable at " + baseURL + ": " + err.Error()}
		}
		if resp.StatusCode != http.StatusTemporaryRedirect && resp.StatusCode != http.StatusFound {
			body := ""
			if resp.Body != nil {
				buf := make([]byte, 1024)
				n, _ := resp.Body.Read(buf)
				if n > 0 {
					body = ": " + strings.TrimSpace(string(buf[:n]))
				}
				resp.Body.Close()
			}
			msg := "login failed: server returned " + resp.Status + body
			if body == "" {
				msg += ". Run the server from this repo with 'go run ./cmd/server' (it loads .env) and try again."
			} else {
				msg += ". Fix the server error and try again."
			}
			return tokenMsg{err: msg}
		}
		resp.Body.Close()

		ch := make(chan string, 1)
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/callback" {
				http.NotFound(w, r)
				return
			}
			token := r.URL.Query().Get("token")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Flywheel</title></head><body><p>Success! You can close this window and return to the TUI.</p></body></html>`))
			select {
			case ch <- token:
			default:
			}
		})}
		go func() {
			_ = srv.Serve(listener)
		}()
		openURL(loginURL)
		var token string
		select {
		case token = <-ch:
		case <-time.After(2 * time.Minute):
			token = ""
		}
		_ = srv.Close()
		if token == "" {
			return tokenMsg{err: "no token received (did you complete sign-in? If the browser showed an error, restart the server and try again.)"}
		}
		return tokenMsg{token: token}
	}
}

func openURL(url string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Start()
	case "linux":
		_ = exec.Command("xdg-open", url).Start()
	case "windows":
		_ = exec.Command("cmd", "/c", "start", url).Start()
	}
}

func selectOrg(orgID string) tea.Cmd {
	return func() tea.Msg {
		return orgSelectedMsg{orgID: orgID}
	}
}

func loadOrgs(api *client.ClientWithResponses) tea.Cmd {
	return func() tea.Msg {
		rsp, err := api.ListOrgsWithResponse(context.Background())
		if err != nil {
			return orgsMsg{err: err.Error()}
		}
		if rsp.JSON200 == nil {
			return orgsMsg{err: "not authorized (list orgs requires OAuth)"}
		}
		return orgsMsg{orgs: *rsp.JSON200}
	}
}

func loadStats(api *client.ClientWithResponses) tea.Cmd {
	return func() tea.Msg {
		rsp, err := api.GetMeStatsWithResponse(context.Background())
		if err != nil {
			return statsMsg{}
		}
		if rsp.JSON200 == nil {
			return statsMsg{}
		}
		return statsMsg{stats: rsp.JSON200}
	}
}

func loadStatsHistory(api *client.ClientWithResponses, days int) tea.Cmd {
	return func() tea.Msg {
		params := &client.GetMeStatsHistoryParams{Days: &days}
		rsp, err := api.GetMeStatsHistoryWithResponse(context.Background(), params)
		if err != nil {
			return statsHistoryMsg{}
		}
		if rsp.JSON200 == nil {
			return statsHistoryMsg{}
		}
		return statsHistoryMsg{history: rsp.JSON200}
	}
}

func loadProjects(api *client.ClientWithResponses, orgID, status string) tea.Cmd {
	return func() tea.Msg {
		var params *client.ListProjectsByOrgParams
		if status != "" {
			s := client.ListProjectsByOrgParamsStatus(status)
			params = &client.ListProjectsByOrgParams{Status: &s}
		}
		rsp, err := api.ListProjectsByOrgWithResponse(context.Background(), orgID, params)
		if err != nil {
			return projectsMsg{err: err.Error()}
		}
		if rsp.JSON200 == nil {
			errMsg := fmt.Sprintf("GET /orgs/%s/projects: %d", orgID, rsp.StatusCode())
			if len(rsp.Body) > 0 && len(rsp.Body) < 200 {
				errMsg += " " + string(rsp.Body)
			} else if len(rsp.Body) > 0 {
				errMsg += " " + string(rsp.Body[:min(200, len(rsp.Body))]) + "..."
			}
			return projectsMsg{err: errMsg}
		}
		return projectsMsg{projects: *rsp.JSON200}
	}
}

func createProjectWithName(api *client.ClientWithResponses, orgID, name string) tea.Cmd {
	return func() tea.Msg {
		slug := slugify(name)
		body := client.CreateProjectJSONRequestBody{
			Name: &name,
			Slug: &slug,
		}
		rsp, err := api.CreateProjectWithResponse(context.Background(), orgID, body)
		if err != nil {
			return projectCreatedMsg{err: err.Error()}
		}
		if rsp.JSON201 == nil {
			return projectCreatedMsg{err: fmt.Sprintf("HTTP %d", rsp.StatusCode())}
		}
		return projectCreatedMsg{}
	}
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "project"
	}
	return s
}

func updateProject(api *client.ClientWithResponses, projectID, status string) tea.Cmd {
	return func() tea.Msg {
		s := client.UpdateProjectRequestStatus(status)
		body := client.UpdateProjectJSONRequestBody{Status: &s}
		rsp, err := api.UpdateProjectWithResponse(context.Background(), projectID, body)
		if err != nil {
			return projectUpdatedMsg{err: err.Error()}
		}
		if rsp.StatusCode() >= 400 {
			return projectUpdatedMsg{err: fmt.Sprintf("HTTP %d", rsp.StatusCode())}
		}
		return projectUpdatedMsg{}
	}
}

func loadGitRemotes(repoPath, currentRepoURL string) tea.Cmd {
	return func() tea.Msg {
		remotes := []string{"Off"}
		selectedIndex := 0
		if repoPath == "" {
			return gitRemotesMsg{remotes: remotes, selectedIndex: 0, err: "No git repo (set FLYWHEEL_REPO_PATH or run from project root)"}
		}
		out, err := exec.Command("git", "-C", repoPath, "remote").Output()
		if err != nil {
			return gitRemotesMsg{remotes: remotes, selectedIndex: 0, err: "Not a git repo or no remotes"}
		}
		names := strings.Fields(strings.TrimSpace(string(out)))
		if len(names) == 0 {
			return gitRemotesMsg{remotes: remotes, selectedIndex: 0, err: "No git remotes configured"}
		}
		for i, name := range names {
			remotes = append(remotes, name)
			if currentRepoURL != "" {
				urlOut, urlErr := exec.Command("git", "-C", repoPath, "remote", "get-url", name).Output()
				if urlErr == nil && strings.TrimSpace(string(urlOut)) == currentRepoURL {
					selectedIndex = i + 1
				}
			}
		}
		if currentRepoURL != "" && selectedIndex == 0 && len(remotes) > 1 {
			selectedIndex = 1
		}
		return gitRemotesMsg{remotes: remotes, selectedIndex: selectedIndex}
	}
}

// saveProjectSettings saves name, slug, default_branch, and repo_url in one PATCH. Always sends all
// so the request body is never empty (avoids "at least one required" error).
func saveProjectSettings(api *client.ClientWithResponses, projectID, repoPath string, remotes []string, selected int, name, slug, defaultBranch string) tea.Cmd {
	return func() tea.Msg {
		var repoURL string
		if selected > 0 && selected < len(remotes) && repoPath != "" {
			out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", remotes[selected]).Output()
			if err != nil {
				return projectUpdatedMsg{err: fmt.Sprintf("git remote get-url: %v", err)}
			}
			repoURL = strings.TrimSpace(string(out))
		}
		if defaultBranch == "" {
			defaultBranch = "main"
		}
		body := client.UpdateProjectJSONRequestBody{
			Name:          &name,
			Slug:          &slug,
			DefaultBranch: &defaultBranch,
			RepoUrl:       &repoURL,
		}
		rsp, err := api.UpdateProjectWithResponse(context.Background(), projectID, body)
		if err != nil {
			return projectUpdatedMsg{err: err.Error()}
		}
		if rsp.StatusCode() >= 400 {
			errMsg := fmt.Sprintf("HTTP %d", rsp.StatusCode())
			if rsp.JSON400 != nil && rsp.JSON400.Error != "" {
				errMsg = rsp.JSON400.Error
			}
			return projectUpdatedMsg{err: errMsg}
		}
		return projectUpdatedMsg{}
	}
}

func loadTickets(api *client.ClientWithResponses, projectID, workStreamID, state string) tea.Cmd {
	return func() tea.Msg {
		params := &client.ListTicketsParams{}
		if workStreamID != "" {
			params.WorkStreamId = &workStreamID
		}
		// "active" and "all" fetch everything; we filter client-side for "active"
		if state != "" && state != "active" && state != "all" {
			s := client.ListTicketsParamsState(state)
			params.State = &s
		}
		rsp, err := api.ListTicketsWithResponse(context.Background(), projectID, params)
		if err != nil {
			return ticketsMsg{err: err.Error()}
		}
		if rsp.StatusCode() != 200 {
			return ticketsMsg{err: fmt.Sprintf("HTTP %d", rsp.StatusCode())}
		}
		var tickets []client.Ticket
		if err := json.Unmarshal(rsp.Body, &tickets); err != nil {
			return ticketsMsg{err: err.Error()}
		}
		return ticketsMsg{tickets: tickets}
	}
}

func loadWorkStreams(api *client.ClientWithResponses, projectID string, statusFilter string) tea.Cmd {
	return func() tea.Msg {
		var status client.ListWorkStreamsParamsStatus
		switch statusFilter {
		case "closed":
			status = client.ListWorkStreamsParamsStatusClosed
		case "all":
			status = client.ListWorkStreamsParamsStatusAll
		default:
			status = client.ListWorkStreamsParamsStatusActive
		}
		params := &client.ListWorkStreamsParams{Status: &status}
		rsp, err := api.ListWorkStreamsWithResponse(context.Background(), projectID, params)
		if err != nil {
			return workStreamsMsg{err: err.Error()}
		}
		if rsp.StatusCode() != 200 {
			return workStreamsMsg{err: fmt.Sprintf("HTTP %d", rsp.StatusCode())}
		}
		var ws []client.WorkStream
		if rsp.JSON200 != nil {
			ws = *rsp.JSON200
		}
		return workStreamsMsg{workStreams: ws}
	}
}

func loadWorkStreamsForList(api *client.ClientWithResponses, projectID string, statusFilter string) tea.Cmd {
	return func() tea.Msg {
		var status client.ListWorkStreamsParamsStatus
		switch statusFilter {
		case "closed":
			status = client.ListWorkStreamsParamsStatusClosed
		case "all":
			status = client.ListWorkStreamsParamsStatusAll
		default:
			status = client.ListWorkStreamsParamsStatusActive
		}
		params := &client.ListWorkStreamsParams{Status: &status}
		rsp, err := api.ListWorkStreamsWithResponse(context.Background(), projectID, params)
		if err != nil {
			return workStreamsListMsg{err: err.Error()}
		}
		if rsp.StatusCode() != 200 {
			return workStreamsListMsg{err: fmt.Sprintf("HTTP %d", rsp.StatusCode())}
		}
		var ws []client.WorkStream
		if rsp.JSON200 != nil {
			ws = *rsp.JSON200
		}
		return workStreamsListMsg{workStreams: ws}
	}
}

func updateWorkStream(api *client.ClientWithResponses, projectID, workStreamID, status string) tea.Cmd {
	return func() tea.Msg {
		s := client.UpdateWorkStreamRequestStatus(status)
		body := client.UpdateWorkStreamJSONRequestBody{Status: &s}
		rsp, err := api.UpdateWorkStreamWithResponse(context.Background(), projectID, workStreamID, body)
		if err != nil {
			return workStreamUpdatedMsg{err: err.Error()}
		}
		if rsp.StatusCode() >= 400 {
			return workStreamUpdatedMsg{err: fmt.Sprintf("HTTP %d", rsp.StatusCode())}
		}
		return workStreamUpdatedMsg{}
	}
}

type workStreamUpdatedMsg struct {
	err string
}

func loadPendingReviews(api *client.ClientWithResponses, projectID string) tea.Cmd {
	return func() tea.Msg {
		rsp, err := api.ListPendingReviewsWithResponse(context.Background(), projectID)
		if err != nil {
			return reviewsMsg{err: err.Error()}
		}
		if rsp.JSON200 == nil {
			return reviewsMsg{err: "no reviews response"}
		}
		tickets := rsp.JSON200.Tickets
		if tickets == nil {
			return reviewsMsg{tickets: []client.Ticket{}}
		}
		return reviewsMsg{tickets: *tickets}
	}
}

func projectSlug(m model) string {
	for _, p := range m.projects {
		if p.Id != nil && *p.Id == m.projectID && p.Slug != nil {
			return *p.Slug
		}
	}
	return ""
}

// resolveRepoPath returns the repo path for git notes. Priority:
// 1. FLYWHEEL_REPO_PATH (global)
// 2. FLYWHEEL_REPO_PATH_<slug> (per-project; slug with - replaced by _)
// 3. cwd if it is a git repo
func resolveRepoPath(projectSlug string) string {
	if p := os.Getenv("FLYWHEEL_REPO_PATH"); p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
				return abs
			}
		}
	}
	if projectSlug != "" {
		envKey := "FLYWHEEL_REPO_PATH_" + strings.ReplaceAll(strings.ToUpper(projectSlug), "-", "_")
		if p := os.Getenv(envKey); p != "" {
			if abs, err := filepath.Abs(p); err == nil {
				if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
					return abs
				}
			}
		}
	}
	if wd, err := os.Getwd(); err == nil {
		if abs, err := filepath.Abs(wd); err == nil {
			if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
				return abs
			}
		}
	}
	return ""
}

func loadGitNotesLog(api *client.ClientWithResponses, orgID, projectID, repoPath, noteType string, limit int) tea.Cmd {
	return func() tea.Msg {
		if repoPath == "" {
			return gitNotesLogMsg{err: "no repo path (set FLYWHEEL_REPO_PATH or run from project root)"}
		}
		params := &client.GetGitNotesLogParams{RepoPath: repoPath, Limit: ptrInt(limit)}
		if noteType != "" {
			t := client.GetGitNotesLogParamsType(noteType)
			params.Type = &t
		} else {
			t := client.GetGitNotesLogParamsTypeDecision
			params.Type = &t
		}
		rsp, err := api.GetGitNotesLogWithResponse(context.Background(), orgID, projectID, params)
		if err != nil {
			return gitNotesLogMsg{err: err.Error()}
		}
		if rsp.StatusCode() == 501 {
			// Fall back to local flywheel-git
			entries, localErr := loadGitNotesLogLocal(repoPath, noteType, limit)
			if localErr != "" {
				return gitNotesLogMsg{err: "server has no repo (501). " + localErr}
			}
			return gitNotesLogMsg{entries: entries}
		}
		if rsp.StatusCode() != 200 {
			return gitNotesLogMsg{err: fmt.Sprintf("HTTP %d", rsp.StatusCode())}
		}
		if rsp.JSON200 == nil || rsp.JSON200.Entries == nil {
			return gitNotesLogMsg{entries: []client.GitNotesLogEntry{}}
		}
		return gitNotesLogMsg{entries: *rsp.JSON200.Entries}
	}
}

// loadGitNotesLogLocal runs flywheel-git note log and parses output. Returns entries or error string.
func loadGitNotesLogLocal(repoPath, noteType string, limit int) ([]client.GitNotesLogEntry, string) {
	if noteType == "" {
		noteType = "decision"
	}
	args := []string{"note", "log", "-t", noteType, "-n", fmt.Sprintf("%d", limit)}
	cmd := exec.Command("flywheel-git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return nil, "flywheel-git: " + strings.TrimSpace(string(out))
		}
		return nil, "flywheel-git not found or failed. Install with: make build-flywheel-git"
	}
	// Format: "commitSHA\nbody\n---\n" per entry
	ref := "refs/notes/flywheel/" + noteType
	parts := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n---\n")
	var entries []client.GitNotesLogEntry
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lines := strings.SplitN(p, "\n", 2)
		if len(lines) < 2 {
			continue
		}
		commitSHA := strings.TrimSpace(lines[0])
		body := strings.TrimSpace(lines[1])
		entries = append(entries, client.GitNotesLogEntry{
			CommitSha: &commitSHA,
			Ref:       &ref,
			Body:      &body,
		})
	}
	return entries, ""
}

func ptrInt(i int) *int { return &i }

func parseGitNoteDetail(commitSHA, body string) *gitNoteDetail {
	d := &gitNoteDetail{CommitSHA: commitSHA, Body: body}
	var v struct {
		V         int    `json:"v"`
		Type      string `json:"type"`
		Message   string `json:"message"`
		AgentID   string `json:"agent_id"`
		TicketID  string `json:"ticket_id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal([]byte(body), &v); err == nil {
		d.Type = v.Type
		d.Message = v.Message
		d.AgentID = v.AgentID
		d.TicketID = v.TicketID
		d.CreatedAt = v.CreatedAt
	}
	return d
}

func loadTrace(api *client.ClientWithResponses, ticketID string) tea.Cmd {
	return func() tea.Msg {
		rsp, err := api.GetTraceWithResponse(context.Background(), ticketID, nil)
		if err != nil {
			return traceMsg{ticketID: ticketID, err: err.Error()}
		}
		if rsp.StatusCode() != 200 {
			return traceMsg{ticketID: ticketID, err: fmt.Sprintf("HTTP %d", rsp.StatusCode())}
		}
		if rsp.JSON200 == nil {
			return traceMsg{ticketID: ticketID}
		}
		return traceMsg{ticketID: ticketID, trace: rsp.JSON200}
	}
}

func submitReview(api *client.ClientWithResponses, ticketID, decision string) tea.Cmd {
	return func() tea.Msg {
		body := client.CreateReviewJSONRequestBody{
			Decision:   client.CreateReviewRequestDecision(decision),
			ReviewerId: ptr("tui"),
		}
		rsp, err := api.CreateReviewWithResponse(context.Background(), ticketID, body)
		if err != nil {
			return reviewDoneMsg{err: err.Error()}
		}
		if rsp.StatusCode() >= 400 {
			return reviewDoneMsg{err: fmt.Sprintf("HTTP %d", rsp.StatusCode())}
		}
		return reviewDoneMsg{decision: decision}
	}
}

func ptr(s string) *string { return &s }

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	if d < 7*24*time.Hour {
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Format("Jan 2")
}

func buildTicketListItems(tickets []client.Ticket, workStreams []client.WorkStream, workStreamFilter string) []list.Item {
	items := make([]list.Item, len(tickets))
	for i, t := range tickets {
		state := ""
		if t.State != nil {
			state = string(*t.State)
		}
		stateStr := state
		if state != "" {
			stateStr = components.StateStyle(state).Render(state)
		}
		title := truncate(str(t.Title), 50)
		wsName := ""
		if workStreamFilter == "" && t.WorkStreamId != nil && *t.WorkStreamId != "" {
			for _, ws := range workStreams {
				if ws.Id != nil && *ws.Id == *t.WorkStreamId && ws.Name != nil {
					wsName = *ws.Name
					break
				}
			}
		}
		relTime := ""
		if t.UpdatedAt != nil {
			relTime = formatRelativeTime(*t.UpdatedAt)
		}
		desc := ""
		if wsName != "" && relTime != "" {
			desc = wsName + " · " + relTime
		} else if wsName != "" {
			desc = wsName
		} else if relTime != "" {
			desc = relTime
		}
		items[i] = ticketItem{
			title: fmt.Sprintf("%s [%s] %s", str(t.Id), stateStr, title),
			desc:  desc,
		}
	}
	return items
}

func formatOutputs(m map[string]interface{}) string {
	if len(m) == 0 {
		return ""
	}
	var lines []string
	if v, ok := m["summary"].(string); ok && v != "" {
		lines = append(lines, "  "+v)
	}
	for k, v := range m {
		if k == "summary" {
			continue
		}
		lines = append(lines, "  "+k+": "+fmt.Sprint(v))
	}
	if len(lines) > 8 {
		lines = lines[:8]
		lines = append(lines, "  ...")
	}
	return strings.Join(lines, "\n") + "\n"
}

// formatOutputsFull is like formatOutputs but shows all keys (no 8-line cap) for rich view.
func formatOutputsFull(m map[string]interface{}) string {
	if len(m) == 0 {
		return ""
	}
	var lines []string
	if v, ok := m["summary"].(string); ok && v != "" {
		lines = append(lines, "  "+v)
	}
	for k, v := range m {
		if k == "summary" {
			continue
		}
		lines = append(lines, "  "+k+": "+fmt.Sprint(v))
	}
	return strings.Join(lines, "\n") + "\n"
}

// maxPayloadLen is the max characters shown for a trace step payload (summary/name/JSON).
const maxPayloadLen = 200

func formatPayloadShort(m map[string]interface{}) string {
	if v, ok := m["name"].(string); ok && v != "" {
		if len(v) > maxPayloadLen {
			return v[:maxPayloadLen-3] + "..."
		}
		return v
	}
	if v, ok := m["summary"].(string); ok && v != "" {
		if len(v) > maxPayloadLen {
			return v[:maxPayloadLen-3] + "..."
		}
		return v
	}
	b, _ := json.Marshal(m)
	s := string(b)
	if len(s) > maxPayloadLen {
		return s[:maxPayloadLen-3] + "..."
	}
	return s
}

// formatTicketBody builds rich ticket content for detail and review views (objective, context, outputs, trace).
// width is the content area width; wrapping uses width-6 for card inner width.
func formatTicketBody(t *client.Ticket, trace *client.ExecutionTrace, forReview bool, width int) string {
	if t == nil {
		return ""
	}
	innerWidth := width - 6
	if innerWidth < 40 {
		innerWidth = 40
	}
	var b strings.Builder
	// Meta
	b.WriteString("  " + components.Primary.Render(str(t.Title)) + "\n")
	if t.Id != nil {
		b.WriteString("  ID: " + *t.Id + "\n")
	}
	var meta []string
	if t.State != nil {
		meta = append(meta, "State: "+string(*t.State))
	}
	if t.Type != nil {
		meta = append(meta, "Type: "+string(*t.Type))
	}
	if t.Priority != nil {
		meta = append(meta, "Priority: "+fmt.Sprint(*t.Priority))
	}
	if len(meta) > 0 {
		b.WriteString("  " + strings.Join(meta, "  ") + "\n")
	}
	if t.AssignedTo != nil && *t.AssignedTo != "" {
		b.WriteString("  Assigned to: " + *t.AssignedTo + "\n")
	}
	if t.CreatedBy != nil && *t.CreatedBy != "" {
		b.WriteString("  Created by: " + *t.CreatedBy + "\n")
	}
	if t.CreatedAt != nil {
		b.WriteString("  Created: " + t.CreatedAt.Format(time.RFC3339) + "\n")
	}
	if t.UpdatedAt != nil {
		b.WriteString("  Updated: " + t.UpdatedAt.Format(time.RFC3339) + "\n")
	}
	if t.DependsOn != nil && len(*t.DependsOn) > 0 {
		b.WriteString("  Depends on: " + strings.Join(*t.DependsOn, ", ") + "\n")
	}
	// Objective
	if t.Objective != nil {
		obj := t.Objective
		if obj.Description != nil && *obj.Description != "" {
			b.WriteString("\n  " + components.Primary.Render("Objective") + "\n")
			b.WriteString(wrapParagraph(*obj.Description, innerWidth, "  "))
			b.WriteString("\n")
		}
		if obj.SuccessCriteria != nil && len(*obj.SuccessCriteria) > 0 {
			b.WriteString("  Success criteria:\n")
			for _, c := range *obj.SuccessCriteria {
				b.WriteString("    • " + c + "\n")
			}
		}
		if obj.AcceptanceTest != nil && *obj.AcceptanceTest != "" {
			label := "  Acceptance test: "
			wrapped := wrapParagraph(*obj.AcceptanceTest, innerWidth-len(label), "  ")
			b.WriteString(label)
			b.WriteString(strings.TrimPrefix(wrapped, "  "))
			b.WriteString("\n")
		}
	}
	// Ticket context
	if t.TicketContext != nil {
		ctx := t.TicketContext
		hasCtx := false
		if ctx.RelevantFiles != nil && len(*ctx.RelevantFiles) > 0 {
			b.WriteString("\n  " + components.Primary.Render("Relevant files") + "\n")
			for _, f := range *ctx.RelevantFiles {
				b.WriteString("    " + f + "\n")
			}
			hasCtx = true
		}
		if ctx.Constraints != nil && len(*ctx.Constraints) > 0 {
			b.WriteString("\n  " + components.Primary.Render("Constraints") + "\n")
			for _, c := range *ctx.Constraints {
				b.WriteString("    • " + c + "\n")
			}
			hasCtx = true
		}
		if ctx.HumanAnswers != nil && len(*ctx.HumanAnswers) > 0 {
			b.WriteString("\n  " + components.Primary.Render("Human answers") + "\n")
			for i, a := range *ctx.HumanAnswers {
				b.WriteString("    " + fmt.Sprint(i+1) + ". " + a + "\n")
			}
			hasCtx = true
		}
		if ctx.PriorAttempts != nil && len(*ctx.PriorAttempts) > 0 {
			b.WriteString("\n  " + components.Primary.Render("Prior attempts") + "\n")
			for i, pa := range *ctx.PriorAttempts {
				b.WriteString("    Attempt " + fmt.Sprint(i+1) + ": ")
				b.WriteString(formatPayloadShort(pa) + "\n")
			}
			hasCtx = true
		}
		if hasCtx {
			b.WriteString("\n")
		}
	}
	// Inputs
	if t.Inputs != nil && len(*t.Inputs) > 0 {
		b.WriteString("\n  " + components.Primary.Render("Inputs") + "\n")
		b.WriteString(formatOutputs(*t.Inputs))
	}
	// Outputs (from agent)
	b.WriteString("\n  " + components.Primary.Render("Outputs (from agent)") + "\n")
	if t.Outputs != nil && len(*t.Outputs) > 0 {
		raw := formatOutputsFull(*t.Outputs)
		b.WriteString(wrapOutputsLines(raw, innerWidth))
		b.WriteString("\n")
	} else {
		if forReview {
			b.WriteString("  (none — agent should call submit_ticket with outputs)\n")
		} else {
			b.WriteString("  (none)\n")
		}
	}
	// Execution trace
	b.WriteString("\n  " + components.Primary.Render("Execution trace") + "\n")
	if trace != nil && trace.AgentId != nil && *trace.AgentId != "" {
		b.WriteString("  Agent: " + *trace.AgentId + "\n")
	}
	if trace != nil && trace.Steps != nil && len(*trace.Steps) > 0 {
		for _, s := range *trace.Steps {
			typ := "?"
			if s.Type != nil {
				typ = string(*s.Type)
			}
			b.WriteString("  - " + typ)
			if s.CreatedAt != nil {
				b.WriteString(" " + components.Muted.Render(s.CreatedAt.Format("15:04:05")))
			}
			if s.Payload != nil && len(*s.Payload) > 0 {
				b.WriteString("\n    " + formatPayloadShort(*s.Payload))
			}
			b.WriteString("\n")
		}
	} else {
		if forReview {
			b.WriteString("  (no steps — agent should use log_step while working)\n")
		} else {
			b.WriteString("  (no steps)\n")
		}
	}
	return b.String()
}

// wrapParagraph wraps s at word boundaries so each line is at most lineWidth runes; every line (including continuations) is prefixed with indent for consistent alignment.
func wrapParagraph(s string, lineWidth int, indent string) string {
	if lineWidth <= len(indent) {
		return s
	}
	contentWidth := lineWidth - len(indent)
	words := strings.Fields(s)
	if len(words) == 0 {
		return indent
	}
	var out strings.Builder
	out.WriteString(indent)
	lineLen := 0
	for _, w := range words {
		if lineLen > 0 && lineLen+1+len(w) > contentWidth {
			out.WriteString("\n" + indent)
			lineLen = 0
		}
		if lineLen > 0 {
			out.WriteString(" ")
			lineLen++
		}
		out.WriteString(w)
		lineLen += len(w)
	}
	return out.String()
}

// wrapOutputsLines wraps each line of s; lines are assumed to start with "  ", which is stripped before wrapping so continuations get the same "  " indent.
func wrapOutputsLines(s string, lineWidth int) string {
	lines := strings.Split(s, "\n")
	var out strings.Builder
	for i, line := range lines {
		if i > 0 {
			out.WriteString("\n")
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		rest := strings.TrimPrefix(line, "  ")
		if len(rest) < len(line) {
			out.WriteString(wrapParagraph(rest, lineWidth, "  "))
		} else {
			out.WriteString(wrapParagraph(line, lineWidth, "  "))
		}
	}
	return out.String()
}

package ui

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	log "github.com/sirupsen/logrus"
	"github.com/xeonx/timeago"

	"github.com/helvethink/gitlab-ci-exporter/pkg/monitor/client"
	pb "github.com/helvethink/gitlab-ci-exporter/pkg/monitor/protobuf"
)

// tab represents the type for tab identifiers.
type tab string

const (
	tabTelemetry tab = "telemetry" // Tab identifier for telemetry view
	tabConfig    tab = "config"    // Tab identifier for configuration view
)

var tabs = [...]tab{
	tabTelemetry,
	tabConfig,
}

// Styling variables for UI elements
var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}

	dataStyle = lipgloss.NewStyle().
			MarginLeft(1).
			MarginRight(5).
			Padding(0, 1).
			Bold(true).
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#a9a9a9"))

	// Tab styling
	activeTabBorder = lipgloss.Border{
		Top: "─", Bottom: " ", Left: "│", Right: "│",
		TopLeft: "╭", TopRight: "╮", BottomLeft: "┘", BottomRight: "└",
	}

	tabBorder = lipgloss.Border{
		Top: "─", Bottom: "─", Left: "│", Right: "│",
		TopLeft: "╭", TopRight: "╮", BottomLeft: "┴", BottomRight: "┴",
	}

	inactiveTab = lipgloss.NewStyle().
			Border(tabBorder, true).
			BorderForeground(highlight).
			Padding(0, 1)

	activeTab = inactiveTab.Copy().Border(activeTabBorder, true)

	tabGap = inactiveTab.Copy().
		BorderTop(false).
		BorderLeft(false).
		BorderRight(false)

	// List styling
	entityStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(subtle)

	// Status Bar styling
	statusStyle = lipgloss.NewStyle().
			Inherit(statusBarStyle).
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#003d80")).
			Padding(0, 1).
			MarginRight(1)

	statusNugget = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#343433", Dark: "#C1C6B2"}).
			Background(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#353533"})

	statusText = lipgloss.NewStyle().Inherit(statusBarStyle)

	versionStyle = statusNugget.Copy().
			Background(lipgloss.Color("#0062cc"))

	// Page styling
	docStyle = lipgloss.NewStyle()
)

// model represents the application model for the UI.
type model struct {
	version         string
	client          *client.Client
	vp              viewport.Model
	progress        *progress.Model
	telemetry       *pb.Telemetry
	telemetryStream chan *pb.Telemetry
	tabID           int
}

// renderConfigViewport renders the configuration viewport content.
func (m *model) renderConfigViewport() string {
	config, err := m.client.GetConfig(context.TODO(), &pb.Empty{})
	if err != nil || config == nil {
		log.WithError(err).Fatal()
	}

	return config.GetContent()
}

// renderTelemetryViewport renders the telemetry viewport content.
func (m *model) renderTelemetryViewport() string {
	if m.telemetry == nil {
		return "\nloading data.."
	}

	gitlabAPIUsage := lipgloss.JoinHorizontal(
		lipgloss.Top,
		" GitLab API usage        ",
		m.progress.ViewAs(m.telemetry.GitlabApiUsage),
		"\n",
	)

	gitlabAPIRequestsCount := lipgloss.JoinHorizontal(
		lipgloss.Top,
		" GitLab API requests    ",
		dataStyle.SetString(strconv.Itoa(int(m.telemetry.GetGitlabApiRequestsCount()))).String(),
		"\n",
	)

	gitlabAPIRateLimit := lipgloss.JoinHorizontal(
		lipgloss.Top,
		" GitLab API limit usage  ",
		m.progress.ViewAs(m.telemetry.GetGitlabApiRateLimit()),
		"\n",
	)

	gitlabAPIRateLimitRemaining := lipgloss.JoinHorizontal(
		lipgloss.Top,
		" GitLab API limit requests remaining ",
		dataStyle.SetString(strconv.Itoa(int(m.telemetry.GetGitlabApiLimitRemaining()))).String(),
		"\n",
	)

	tasksBufferUsage := lipgloss.JoinHorizontal(
		lipgloss.Top,
		" Tasks buffer usage      ",
		m.progress.ViewAs(m.telemetry.GetTasksBufferUsage()),
		"\n",
	)

	tasksExecuted := lipgloss.JoinHorizontal(
		lipgloss.Top,
		" Tasks executed         ",
		dataStyle.SetString(strconv.Itoa(int(m.telemetry.GetTasksExecutedCount()))).String(),
		"\n",
	)

	return strings.Join([]string{
		"",
		gitlabAPIUsage,
		gitlabAPIRequestsCount,
		gitlabAPIRateLimit,
		gitlabAPIRateLimitRemaining,
		tasksBufferUsage,
		tasksExecuted,
		renderEntity("Projects", m.telemetry.GetProjects()),
		renderEntity("Environments", m.telemetry.GetEnvs()),
		renderEntity("Refs", m.telemetry.GetRefs()),
		renderEntity("Metrics", m.telemetry.GetMetrics()),
	}, "\n")
}

// renderEntity renders an entity with its details.
func renderEntity(name string, e *pb.Entity) string {
	return entityStyle.Render(lipgloss.JoinHorizontal(
		lipgloss.Top,
		" "+name+strings.Repeat(" ", 24-len(name)),
		lipgloss.JoinVertical(
			lipgloss.Left,
			"Total      "+dataStyle.SetString(strconv.Itoa(int(e.Count))).String()+"\n",
			"Last Pull  "+dataStyle.SetString(prettyTimeago(e.LastPull.AsTime())).String()+"\n",
			"Last GC    "+dataStyle.SetString(prettyTimeago(e.LastGc.AsTime())).String()+"\n",
			"Next Pull  "+dataStyle.SetString(prettyTimeago(e.NextPull.AsTime())).String()+"\n",
			"Next GC    "+dataStyle.SetString(prettyTimeago(e.NextGc.AsTime())).String()+"\n",
		),
		"\n",
	))
}

// prettyTimeago formats a time into a human-readable string.
func prettyTimeago(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}

	return timeago.English.Format(t)
}

// newModel initializes a new model instance.
func newModel(version string, endpoint *url.URL) (m *model) {
	p := progress.NewModel(progress.WithScaledGradient("#80c904", "#ff9d5c"))

	m = &model{
		version:         version,
		vp:              viewport.Model{},
		telemetryStream: make(chan *pb.Telemetry),
		progress:        &p,
		client:          client.NewClient(context.TODO(), endpoint),
	}

	return
}

// Init initializes the model and returns a command to fetch telemetry data.
func (m *model) Init() tea.Cmd {
	return tea.Batch(
		m.streamTelemetry(context.TODO()),
		waitForTelemetryUpdate(m.telemetryStream),
	)
}

// Update handles messages and updates the model accordingly.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 4
		m.progress.Width = msg.Width - 27
		m.setPaneContent()

		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyLeft:
			if m.tabID > 0 {
				m.tabID--
				m.setPaneContent()
			}
			return m, nil
		case tea.KeyRight:
			if m.tabID < len(tabs)-1 {
				m.tabID++
				m.setPaneContent()
			}
			return m, nil
		case tea.KeyUp, tea.KeyDown, tea.KeyPgDown, tea.KeyPgUp:
			vp, cmd := m.vp.Update(msg)
			m.vp = vp
			return m, cmd
		}
	case *pb.Telemetry:
		m.telemetry = msg
		m.setPaneContent()
		return m, waitForTelemetryUpdate(m.telemetryStream)
	}

	return m, nil
}

// View renders the UI view.
func (m *model) View() string {
	doc := strings.Builder{}

	// Render tabs
	{
		renderedTabs := []string{}
		for tabID, t := range tabs {
			if m.tabID == tabID {
				renderedTabs = append(renderedTabs, activeTab.Render(string(t)))
				continue
			}
			renderedTabs = append(renderedTabs, inactiveTab.Render(string(t)))
		}

		row := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
		gap := tabGap.Render(strings.Repeat(" ", max(0, m.vp.Width-lipgloss.Width(row))))
		row = lipgloss.JoinHorizontal(lipgloss.Bottom, row, gap)
		doc.WriteString(row + "\n")
	}

	// Render pane
	{
		doc.WriteString(m.vp.View() + "\n")
	}

	// Render status bar
	{
		bar := lipgloss.JoinHorizontal(lipgloss.Top,
			statusStyle.Render("github.com/helvethink/gitlab-ci-exporter"),
			statusText.Copy().
				Width(max(0, m.vp.Width-(55+len(m.version)))).
				Render(""),
			versionStyle.Render(m.version),
		)

		doc.WriteString(statusBarStyle.Width(m.vp.Width).Render(bar))
	}

	return docStyle.Render(doc.String())
}

// streamTelemetry streams telemetry data from the client.
func (m *model) streamTelemetry(ctx context.Context) tea.Cmd {
	c, err := m.client.GetTelemetry(ctx, &pb.Empty{})
	if err != nil {
		log.WithError(err).Fatal()
	}

	go func(m *model) {
		for {
			telemetry, err := c.Recv()
			if err != nil {
				log.WithError(err).Fatal()
			}

			m.telemetryStream <- telemetry
		}
	}(m)

	return nil
}

// waitForTelemetryUpdate waits for a telemetry update and returns a command.
func waitForTelemetryUpdate(t chan *pb.Telemetry) tea.Cmd {
	return func() tea.Msg {
		return <-t
	}
}

// Start initializes and starts the UI program.
func Start(version string, listenerAddress *url.URL) {
	if err := tea.NewProgram(
		newModel(version, listenerAddress),
		tea.WithAltScreen(),
	).Start(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}

// setPaneContent sets the content of the viewport pane based on the current tab.
func (m *model) setPaneContent() {
	switch tabs[m.tabID] {
	case tabTelemetry:
		m.vp.SetContent(m.renderTelemetryViewport())
	case tabConfig:
		m.vp.SetContent(m.renderConfigViewport())
	}
}

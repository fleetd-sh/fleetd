package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

type ServiceStatus struct {
	Name    string
	Status  string // running, stopped, error, pending
	Uptime  time.Duration
	Message string
}

type LogEntry struct {
	Time    time.Time
	Level   string // info, success, warning, error
	Service string
	Message string
}

type DashboardModel struct {
	services    []ServiceStatus
	logs        []LogEntry
	width       int
	height      int
	ready       bool
	quitting    bool
	spring      harmonica.Spring
	scrollY     float64
	targetY     float64
	zone        *zone.Manager
	focusedPane string // "services" or "logs"
}

type tickMsg time.Time
type logMsg LogEntry
type serviceUpdateMsg ServiceStatus

func NewDashboard() DashboardModel {
	return DashboardModel{
		services:    []ServiceStatus{},
		logs:        []LogEntry{},
		spring:      harmonica.NewSpring(harmonica.FPS(60), 10.0, 0.8),
		zone:        zone.New(),
		focusedPane: "services",
	}
}

func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		tea.EnterAltScreen,
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "tab":
			if m.focusedPane == "services" {
				m.focusedPane = "logs"
			} else {
				m.focusedPane = "services"
			}
		case "up", "k":
			m.targetY -= 1
		case "down", "j":
			m.targetY += 1
		case "r":
			// Reload services
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

	case tea.MouseMsg:
		// Handle mouse clicks for interactive zones
		if msg.Action == tea.MouseActionPress {
			// Check if clicked on service cards
			for i := range m.services {
				zoneID := fmt.Sprintf("service-%d", i)
				if m.zone.Get(zoneID).InBounds(msg) {
					// Handle service click
				}
			}
		}

	case tickMsg:
		// Update spring animation
		m.scrollY, _ = m.spring.Update(m.scrollY, 0, m.targetY)
		return m, tickCmd()

	case logMsg:
		m.logs = append(m.logs, LogEntry(msg))
		if len(m.logs) > 100 {
			m.logs = m.logs[1:]
		}

	case serviceUpdateMsg:
		// Update service status
		for i, svc := range m.services {
			if svc.Name == msg.Name {
				m.services[i] = ServiceStatus(msg)
				break
			}
		}
	}

	return m, nil
}

func (m DashboardModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	if m.quitting {
		return "Shutting down...\n"
	}

	var sections []string

	// Header
	header := TitleStyle.Render("fleetd Dashboard")
	sections = append(sections, header)

	// Services section
	servicesView := m.renderServices()
	sections = append(sections, servicesView)

	// Logs section
	logsView := m.renderLogs()
	sections = append(sections, logsView)

	// Help
	help := HelpStyle.Render("q: quit • tab: switch pane • ↑/↓: scroll • r: reload")
	sections = append(sections, help)

	view := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return m.zone.Scan(view)
}

func (m DashboardModel) renderServices() string {
	if len(m.services) == 0 {
		return CardStyle.Render("No services running")
	}

	var serviceCards []string
	for i, svc := range m.services {
		card := m.renderServiceCard(svc, i)
		serviceCards = append(serviceCards, card)
	}

	// Arrange in grid (2 columns)
	var rows []string
	for i := 0; i < len(serviceCards); i += 2 {
		if i+1 < len(serviceCards) {
			row := lipgloss.JoinHorizontal(lipgloss.Top, serviceCards[i], serviceCards[i+1])
			rows = append(rows, row)
		} else {
			rows = append(rows, serviceCards[i])
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m DashboardModel) renderServiceCard(svc ServiceStatus, index int) string {
	zoneID := fmt.Sprintf("service-%d", index)

	var statusBadge string
	switch svc.Status {
	case "running":
		statusBadge = StatusRunning.Render(" RUNNING ")
	case "stopped":
		statusBadge = StatusStopped.Render(" STOPPED ")
	case "error":
		statusBadge = StatusError.Render(" ERROR ")
	case "pending":
		statusBadge = StatusPending.Render(" PENDING ")
	}

	title := lipgloss.NewStyle().Bold(true).Render(svc.Name)
	uptime := fmt.Sprintf("Uptime: %s", svc.Uptime.Round(time.Second))
	message := svc.Message

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		statusBadge,
		uptime,
		lipgloss.NewStyle().Foreground(muted).Render(message),
	)

	card := CardStyle.Width(m.width/2 - 4).Render(content)
	return m.zone.Mark(zoneID, card)
}

func (m DashboardModel) renderLogs() string {
	if len(m.logs) == 0 {
		return LogStyle.Width(m.width - 4).Render("No logs yet...")
	}

	var logLines []string
	start := len(m.logs) - 10
	if start < 0 {
		start = 0
	}

	for _, log := range m.logs[start:] {
		line := m.renderLogLine(log)
		logLines = append(logLines, line)
	}

	content := strings.Join(logLines, "\n")
	title := lipgloss.NewStyle().Bold(true).Foreground(secondary).Render("Logs")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		LogStyle.Width(m.width-4).Render(content),
	)
}

func (m DashboardModel) renderLogLine(log LogEntry) string {
	timestamp := log.Time.Format("15:04:05")
	service := lipgloss.NewStyle().Foreground(muted).Render(fmt.Sprintf("[%s]", log.Service))

	var levelStyle lipgloss.Style
	var level string

	switch log.Level {
	case "info":
		levelStyle = LogInfoStyle
		level = "INFO "
	case "success":
		levelStyle = LogSuccessStyle
		level = "OK   "
	case "warning":
		levelStyle = LogWarningStyle
		level = "WARN "
	case "error":
		levelStyle = LogErrorStyle
		level = "ERROR"
	}

	levelBadge := levelStyle.Render(level)

	return fmt.Sprintf("%s %s %s %s",
		lipgloss.NewStyle().Foreground(muted).Render(timestamp),
		levelBadge,
		service,
		log.Message,
	)
}

// Helper functions to send messages to the model

func AddLog(level, service, message string) tea.Cmd {
	return func() tea.Msg {
		return logMsg{
			Time:    time.Now(),
			Level:   level,
			Service: service,
			Message: message,
		}
	}
}

func UpdateService(name, status, message string, uptime time.Duration) tea.Cmd {
	return func() tea.Msg {
		return serviceUpdateMsg{
			Name:    name,
			Status:  status,
			Uptime:  uptime,
			Message: message,
		}
	}
}

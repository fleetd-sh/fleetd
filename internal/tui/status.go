package tui

import (
	"fmt"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// StatusModel shows live status updates during operations
type StatusModel struct {
	tasks   []Task
	mu      sync.Mutex
	width   int
	height  int
	program *tea.Program
}

type Task struct {
	ID      string
	Name    string
	Status  string // pending, running, success, error
	Message string
	Started time.Time
	mu      sync.Mutex
}

type taskUpdateMsg struct {
	id      string
	status  string
	message string
}

func NewStatus() *StatusModel {
	return &StatusModel{
		tasks: make([]Task, 0),
	}
}

// Start begins the TUI if in a TTY, otherwise does nothing
func (s *StatusModel) Start() {
	if !isatty() {
		return
	}

	s.program = tea.NewProgram(s, tea.WithAltScreen())
	go s.program.Run()
	time.Sleep(time.Millisecond * 50) // Give TUI time to start
}

// Stop ends the TUI
func (s *StatusModel) Stop() {
	if s.program != nil {
		s.program.Quit()
	}
}

// AddTask adds a new task to track
func (s *StatusModel) AddTask(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasks = append(s.tasks, Task{
		ID:      id,
		Name:    name,
		Status:  "pending",
		Started: time.Now(),
	})

	if s.program != nil {
		s.program.Send(taskUpdateMsg{id: id, status: "pending", message: ""})
	} else {
		fmt.Printf("⏳ %s\n", name)
	}
}

// UpdateTask updates a task's status
func (s *StatusModel) UpdateTask(id, status, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.tasks {
		if s.tasks[i].ID == id {
			s.tasks[i].Status = status
			s.tasks[i].Message = message
			break
		}
	}

	if s.program != nil {
		s.program.Send(taskUpdateMsg{id: id, status: status, message: message})
	} else {
		s.printPlain(status, message)
	}
}

func (s *StatusModel) printPlain(status, message string) {
	switch status {
	case "success":
		fmt.Printf("✓ %s\n", message)
	case "error":
		fmt.Printf("✗ %s\n", message)
	case "running":
		fmt.Printf("→ %s\n", message)
	default:
		fmt.Printf("  %s\n", message)
	}
}

// Bubbletea interface
func (s *StatusModel) Init() tea.Cmd {
	return nil
}

func (s *StatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return s, tea.Quit
		}

	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height

	case taskUpdateMsg:
		s.mu.Lock()
		for i := range s.tasks {
			if s.tasks[i].ID == msg.id {
				s.tasks[i].Status = msg.status
				s.tasks[i].Message = msg.message
				break
			}
		}
		s.mu.Unlock()
	}

	return s, nil
}

func (s *StatusModel) View() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.tasks) == 0 {
		return ""
	}

	var sections []string

	// Header
	header := TitleStyle.Render("fleetd")
	sections = append(sections, header)

	// Tasks
	for _, task := range s.tasks {
		taskView := s.renderTask(task)
		sections = append(sections, taskView)
	}

	// Help
	help := HelpStyle.Render("ctrl+c: cancel")
	sections = append(sections, "\n"+help)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (s *StatusModel) renderTask(task Task) string {
	var icon, statusText string
	var style lipgloss.Style

	switch task.Status {
	case "pending":
		icon = "⏳"
		statusText = "PENDING"
		style = StatusPending
	case "running":
		icon = spinnerFrames[int(time.Since(task.Started).Milliseconds()/80)%len(spinnerFrames)]
		statusText = "RUNNING"
		style = lipgloss.NewStyle().Foreground(secondary)
	case "success":
		icon = "✓"
		statusText = "SUCCESS"
		style = StatusRunning
	case "error":
		icon = "✗"
		statusText = "ERROR"
		style = StatusError
	}

	badge := style.Render(" " + statusText + " ")
	name := lipgloss.NewStyle().Bold(true).Render(task.Name)

	line := fmt.Sprintf("%s %s %s", icon, badge, name)

	if task.Message != "" {
		message := lipgloss.NewStyle().Foreground(muted).Render(task.Message)
		line += "\n  " + message
	}

	return line
}

func isatty() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Singleton for global access
var globalStatus *StatusModel
var once sync.Once

func GetStatus() *StatusModel {
	once.Do(func() {
		globalStatus = NewStatus()
	})
	return globalStatus
}

// Helper functions for common patterns
func Info(message string) {
	if status := GetStatus(); status.program != nil {
		// TUI is active, add as task
		id := fmt.Sprintf("info-%d", time.Now().UnixNano())
		status.AddTask(id, message)
		status.UpdateTask(id, "success", "")
	} else {
		fmt.Printf("→ %s\n", message)
	}
}

func Success(message string) {
	if status := GetStatus(); status.program != nil {
		id := fmt.Sprintf("success-%d", time.Now().UnixNano())
		status.AddTask(id, message)
		status.UpdateTask(id, "success", "")
	} else {
		fmt.Printf("✓ %s\n", message)
	}
}

func Error(message string) {
	if status := GetStatus(); status.program != nil {
		id := fmt.Sprintf("error-%d", time.Now().UnixNano())
		status.AddTask(id, message)
		status.UpdateTask(id, "error", "")
	} else {
		fmt.Printf("✗ %s\n", message)
	}
}

func Warning(message string) {
	if status := GetStatus(); status.program != nil {
		id := fmt.Sprintf("warning-%d", time.Now().UnixNano())
		status.AddTask(id, message)
		status.UpdateTask(id, "running", message)
	} else {
		fmt.Printf("⚠ %s\n", message)
	}
}

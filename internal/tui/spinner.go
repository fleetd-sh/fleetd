package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type SpinnerModel struct {
	frame   int
	message string
	done    bool
	success bool
	err     error
}

type spinnerTickMsg time.Time

func NewSpinner(message string) SpinnerModel {
	return SpinnerModel{
		message: message,
	}
}

func (m SpinnerModel) Init() tea.Cmd {
	return spinnerTick()
}

func spinnerTick() tea.Cmd {
	return tea.Tick(time.Millisecond*80, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

func (m SpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case spinnerTickMsg:
		if !m.done {
			m.frame = (m.frame + 1) % len(spinnerFrames)
			return m, spinnerTick()
		}
	}

	return m, nil
}

func (m SpinnerModel) View() string {
	if m.done {
		if m.err != nil {
			icon := StatusError.Render(" ✗ ")
			return icon + " " + m.message + ": " + m.err.Error() + "\n"
		}
		if m.success {
			icon := StatusRunning.Render(" ✓ ")
			return icon + " " + m.message + "\n"
		}
	}

	spinner := lipgloss.NewStyle().Foreground(primary).Render(spinnerFrames[m.frame])
	return spinner + " " + m.message + "\n"
}

func (m *SpinnerModel) Complete(success bool, err error) {
	m.done = true
	m.success = success
	m.err = err
}

func (m *SpinnerModel) UpdateMessage(message string) {
	m.message = message
}

// Package tui implements the interactive channel picker / push-to-talk
// terminal UI for net-ptt.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kdudkov/net-ptt/internal/client"
	"github.com/kdudkov/net-ptt/internal/config"
)

type rxCheckMsg struct{}

// Model is the bubbletea model for the channel picker / PTT screen.
type Model struct {
	client   *client.CommsClient
	channels []config.Channel
	cursor   int

	transmitting       bool
	supportKeyRelease  bool
	receiving          bool
	statusMsg          string
}

// New builds the initial model, placing the cursor on initialChannel.
func New(c *client.CommsClient, channels []config.Channel, initialChannel int) Model {
	cursor := 0
	for i, ch := range channels {
		if ch.Number == initialChannel {
			cursor = i
			break
		}
	}
	return Model{client: c, channels: channels, cursor: cursor}
}

func (m Model) Init() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return rxCheckMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyboardEnhancementsMsg:
		m.supportKeyRelease = msg.SupportsEventTypes()

	case tea.KeyPressMsg:
		switch msg.String() {

		case "ctrl+c", "q":
			m.client.StopTransmit()
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.selectChannel()
			}

		case "down", "j":
			if m.cursor < len(m.channels)-1 {
				m.cursor++
				m.selectChannel()
			}

		case "space":
			if m.supportKeyRelease {
				// Hold-to-talk: always (re)start on press. StartTransmit()
				// is idempotent — it checks IsTxEnabled() internally.
				m.transmitting = true
				m.client.StartTransmit()
			} else {
				// No confirmed key-release supportKeyRelease: use press-to-toggle.
				m.transmitting = !m.transmitting
				if m.transmitting {
					m.client.StartTransmit()
				} else {
					m.client.StopTransmit()
				}
			}
		}

	case tea.KeyReleaseMsg:
		if m.supportKeyRelease && msg.String() == "space" && m.transmitting {
			m.transmitting = false
			m.client.StopTransmit()
		}

	case rxCheckMsg:
		m.receiving = m.client.IsReceiving()
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return rxCheckMsg{}
		})
	}

	return m, nil
}

func (m *Model) selectChannel() {
	ch := m.channels[m.cursor]
	if err := m.client.SwitchChannel(ch.Number); err != nil {
		m.statusMsg = fmt.Sprintf("failed to switch to %s: %v", ch.Name, err)
		return
	}
	m.statusMsg = ""
}

func (m Model) View() tea.View {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Network PTT\n\n"))

	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	for i, ch := range m.channels {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		name := ch.Name
		if m.receiving && i == m.cursor {
			name = greenStyle.Render(name)
		}
		fmt.Fprintf(&b, "%s%s (port %d)\n", prefix, name, ch.Port)
	}

	b.WriteString("\n")

	switch {
	case m.transmitting:
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("160")).Render("[ TRANSMITTING ]"))
	case !m.supportKeyRelease:
		b.WriteString("Press SPACE to start/stop talking (toggle mode)")
	default:
		b.WriteString("Hold SPACE to talk")
	}
	b.WriteString("\n")

	if m.statusMsg != "" {
		fmt.Fprintf(&b, "\n%s\n", m.statusMsg)
	}

	b.WriteString("\nup/down: select channel   space: talk   q/ctrl+c: quit")

	bg := lipgloss.Color("236")
	fg := lipgloss.Color("15")

	st := lipgloss.NewStyle().Width(80).Padding(1).
		Border(lipgloss.RoundedBorder()).BorderForeground(fg).BorderBackground(bg).
		Background(bg).Foreground(fg)

	v := tea.NewView(st.Render(b.String()))

	v.KeyboardEnhancements.ReportEventTypes = true

	return v
}

// Package tui provides the interactive profile picker used by `ccx use`.
package tui

import (
	"errors"
	"fmt"
	"os"

	"github.com/arafa-dev/ccx/internal/contracts"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

// PickProfile prompts the user to select one profile from the list.
// If there is no TTY, it returns the first profile without blocking.
func PickProfile(profiles []contracts.Profile) (contracts.Profile, error) {
	if len(profiles) == 0 {
		return contracts.Profile{}, errors.New("no profiles to pick from")
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return profiles[0], nil
	}
	m := initialModel(profiles)
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return contracts.Profile{}, err
	}
	finalModel, ok := final.(model)
	if !ok || finalModel.chosen < 0 {
		return contracts.Profile{}, errors.New("cancelled")
	}
	return profiles[finalModel.chosen], nil
}

type model struct {
	profiles []contracts.Profile
	cursor   int
	chosen   int
}

func initialModel(profiles []contracts.Profile) model {
	return model{profiles: profiles, chosen: -1}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.cursor
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func (m model) View() string {
	s := titleStyle.Render("Pick a profile:") + "\n\n"
	for i := range m.profiles {
		p := &m.profiles[i]
		line := fmt.Sprintf("  %-12s %s", p.Name, dimStyle.Render(p.ConfigDir))
		if i == m.cursor {
			line = selectedStyle.Render("> " + p.Name + "   " + p.ConfigDir)
		}
		s += line + "\n"
	}
	s += "\n" + dimStyle.Render("up/down select - enter confirm - q cancel")
	return s
}

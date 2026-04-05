package gateway

import (
	"fmt"
	"strings"
)

type Command struct {
	Name    string
	Profile string
	Task    string
}

func ParseCommand(text string) (Command, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return Command{}, false
	}

	fields := strings.Fields(text)
	if len(fields) == 0 {
		return Command{}, false
	}
	name := strings.TrimPrefix(strings.ToLower(fields[0]), "/")
	cmd := Command{Name: name}

	switch name {
	case "profile":
		if len(fields) > 1 {
			cmd.Profile = strings.ToLower(strings.TrimSpace(fields[1]))
		}
		if len(fields) > 2 {
			cmd.Task = strings.TrimSpace(strings.Join(fields[2:], " "))
		}
	case "help", "reset", "memory":
	default:
		cmd.Task = strings.TrimSpace(strings.Join(fields[1:], " "))
	}
	return cmd, true
}

func HandleCommand(cmd Command, session *Session) (summary string, continueTask string, continueProfile string, reset bool, err error) {
	switch cmd.Name {
	case "help":
		return "Commands: /help, /memory, /reset, /profile <auto|software|research|incident|content|assistant> [task]", "", "", false, nil
	case "memory":
		return FormatSession(session, 6), "", "", false, nil
	case "reset":
		return "Session memory cleared. The next message will start a fresh conversation.", "", "", true, nil
	case "profile":
		if !isValidProfile(cmd.Profile) {
			return "", "", "", false, fmt.Errorf("unknown profile %q", cmd.Profile)
		}
		if strings.TrimSpace(cmd.Task) == "" {
			return fmt.Sprintf("Preferred profile set to %s for this chat.", cmd.Profile), "", cmd.Profile, false, nil
		}
		return "", cmd.Task, cmd.Profile, false, nil
	default:
		return "", "", "", false, fmt.Errorf("unknown command %q", cmd.Name)
	}
}

func isValidProfile(profile string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "auto", "software", "research", "incident", "content", "assistant":
		return true
	default:
		return false
	}
}

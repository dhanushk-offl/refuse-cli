package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func init() { register(&codex{}) }

type codex struct{}

func (codex) Name() string { return "codex" }

func (c codex) configPath() (string, error) {
	if p := os.Getenv("CODEX_HOME"); p != "" {
		return filepath.Join(p, "hooks.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "hooks.json"), nil
}

func (c codex) Install(refuseBin string) error {
	p, err := c.configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	settings := map[string]any{}
	if raw, err := os.ReadFile(p); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	pre, _ := hooks["PreToolUse"].([]any)
	pre = withoutRefuseEntries(pre)

	desired := claudeHookGroup{
		Matcher: "Bash",
		Hooks: []claudeHookCmd{{
			Type:    "command",
			Command: fmt.Sprintf("%s gate --agent=codex", refuseBin),
			Timeout: 5000,
		}},
	}
	desiredAny, err := toGenericMap(desired)
	if err != nil {
		return err
	}
	pre = append(pre, desiredAny)

	hooks["PreToolUse"] = pre
	settings["hooks"] = hooks

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(p, append(out, '\n'), 0o600)
}

func (c codex) Remove() error {
	p, err := c.configPath()
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	settings := map[string]any{}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return fmt.Errorf("parse %s: %w", p, err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	pre, _ := hooks["PreToolUse"].([]any)
	stripped := withoutRefuseEntries(pre)
	if len(stripped) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = stripped
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(p, append(out, '\n'), 0o600)
}

func (c codex) Status() (Status, error) {
	st := Status{Agent: c.Name()}
	p, err := c.configPath()
	if err != nil {
		return st, err
	}
	st.ConfigPath = p
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			st.Detail = "hooks.json not found"
			return st, nil
		}
		return st, err
	}
	st.Found = true
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		st.Detail = "hooks.json malformed: " + err.Error()
		return st, nil
	}
	hooks, _ := settings["hooks"].(map[string]any)
	pre, _ := hooks["PreToolUse"].([]any)
	for _, g := range pre {
		if entryHasRefuse(g) {
			st.Installed = true
			break
		}
	}
	if !st.Installed {
		st.Detail = "no refuse hook"
	}
	return st, nil
}

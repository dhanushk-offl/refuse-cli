// Package gate is the decision engine for both shims and agent hooks.
//
// Inputs: a ParseResult from the parsers package and a Client+Policy. Output:
// an exit-code-style Decision plus rendered messages.
package gate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/RefuseHQ/refuse-cli/internal/allowlist"
	"github.com/RefuseHQ/refuse-cli/internal/config"
	"github.com/RefuseHQ/refuse-cli/internal/parsers"
	"github.com/RefuseHQ/refuse-cli/internal/server"
)

// Decision is the gate verdict.
type Decision int

const (
	// DecisionAllow — proceed with the original install.
	DecisionAllow Decision = iota
	// DecisionBlock — abort the install with a reason.
	DecisionBlock
)

// Result is the structured verdict + the rendered human message.
type Result struct {
	Decision     Decision
	Message      string                        // human-readable block reason, empty on allow
	Findings     []server.CheckPackageResponse // per-package result for direct mode
	BatchSummary *server.BatchSummary          // populated for batch / lockfile
	ServerError  error                         // non-nil if the server call failed (fail-open path)
}

// Engine carries the deps the gate needs.
type Engine struct {
	Client    *server.Client
	Policy    config.Policy
	AgentMode bool // true when called from an agent hook; suppresses bypass hints in output
}

// Decide runs the gate against a parser result.
func (e Engine) Decide(ctx context.Context, p parsers.ParseResult) Result {
	if !p.IsInstall || (p.Mode == parsers.ModeDirect && len(p.Packages) == 0) {
		// Not an install verb, or install with no positional packages we
		// can vet (transitive resolution happens on the lockfile side).
		return Result{Decision: DecisionAllow}
	}

	// Override env wins early so users can force a known-bad install through
	// after seeing the block message.
	if e.Policy.OverrideEnv != "" {
		if v, ok := envIfSet(e.Policy.OverrideEnv); ok && truthy(v) {
			return Result{Decision: DecisionAllow}
		}
	}

	if p.Mode == parsers.ModeLockfile {
		// v1: lockfile installs are passed through. The shim layer can hand
		// the lockfile path to `refuse check-lockfile` separately if the
		// user wants. Gating the install itself requires resolving the
		// lockfile against the server, which is its own subcommand.
		return Result{Decision: DecisionAllow}
	}

	pkgs := make([]server.CheckPackageRequest, 0, len(p.Packages))
	for _, ref := range p.Packages {
		pkgs = append(pkgs, server.CheckPackageRequest{
			Ecosystem: ref.Ecosystem,
			Name:      ref.Name,
			Version:   firstNonEmpty(ref.Version, "latest"),
		})
	}
	resp, err := e.Client.CheckBatch(ctx, server.BatchCheckRequest{Packages: pkgs})
	if err != nil {
		return e.failOpenOrClosed(err)
	}

	// Apply the project-local allowlist (`.refuse.yaml`) — strip
	// advisories the team has formally accepted before the gate decides.
	applyAllowlist(p, &resp)

	worst := worstSeverity(resp.Results)
	threshold := config.SeverityRank(e.Policy.SeverityThreshold)
	if worst >= threshold && worst > 0 {
		return Result{
			Decision:     DecisionBlock,
			Message:      renderBlock(resp, e.AgentMode),
			Findings:     resp.Results,
			BatchSummary: &resp.Summary,
		}
	}
	return Result{Decision: DecisionAllow, Findings: resp.Results, BatchSummary: &resp.Summary}
}

func (e Engine) failOpenOrClosed(err error) Result {
	// Rate-limit errors carry structured quota info on the hosted backend.
	// Render those specially in both fail-open and fail-closed modes — the
	// raw "server: rate limited" line is technically correct but useless
	// at the terminal.
	var rate *server.RateLimitedError
	if errors.As(err, &rate) {
		if e.Policy.FailClosed {
			return Result{
				Decision:    DecisionBlock,
				Message:     renderRateLimited(rate, true),
				ServerError: err,
			}
		}
		return Result{
			Decision:    DecisionAllow,
			Message:     renderRateLimited(rate, false),
			ServerError: err,
		}
	}

	if e.Policy.FailClosed {
		return Result{
			Decision:    DecisionBlock,
			Message:     fmt.Sprintf("refuse: %v (fail-closed)", err),
			ServerError: err,
		}
	}
	hint := ""
	if !e.AgentMode {
		if errors.Is(err, server.ErrServerUnreachable) {
			hint = " (set REFUSE_FAIL_CLOSED=1 to require gate)"
		} else if errors.Is(err, server.ErrRateLimited) {
			hint = " — rate limited, allowing install (upgrade plan to raise the limit)"
		} else if errors.Is(err, server.ErrUnauthorized) {
			hint = " — set REFUSE_API_KEY or run `refuse init`"
		}
	}
	return Result{
		Decision:    DecisionAllow,
		Message:     fmt.Sprintf("refuse: %v%s", err, hint),
		ServerError: err,
	}
}

// renderRateLimited formats a rich, multi-line message for a 429 from the
// hosted backend. Includes used/limit, reset window, optional upgrade URL,
// and the action knob (REFUSE_FAIL_CLOSED) so the user can change behavior
// without reading docs.
func renderRateLimited(rate *server.RateLimitedError, blocked bool) string {
	var b strings.Builder
	b.WriteString("refuse: rate limited — account quota ")
	b.WriteString(formatThousands(rate.Used))
	b.WriteString("/")
	b.WriteString(formatThousands(rate.Limit))
	b.WriteString(" used")
	if !rate.PeriodEnd.IsZero() {
		days := int(time.Until(rate.PeriodEnd).Hours() / 24)
		if days < 0 {
			days = 0
		}
		fmt.Fprintf(&b, " (resets %s, %d %s)",
			rate.PeriodEnd.Format("Jan 2"),
			days,
			pluralize(days, "day", "days"))
	}
	if rate.UpgradeURL != "" {
		b.WriteString("\n        upgrade: ")
		b.WriteString(rate.UpgradeURL)
	}
	if blocked {
		b.WriteString("\n        install blocked (REFUSE_FAIL_CLOSED=1)")
	} else {
		b.WriteString("\n        install allowed — set REFUSE_FAIL_CLOSED=1 to block on rate limit")
	}
	return b.String()
}

// formatThousands turns 100000 into "100,000". Avoids pulling in
// golang.org/x/text just for this.
func formatThousands(n int64) string {
	if n < 0 {
		return "-" + formatThousands(-n)
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	// Walk back-to-front inserting commas every 3 digits.
	out := make([]byte, 0, len(s)+(len(s)-1)/3)
	first := len(s) % 3
	if first > 0 {
		out = append(out, s[:first]...)
		if len(s) > first {
			out = append(out, ',')
		}
	}
	for i := first; i < len(s); i += 3 {
		out = append(out, s[i:i+3]...)
		if i+3 < len(s) {
			out = append(out, ',')
		}
	}
	return string(out)
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func renderBlock(resp server.BatchCheckResponse, agentMode bool) string {
	var b strings.Builder
	b.WriteString("refuse: blocked — vulnerable package(s)\n")
	for _, r := range resp.Results {
		if !r.Vulnerable {
			continue
		}
		for _, v := range r.Vulnerabilities {
			label := v.SeverityLabel
			if label == "" {
				label = "unknown"
			}
			id := v.RefuseID
			if v.CVE != nil && *v.CVE != "" {
				id = *v.CVE
			} else if v.GHSA != nil && *v.GHSA != "" {
				id = *v.GHSA
			}
			fmt.Fprintf(&b, "  %s@%s  %s  %s  %s\n", r.Package, r.Version, id, label, truncate(v.Summary, 80))
		}
		if len(r.SuggestedFixes) > 0 {
			f := r.SuggestedFixes[0]
			fmt.Fprintf(&b, "    Suggested: %s (%s)%s\n", f.Version, f.Type, breakingTag(f.BreakingChange))
		}
	}
	// Override hint — hidden from agents so they can't self-bypass
	if !agentMode {
		b.WriteString("\nOverride: set REFUSE_ALLOW_VULNERABLE=1 to bypass.\n")
	}
	return b.String()
}

func breakingTag(b bool) string {
	if b {
		return " ⚠ breaking change"
	}
	return ""
}

func worstSeverity(results []server.CheckPackageResponse) int {
	worst := 0
	for _, r := range results {
		if !r.Vulnerable {
			continue
		}
		for _, v := range r.Vulnerabilities {
			if rank := config.SeverityRank(v.SeverityLabel); rank > worst {
				worst = rank
			}
		}
	}
	return worst
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func truthy(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// applyAllowlist strips advisories that match a project-local
// `.refuse.yaml` allowlist entry. After this runs, the gate's severity
// counting and renderBlock behave as if the allowlisted CVEs simply
// weren't present. A package whose advisories all get stripped has its
// `vulnerable` flag cleared and BatchSummary.Vulnerable decremented.
//
// Best-effort: any error loading `.refuse.yaml` is silently ignored so a
// malformed file can't break the gate. Walk start = cwd (mirrors how
// project tools usually resolve config).
func applyAllowlist(_ parsers.ParseResult, resp *server.BatchCheckResponse) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	path := allowlist.Find(cwd)
	if path == "" {
		return
	}
	f, err := allowlist.Load(path)
	if err != nil || f == nil || len(f.Allowlist) == 0 {
		return
	}
	now := time.Now()
	for i := range resp.Results {
		r := &resp.Results[i]
		if !r.Vulnerable {
			continue
		}
		kept := r.Vulnerabilities[:0]
		for _, v := range r.Vulnerabilities {
			cve := ""
			if v.CVE != nil {
				cve = *v.CVE
			}
			if cve == "" {
				kept = append(kept, v)
				continue
			}
			// Try matching against the package's ecosystem when we have
			// it. Server responses don't carry the eco label per row, so
			// only the package-name leg of the match is enforced.
			if _, ok := f.IsAllowed(now, cve, "", r.Package); ok {
				// Suppressed — fall through (do not append).
				if resp.Summary.BySeverity != nil {
					if rank := config.SeverityRank(v.SeverityLabel); rank > 0 {
						if resp.Summary.BySeverity[v.SeverityLabel] > 0 {
							resp.Summary.BySeverity[v.SeverityLabel]--
						}
					}
				}
				continue
			}
			kept = append(kept, v)
		}
		r.Vulnerabilities = kept
		if len(kept) == 0 {
			r.Vulnerable = false
			if resp.Summary.Vulnerable > 0 {
				resp.Summary.Vulnerable--
			}
		}
	}
}

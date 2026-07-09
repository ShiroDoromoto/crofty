// Package access turns a permission wall into a fork the author chooses.
//
// crofty is usually driven by an AI on the author's behalf. When a bare OS
// error like "Access is denied." comes back, that agent reads it as an obstacle
// to route around, and starts doing things the author never asked for —
// rewriting %APPDATA%, dropping the binary inside the project. It should have
// come back and asked.
//
// So a permission error is not a failure to report: it is a branch that needs
// the author's consent. Denied carries what crofty was trying to do, on which
// path, why it was refused, and the choices — marking which of them need the
// author's permission. The human text and the --json payload are rendered from
// the same value, so the two can never disagree (D-1).
package access

import (
	"errors"
	"io/fs"
	"os"
	"strings"
)

// AgentRule is the norm this package exists to enforce, stated in the one place
// an agent will actually read it: the output of the command that just failed.
// `crofty agent` and the generated AGENTS.md say the same thing.
const AgentRule = "Do not invent a workaround: do not change environment variables, " +
	"move files, or elevate privileges on your own. Show these choices to the author and let them pick."

// Choice is one way past the wall. Permission is what the author would have to
// grant for it — empty when the choice needs nothing from them (crofty can do
// it, or the author only has to type a command).
type Choice struct {
	Do         string // what this choice does, in one line
	Command    string // the exact command to run, when there is one
	Permission string // the permission the author must grant, when there is one
}

// Denied is a permission wall crofty hit.
type Denied struct {
	Op      string // what crofty was trying to do, in the author's terms
	Path    string // the path it could not touch
	Err     error  // the underlying OS error
	Choices []Choice
}

func (d *Denied) Error() string {
	return "cannot " + d.Op + ": " + d.Path + ": " + Reason(d.Err)
}

func (d *Denied) Unwrap() error { return d.Err }

// Deny wraps err as a Denied when it is a permission wall, and returns it
// unchanged otherwise (nil stays nil) — so a call site can wrap every write
// without first asking what kind of error it got. An empty path is filled in
// from the OS error, which usually knows it.
func Deny(op, path string, err error, choices ...Choice) error {
	if err == nil {
		return nil
	}
	if !IsPermission(err) {
		return err
	}
	if path == "" {
		path = pathOf(err)
	}
	return &Denied{Op: op, Path: path, Err: err, Choices: choices}
}

// IsPermission reports whether err is a permission wall. Windows'
// ERROR_ACCESS_DENIED maps onto fs.ErrPermission, so this covers the case that
// started all of this.
func IsPermission(err error) bool { return errors.Is(err, fs.ErrPermission) }

// From returns the Denied behind err: the one Deny built, or — for a permission
// error nobody wrapped — one promoted from what the OS error itself knows. That
// promotion is why no command can leak a bare "Access is denied.": every call
// site is covered, whether or not it has choices to offer yet.
func From(err error) (*Denied, bool) {
	var d *Denied
	if errors.As(err, &d) {
		return d, true
	}
	if !IsPermission(err) {
		return nil, false
	}
	return &Denied{Op: opOf(err), Path: pathOf(err), Err: err}, true
}

// Denials is every wall a command found before it started work. A command that
// looks ahead (init's preflight) can find more than one, and the author should
// be asked for all of them at once: granting one permission, running the command
// again and being asked for the next is the experience crofty is trying to
// avoid. As an error it is one error, so a caller may just return it.
type Denials []*Denied

func (ds Denials) Error() string {
	msgs := make([]string, 0, len(ds))
	for _, d := range ds {
		msgs = append(msgs, d.Error())
	}
	return strings.Join(msgs, "; ")
}

// Unwrap lets errors.Is/As see every wall, so a caller that only knows about one
// Denied still finds one.
func (ds Denials) Unwrap() []error {
	errs := make([]error, 0, len(ds))
	for _, d := range ds {
		errs = append(errs, d)
	}
	return errs
}

// Reason is the OS's own words, without the "open /some/path:" preamble that
// fs.PathError repeats — crofty prints the path on its own line.
func Reason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err.Error()
	}
	return err.Error()
}

// opOf recovers the syscall name ("open", "mkdir", …) for a wall no call site
// described. It is coarser than an author-facing Op, and that is the point: it
// is honest about the fact that nobody wrote a better sentence here yet.
func opOf(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Op
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Op
	}
	return "write"
}

func pathOf(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Path
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.New
	}
	return ""
}

// --- the wire shape -------------------------------------------------------

// Report is the --json rendering of a permission wall, or of several found at
// once. The walls are always a list, so an agent parsing crofty's answer never
// has to handle two shapes; the rule is stated once, where it belongs.
type Report struct {
	Error     string    `json:"error"` // always "permission_denied"
	AgentRule string    `json:"agentRule"`
	Walls     []Payload `json:"walls"`
}

// Payload is one wall on the wire. It exists so the machine-readable output is
// derived from the same value the human text is, and so needsPermission is
// stated outright rather than left for a reader to infer from an empty field.
type Payload struct {
	Op      string          `json:"op"`
	Path    string          `json:"path"`
	Reason  string          `json:"reason"`
	Choices []PayloadChoice `json:"choices"`
}

// PayloadChoice is one Choice on the wire.
type PayloadChoice struct {
	Do              string `json:"do"`
	Command         string `json:"command,omitempty"`
	NeedsPermission bool   `json:"needsPermission"`
	Permission      string `json:"permission,omitempty"`
}

func (d *Denied) Payload() Payload {
	choices := make([]PayloadChoice, 0, len(d.Choices))
	for _, c := range d.Choices {
		choices = append(choices, PayloadChoice{
			Do:              c.Do,
			Command:         c.Command,
			NeedsPermission: c.Permission != "",
			Permission:      c.Permission,
		})
	}
	return Payload{
		Op:      d.Op,
		Path:    d.Path,
		Reason:  Reason(d.Err),
		Choices: choices,
	}
}

// Report renders every wall under one rule.
func (ds Denials) Report() Report {
	walls := make([]Payload, 0, len(ds))
	for _, d := range ds {
		walls = append(walls, d.Payload())
	}
	return Report{Error: "permission_denied", AgentRule: AgentRule, Walls: walls}
}

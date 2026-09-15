// Package cli implements brunerctl, the Brunernetes command line interface.
//
// brunerctl is not a kubectl replacement. It covers the small set of
// operations a single-node factory needs: install the environment, look at the
// factory, deploy and inspect the one application, run a delivery, read events,
// diagnose faults and uninstall.
//
// Every message printed by this package states the real technical condition
// first and adds themed wording afterwards. All output is English only.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// Printer writes the command output. It keeps stdout for results and stderr for
// diagnostics so the commands compose in a shell pipeline.
type Printer struct {
	Out    io.Writer
	Err    io.Writer
	Quiet  bool
	NoWrap bool
}

// NewPrinter returns a printer bound to the process streams.
func NewPrinter() *Printer {
	return &Printer{Out: os.Stdout, Err: os.Stderr}
}

// Printf writes a formatted line to stdout.
func (p *Printer) Printf(format string, args ...any) {
	fmt.Fprintf(p.Out, format+"\n", args...)
}

// Println writes a raw line to stdout.
func (p *Printer) Println(line string) {
	fmt.Fprintln(p.Out, line)
}

// Section writes a section header.
func (p *Printer) Section(title string) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Out, "\n%s\n%s\n", title, strings.Repeat("-", len(title)))
}

// Step reports a step of a multi-step operation.
func (p *Printer) Step(index, total int, description string) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Out, "[%d/%d] %s\n", index, total, description)
}

// OK reports a successful check or action.
func (p *Printer) OK(format string, args ...any) {
	fmt.Fprintf(p.Out, "  OK      %s\n", fmt.Sprintf(format, args...))
}

// Warn reports a non-blocking problem.
func (p *Printer) Warn(format string, args ...any) {
	fmt.Fprintf(p.Out, "  WARN    %s\n", fmt.Sprintf(format, args...))
}

// Fail reports a blocking problem.
func (p *Printer) Fail(format string, args ...any) {
	fmt.Fprintf(p.Out, "  FAIL    %s\n", fmt.Sprintf(format, args...))
}

// Skip reports a check or action that was deliberately not performed.
func (p *Printer) Skip(format string, args ...any) {
	fmt.Fprintf(p.Out, "  SKIP    %s\n", fmt.Sprintf(format, args...))
}

// Info writes an informational line without a status marker.
func (p *Printer) Info(format string, args ...any) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Out, "  %s\n", fmt.Sprintf(format, args...))
}

// DryRun reports a command that would run in a real installation.
func (p *Printer) DryRun(command string) {
	fmt.Fprintf(p.Out, "  DRY-RUN would run: %s\n", command)
}

// Errorf writes a diagnostic to stderr.
func (p *Printer) Errorf(format string, args ...any) {
	fmt.Fprintf(p.Err, format+"\n", args...)
}

// Table renders aligned columns on stdout.
func (p *Printer) Table(header []string, rows [][]string) {
	writer := tabwriter.NewWriter(p.Out, 0, 4, 3, ' ', 0)
	if len(header) > 0 {
		fmt.Fprintln(writer, strings.Join(header, "\t"))
	}
	for _, row := range rows {
		fmt.Fprintln(writer, strings.Join(row, "\t"))
	}
	_ = writer.Flush()
}

// KeyValues renders an aligned key and value block.
func (p *Printer) KeyValues(pairs [][2]string) {
	writer := tabwriter.NewWriter(p.Out, 0, 4, 2, ' ', 0)
	for _, pair := range pairs {
		fmt.Fprintf(writer, "  %s:\t%s\n", pair[0], pair[1])
	}
	_ = writer.Flush()
}

// truncate shortens a string for table output.
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

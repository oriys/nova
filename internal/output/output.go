package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// Format represents output format
type Format string

const (
	FormatTable Format = "table"
	FormatWide  Format = "wide"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// ParseFormat parses a format string
func ParseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "json":
		return FormatJSON
	case "yaml", "yml":
		return FormatYAML
	case "wide":
		return FormatWide
	default:
		return FormatTable
	}
}

// Printer handles formatted output
type Printer struct {
	format Format
	writer io.Writer
	noColor bool
}

// NewPrinter creates a new printer
func NewPrinter(format Format) *Printer {
	return &Printer{
		format:  format,
		writer:  os.Stdout,
		noColor: os.Getenv("NO_COLOR") != "",
	}
}

// SetWriter sets the output writer
func (p *Printer) SetWriter(w io.Writer) {
	p.writer = w
}

// Print outputs data in the configured format
func (p *Printer) Print(data interface{}) error {
	switch p.format {
	case FormatJSON:
		return p.printJSON(data)
	case FormatYAML:
		return p.printYAML(data)
	default:
		// Table and Wide are handled by specific methods
		return p.printJSON(data)
	}
}

func (p *Printer) printJSON(data interface{}) error {
	enc := json.NewEncoder(p.writer)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func (p *Printer) printYAML(data interface{}) error {
	enc := yaml.NewEncoder(p.writer)
	enc.SetIndent(2)
	return enc.Encode(data)
}

// Color codes
const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Red       = "\033[31m"
	Green     = "\033[32m"
	Yellow    = "\033[33m"
	Blue      = "\033[34m"
	Magenta   = "\033[35m"
	Cyan      = "\033[36m"
	Gray      = "\033[90m"
)

// Colorize adds color to text
func (p *Printer) Colorize(color, text string) string {
	if p.noColor {
		return text
	}
	return color + text + Reset
}

// TableWriter creates a tabwriter for aligned output
func (p *Printer) TableWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(p.writer, 0, 0, 2, ' ', 0)
}

// FunctionRow represents a function in table output
type FunctionRow struct {
	Name        string `json:"name" yaml:"name"`
	Runtime     string `json:"runtime" yaml:"runtime"`
	Handler     string `json:"handler" yaml:"handler"`
	Memory      int    `json:"memory_mb" yaml:"memory_mb"`
	Timeout     int    `json:"timeout_s" yaml:"timeout_s"`
	MinReplicas int    `json:"min_replicas" yaml:"min_replicas"`
	Mode        string `json:"mode" yaml:"mode"`
	Version     int    `json:"version,omitempty" yaml:"version,omitempty"`
	Created     string `json:"created" yaml:"created"`
	Updated     string `json:"updated,omitempty" yaml:"updated,omitempty"`
}

// PrintFunctions prints function list
func (p *Printer) PrintFunctions(rows []FunctionRow) error {
	if p.format == FormatJSON || p.format == FormatYAML {
		return p.Print(rows)
	}

	if len(rows) == 0 {
		fmt.Fprintln(p.writer, "No functions found")
		return nil
	}

	w := p.TableWriter()

	// Header
	if p.format == FormatWide {
		fmt.Fprintln(w, p.Colorize(Bold, "NAME\tRUNTIME\tHANDLER\tMEMORY\tTIMEOUT\tMIN\tMODE\tVERSION\tCREATED\tUPDATED"))
	} else {
		fmt.Fprintln(w, p.Colorize(Bold, "NAME\tRUNTIME\tMEMORY\tTIMEOUT\tCREATED"))
	}

	for _, row := range rows {
		if p.format == FormatWide {
			fmt.Fprintf(w, "%s\t%s\t%s\t%dMB\t%ds\t%d\t%s\tv%d\t%s\t%s\n",
				p.Colorize(Cyan, row.Name),
				row.Runtime,
				row.Handler,
				row.Memory,
				row.Timeout,
				row.MinReplicas,
				row.Mode,
				row.Version,
				row.Created,
				row.Updated,
			)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%dMB\t%ds\t%s\n",
				p.Colorize(Cyan, row.Name),
				row.Runtime,
				row.Memory,
				row.Timeout,
				row.Created,
			)
		}
	}

	return w.Flush()
}

// InvokeResult represents invocation result
type InvokeResult struct {
	RequestID  string          `json:"request_id" yaml:"request_id"`
	Success    bool            `json:"success" yaml:"success"`
	Output     json.RawMessage `json:"output,omitempty" yaml:"output,omitempty"`
	Error      string          `json:"error,omitempty" yaml:"error,omitempty"`
	DurationMs int64           `json:"duration_ms" yaml:"duration_ms"`
	ColdStart  bool            `json:"cold_start" yaml:"cold_start"`
	Mode       string          `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// PrintInvokeResult prints invocation result
func (p *Printer) PrintInvokeResult(result InvokeResult) error {
	if p.format == FormatJSON || p.format == FormatYAML {
		return p.Print(result)
	}

	// Table format - human readable
	if result.Mode != "" {
		fmt.Fprintf(p.writer, "%s %s\n", p.Colorize(Bold, "Mode:"), result.Mode)
	}
	fmt.Fprintf(p.writer, "%s %s\n", p.Colorize(Bold, "Request ID:"), result.RequestID)

	if result.ColdStart {
		fmt.Fprintf(p.writer, "%s %s\n", p.Colorize(Bold, "Cold Start:"), p.Colorize(Yellow, "true"))
	} else {
		fmt.Fprintf(p.writer, "%s %s\n", p.Colorize(Bold, "Cold Start:"), p.Colorize(Green, "false"))
	}

	fmt.Fprintf(p.writer, "%s %d ms\n", p.Colorize(Bold, "Duration:"), result.DurationMs)

	if result.Error != "" {
		fmt.Fprintf(p.writer, "%s %s\n", p.Colorize(Bold, "Error:"), p.Colorize(Red, result.Error))
	} else {
		fmt.Fprintf(p.writer, "%s\n", p.Colorize(Bold, "Output:"))
		// Pretty print JSON output
		var prettyOutput interface{}
		if err := json.Unmarshal(result.Output, &prettyOutput); err == nil {
			formatted, _ := json.MarshalIndent(prettyOutput, "", "  ")
			fmt.Fprintln(p.writer, string(formatted))
		} else {
			fmt.Fprintln(p.writer, string(result.Output))
		}
	}

	return nil
}

// FunctionDetail represents detailed function info
type FunctionDetail struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	Runtime     string            `json:"runtime" yaml:"runtime"`
	Handler     string            `json:"handler" yaml:"handler"`
	CodePath    string            `json:"code_path" yaml:"code_path"`
	CodeHash    string            `json:"code_hash,omitempty" yaml:"code_hash,omitempty"`
	MemoryMB    int               `json:"memory_mb" yaml:"memory_mb"`
	TimeoutS    int               `json:"timeout_s" yaml:"timeout_s"`
	MinReplicas int               `json:"min_replicas" yaml:"min_replicas"`
	MaxReplicas int               `json:"max_replicas,omitempty" yaml:"max_replicas,omitempty"`
	Mode        string            `json:"mode" yaml:"mode"`
	Version     int               `json:"version,omitempty" yaml:"version,omitempty"`
	EnvVars     map[string]string `json:"env_vars,omitempty" yaml:"env_vars,omitempty"`
	Limits      interface{}       `json:"limits,omitempty" yaml:"limits,omitempty"`
	HasSnapshot bool              `json:"has_snapshot" yaml:"has_snapshot"`
	Versions    int               `json:"versions_count,omitempty" yaml:"versions_count,omitempty"`
	Aliases     []string          `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Created     string            `json:"created" yaml:"created"`
	Updated     string            `json:"updated" yaml:"updated"`
}

// PrintFunctionDetail prints detailed function info
func (p *Printer) PrintFunctionDetail(detail FunctionDetail) error {
	if p.format == FormatJSON || p.format == FormatYAML {
		return p.Print(detail)
	}

	// Human-readable format
	fmt.Fprintf(p.writer, "%s %s\n", p.Colorize(Bold, "Function:"), p.Colorize(Cyan, detail.Name))
	fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "ID:"), detail.ID)
	fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Runtime:"), detail.Runtime)
	fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Handler:"), detail.Handler)
	fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Code Path:"), detail.CodePath)
	if detail.CodeHash != "" {
		fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Code Hash:"), detail.CodeHash)
	}
	fmt.Fprintf(p.writer, "  %s %d MB\n", p.Colorize(Gray, "Memory:"), detail.MemoryMB)
	fmt.Fprintf(p.writer, "  %s %d s\n", p.Colorize(Gray, "Timeout:"), detail.TimeoutS)
	fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Mode:"), detail.Mode)
	fmt.Fprintf(p.writer, "  %s %d\n", p.Colorize(Gray, "Min Replicas:"), detail.MinReplicas)
	if detail.MaxReplicas > 0 {
		fmt.Fprintf(p.writer, "  %s %d\n", p.Colorize(Gray, "Max Replicas:"), detail.MaxReplicas)
	}
	if detail.Version > 0 {
		fmt.Fprintf(p.writer, "  %s v%d\n", p.Colorize(Gray, "Version:"), detail.Version)
	}

	if detail.HasSnapshot {
		fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Snapshot:"), p.Colorize(Green, "Yes"))
	} else {
		fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Snapshot:"), "No")
	}

	if len(detail.EnvVars) > 0 {
		fmt.Fprintf(p.writer, "  %s\n", p.Colorize(Gray, "Env Vars:"))
		for k, v := range detail.EnvVars {
			// Mask secret references
			displayV := v
			if strings.HasPrefix(v, "$SECRET:") {
				displayV = p.Colorize(Yellow, v)
			}
			fmt.Fprintf(p.writer, "    %s=%s\n", k, displayV)
		}
	}

	if detail.Versions > 0 {
		fmt.Fprintf(p.writer, "  %s %d\n", p.Colorize(Gray, "Versions:"), detail.Versions)
	}
	if len(detail.Aliases) > 0 {
		fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Aliases:"), strings.Join(detail.Aliases, ", "))
	}

	fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Created:"), detail.Created)
	fmt.Fprintf(p.writer, "  %s %s\n", p.Colorize(Gray, "Updated:"), detail.Updated)

	return nil
}

// LogEntry represents a log entry
type LogEntry struct {
	Timestamp  string `json:"timestamp" yaml:"timestamp"`
	RequestID  string `json:"request_id" yaml:"request_id"`
	Function   string `json:"function" yaml:"function"`
	Level      string `json:"level" yaml:"level"`
	Message    string `json:"message" yaml:"message"`
	DurationMs int64  `json:"duration_ms,omitempty" yaml:"duration_ms,omitempty"`
}

// PrintLogEntry prints a single log entry
func (p *Printer) PrintLogEntry(entry LogEntry) error {
	if p.format == FormatJSON {
		return p.printJSON(entry)
	}

	// Colorize level
	levelColor := Gray
	switch strings.ToUpper(entry.Level) {
	case "ERROR", "ERR":
		levelColor = Red
	case "WARN", "WARNING":
		levelColor = Yellow
	case "INFO":
		levelColor = Green
	case "DEBUG":
		levelColor = Gray
	}

	fmt.Fprintf(p.writer, "%s %s %s %s\n",
		p.Colorize(Gray, entry.Timestamp),
		p.Colorize(Cyan, "["+entry.RequestID+"]"),
		p.Colorize(levelColor, entry.Level),
		entry.Message,
	)

	return nil
}

// Success prints a success message
func (p *Printer) Success(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(p.writer, p.Colorize(Green, "✓ ")+msg)
}

// Error prints an error message
func (p *Printer) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(p.writer, p.Colorize(Red, "✗ ")+msg)
}

// Warning prints a warning message
func (p *Printer) Warning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(p.writer, p.Colorize(Yellow, "⚠ ")+msg)
}

// Info prints an info message
func (p *Printer) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(p.writer, p.Colorize(Blue, "ℹ ")+msg)
}

package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultReadMaxLines = 2000
	defaultReadMaxBytes = 50 * 1024
)

type ReadTool struct {
	WorkingDir string
}

func (t *ReadTool) Name() string { return "read" }
func (t *ReadTool) Description() string {
	return "Read a file from the local filesystem using an absolute path or a path relative to the working directory"
}
func (t *ReadTool) Parameters() map[string]interface{} {
	return ObjectSchema(
		map[string]map[string]interface{}{
			"file_path": StringParam("The path to the file to read, absolute or relative to the working directory"),
			"offset":    IntParam("Line number to start reading from (1-indexed)"),
			"limit":     IntParam("Maximum number of lines to read"),
		},
		"file_path",
	)
}

func (t *ReadTool) Run(ctx context.Context, args map[string]interface{}) (Result, error) {
	filePath, ok := stringArg(args, "file_path")
	if !ok || strings.TrimSpace(filePath) == "" {
		return Result{Content: "file_path is required", IsError: true}, nil
	}
	filePath, err := t.resolvePath(filePath)
	if err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}

	offset, err := intArgWithDefault(args, "offset", 1)
	if err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}
	if offset < 1 {
		return Result{Content: "offset must be >= 1", IsError: true}, nil
	}

	limit, hasLimit, err := optionalIntArg(args, "limit")
	if err != nil {
		return Result{Content: err.Error(), IsError: true}, nil
	}
	if hasLimit && limit < 1 {
		return Result{Content: "limit must be >= 1", IsError: true}, nil
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Content: fmt.Sprintf("file does not exist: %s", filePath), IsError: true}, nil
		}
		if os.IsPermission(err) {
			return Result{Content: fmt.Sprintf("file is not readable: %s", filePath), IsError: true}, nil
		}
		return Result{}, err
	}
	if info.IsDir() {
		return Result{Content: "path is a directory, not a file", IsError: true}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsPermission(err) {
			return Result{Content: fmt.Sprintf("file is not readable: %s", filePath), IsError: true}, nil
		}
		return Result{}, err
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	lines := splitFileLines(string(data))
	totalLines := len(lines)
	if totalLines == 0 {
		return Result{Content: ""}, nil
	}
	startIdx := offset - 1
	if startIdx >= totalLines {
		return Result{Content: fmt.Sprintf("offset %d is beyond end of file (%d lines total)", offset, totalLines), IsError: true}, nil
	}

	endIdx := totalLines
	if hasLimit && startIdx+limit < endIdx {
		endIdx = startIdx + limit
	}

	selected := lines[startIdx:endIdx]
	formattedLines := make([]string, 0, len(selected))
	for i, line := range selected {
		formattedLines = append(formattedLines, fmt.Sprintf("%d: %s", startIdx+i+1, line))
	}

	body, outputLines, truncated := truncateFormattedLines(formattedLines, defaultReadMaxLines, defaultReadMaxBytes)
	lastShownLine := startIdx + outputLines
	remainingLines := totalLines - lastShownLine

	if truncated.byBytes {
		if outputLines == 0 {
			return Result{Content: fmt.Sprintf("line %d exceeds %d byte limit", offset, defaultReadMaxBytes), IsError: true}, nil
		}
		body += fmt.Sprintf("\n\n[%d more lines in file. Use offset=%d to continue.]", remainingLines, lastShownLine+1)
		return Result{Content: body}, nil
	}
	if truncated.byLines {
		body += fmt.Sprintf("\n\n[%d more lines in file. Use offset=%d to continue.]", remainingLines, lastShownLine+1)
		return Result{Content: body}, nil
	}

	if hasLimit && endIdx < totalLines {
		body += fmt.Sprintf("\n\n[%d more lines in file. Use offset=%d to continue.]", totalLines-endIdx, endIdx+1)
	}

	return Result{Content: body}, nil
}

func (t *ReadTool) resolvePath(filePath string) (string, error) {
	if filepath.IsAbs(filePath) {
		return filepath.Clean(filePath), nil
	}
	workingDir := t.WorkingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("could not determine working directory")
		}
	}
	return filepath.Clean(filepath.Join(workingDir, filePath)), nil
}

func stringArg(args map[string]interface{}, key string) (string, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func intArgWithDefault(args map[string]interface{}, key string, defaultValue int) (int, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultValue, nil
	}
	return intFromValue(v, key)
}

func optionalIntArg(args map[string]interface{}, key string) (int, bool, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	n, err := intFromValue(v, key)
	if err != nil {
		return 0, false, err
	}
	return n, true, nil
}

func intFromValue(v interface{}, key string) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func splitFileLines(text string) []string {
	if text == "" {
		return []string{}
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type readTruncation struct {
	byLines bool
	byBytes bool
}

func truncateFormattedLines(lines []string, maxLines, maxBytes int) (string, int, readTruncation) {
	if len(lines) == 0 {
		return "", 0, readTruncation{}
	}

	var b strings.Builder
	outputLines := 0

	for i, line := range lines {
		if outputLines >= maxLines {
			break
		}

		additionalBytes := len(line)
		if i > 0 {
			additionalBytes++
		}
		if b.Len()+additionalBytes > maxBytes {
			return b.String(), outputLines, readTruncation{byBytes: true}
		}

		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		outputLines++
	}

	truncated := readTruncation{}
	if outputLines < len(lines) {
		truncated.byLines = true
	}

	return b.String(), outputLines, truncated
}

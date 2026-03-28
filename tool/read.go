package tool

import "context"

type ReadTool struct{}

func (t *ReadTool) Name() string        { return "read" }
func (t *ReadTool) Description() string { return "Read a file from the local filesystem" }
func (t *ReadTool) Parameters() map[string]interface{} {
	return ObjectSchema(
		map[string]map[string]interface{}{
			"file_path": StringParam("The absolute path to the file to read"),
			"offset":    IntParam("Line number to start reading from (1-indexed)"),
			"limit":     IntParam("Maximum number of lines to read"),
		},
		"file_path",
	)
}
func (t *ReadTool) Run(_ context.Context, args map[string]interface{}) (Result, error) {
	return Result{Content: "stub: read not implemented", IsError: true}, nil
}

package tool

import "context"

type WriteTool struct{}

func (t *WriteTool) Name() string        { return "write" }
func (t *WriteTool) Description() string { return "Write content to a file on the local filesystem" }
func (t *WriteTool) Parameters() map[string]interface{} {
	return ObjectSchema(
		map[string]map[string]interface{}{
			"file_path": StringParam("The absolute path to the file to write"),
			"content":   StringParam("The content to write to the file"),
		},
		"file_path", "content",
	)
}
func (t *WriteTool) Run(_ context.Context, args map[string]interface{}) (Result, error) {
	return Result{Content: "stub: write not implemented", IsError: true}, nil
}

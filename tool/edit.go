package tool

import "context"

type EditTool struct{}

func (t *EditTool) Name() string        { return "edit" }
func (t *EditTool) Description() string { return "Perform exact string replacements in a file" }
func (t *EditTool) Parameters() map[string]interface{} {
	return ObjectSchema(
		map[string]map[string]interface{}{
			"file_path":  StringParam("The absolute path to the file to modify"),
			"old_string": StringParam("The text to replace"),
			"new_string": StringParam("The text to replace it with"),
		},
		"file_path", "old_string", "new_string",
	)
}
func (t *EditTool) Run(_ context.Context, args map[string]interface{}) (Result, error) {
	return Result{Content: "stub: edit not implemented", IsError: true}, nil
}

package tool

import "context"

type BashTool struct{}

func (t *BashTool) Name() string { return "bash" }
func (t *BashTool) Description() string {
	return "Execute a bash command in a persistent shell session"
}
func (t *BashTool) Parameters() map[string]interface{} {
	return ObjectSchema(
		map[string]map[string]interface{}{
			"command": StringParam("The bash command to execute"),
		},
		"command",
	)
}
func (t *BashTool) Run(_ context.Context, args map[string]interface{}) (Result, error) {
	return Result{Content: "stub: bash not implemented", IsError: true}, nil
}

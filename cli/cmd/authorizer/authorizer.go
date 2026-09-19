package authorizer

const (
	allowed string = "allowed"
)

type AuthorizerCmd struct {
	CheckDecision EvalCmd `cmd:"" name:"eval" help:"evaluate policy decision"`
	Test          TestCmd `cmd:"" help:"execute authorizer assertions"`
}

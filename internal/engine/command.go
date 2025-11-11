// internal/engine/command.go
package engine

type CommandType int

const (
	CmdPlace CommandType = iota
	CmdCancel
)

type Command struct {
	Type  CommandType
	Order *Order   // used when Type == CmdPlace
	ID    string   // used when Type == CmdCancel
	Resp  chan any // engine sends the result back here
}

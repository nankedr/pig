package protocol

// CommandName is a request-command wire discriminator.
type CommandName string

const (
	CommandNameList        CommandName = "list"
	CommandNameCreate      CommandName = "create"
	CommandNameAttach      CommandName = "attach"
	CommandNameDetach      CommandName = "detach"
	CommandNamePrompt      CommandName = "prompt"
	CommandNameSteer       CommandName = "steer"
	CommandNameAbort       CommandName = "abort"
	CommandNameSetModel    CommandName = "set_model"
	CommandNameSetThinking CommandName = "set_thinking"
)

// Command is a closed union of remote session commands.
type Command interface {
	command()
	CommandName() CommandName
}

type ListCommand struct {
	Command CommandName `json:"command"`
}

func (ListCommand) command()                   {}
func (c ListCommand) CommandName() CommandName { return c.Command }

type CreateCommand struct {
	Command       CommandName             `json:"command"`
	CWD           Optional[string]        `json:"cwd"`
	Name          Optional[string]        `json:"name"`
	Model         Optional[ModelRef]      `json:"model"`
	ThinkingLevel Optional[ThinkingLevel] `json:"thinkingLevel"`
}

func (CreateCommand) command()                   {}
func (c CreateCommand) CommandName() CommandName { return c.Command }

type AttachCommand struct {
	Command   CommandName `json:"command"`
	SessionID string      `json:"sessionId"`
}

func (AttachCommand) command()                   {}
func (c AttachCommand) CommandName() CommandName { return c.Command }

type DetachCommand struct {
	Command   CommandName `json:"command"`
	SessionID string      `json:"sessionId"`
}

func (DetachCommand) command()                   {}
func (c DetachCommand) CommandName() CommandName { return c.Command }

type PromptCommand struct {
	Command   CommandName `json:"command"`
	SessionID string      `json:"sessionId"`
	Text      string      `json:"text"`
}

func (PromptCommand) command()                   {}
func (c PromptCommand) CommandName() CommandName { return c.Command }

type SteerCommand struct {
	Command   CommandName `json:"command"`
	SessionID string      `json:"sessionId"`
	Text      string      `json:"text"`
}

func (SteerCommand) command()                   {}
func (c SteerCommand) CommandName() CommandName { return c.Command }

type AbortCommand struct {
	Command   CommandName `json:"command"`
	SessionID string      `json:"sessionId"`
}

func (AbortCommand) command()                   {}
func (c AbortCommand) CommandName() CommandName { return c.Command }

type SetModelCommand struct {
	Command   CommandName `json:"command"`
	SessionID string      `json:"sessionId"`
	Model     ModelRef    `json:"model"`
}

func (SetModelCommand) command()                   {}
func (c SetModelCommand) CommandName() CommandName { return c.Command }

type SetThinkingCommand struct {
	Command       CommandName   `json:"command"`
	SessionID     string        `json:"sessionId"`
	ThinkingLevel ThinkingLevel `json:"thinkingLevel"`
}

func (SetThinkingCommand) command()                   {}
func (c SetThinkingCommand) CommandName() CommandName { return c.Command }

// CommandResult is a closed union of command results. Concrete result variants
// correlate with commands through their CommandName discriminator; Pig does not
// reproduce the upstream TypeScript conditional type.
type CommandResult interface {
	commandResult()
	ResultCommandName() CommandName
}

type ListResult struct {
	Command  CommandName       `json:"command"`
	Sessions []SessionMetadata `json:"sessions"`
}

func (ListResult) commandResult()                   {}
func (r ListResult) ResultCommandName() CommandName { return r.Command }

type CreateResult struct {
	Command CommandName     `json:"command"`
	Session SessionSnapshot `json:"session"`
}

func (CreateResult) commandResult()                   {}
func (r CreateResult) ResultCommandName() CommandName { return r.Command }

type AttachResult struct {
	Command CommandName     `json:"command"`
	Session SessionSnapshot `json:"session"`
}

func (AttachResult) commandResult()                   {}
func (r AttachResult) ResultCommandName() CommandName { return r.Command }

type DetachResult struct {
	Command   CommandName `json:"command"`
	SessionID string      `json:"sessionId"`
}

func (DetachResult) commandResult()                   {}
func (r DetachResult) ResultCommandName() CommandName { return r.Command }

type PromptResult struct {
	Command CommandName     `json:"command"`
	Session SessionSnapshot `json:"session"`
}

func (PromptResult) commandResult()                   {}
func (r PromptResult) ResultCommandName() CommandName { return r.Command }

type SteerResult struct {
	Command CommandName     `json:"command"`
	Session SessionSnapshot `json:"session"`
}

func (SteerResult) commandResult()                   {}
func (r SteerResult) ResultCommandName() CommandName { return r.Command }

type AbortResult struct {
	Command CommandName     `json:"command"`
	Session SessionSnapshot `json:"session"`
}

func (AbortResult) commandResult()                   {}
func (r AbortResult) ResultCommandName() CommandName { return r.Command }

type SetModelResult struct {
	Command CommandName     `json:"command"`
	Session SessionSnapshot `json:"session"`
}

func (SetModelResult) commandResult()                   {}
func (r SetModelResult) ResultCommandName() CommandName { return r.Command }

type SetThinkingResult struct {
	Command CommandName     `json:"command"`
	Session SessionSnapshot `json:"session"`
}

func (SetThinkingResult) commandResult()                   {}
func (r SetThinkingResult) ResultCommandName() CommandName { return r.Command }

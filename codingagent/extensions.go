package codingagent

// This file fixes the public extension boundary without selecting an
// extension execution ABI.  The data carriers are usable by callers today;
// operations that would load or run extension code remain explicit capability
// stubs until the extension-runtime milestone.

import (
	"encoding/json"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
)

// EventBus inventories the extension-facing named-channel boundary without
// selecting callback, dispatch, unsubscribe, or error-isolation semantics.
type EventBus struct {
	Emit ExtensionHandler
	On   ExtensionHandler
}

type EventBusController struct {
	EventBus
	Clear ExtensionHandler
}

func CreateEventBus() (EventBusController, error) {
	return EventBusController{}, notImplemented("CreateEventBus")
}

type ExecOptions struct {
	CWD     string
	Timeout time.Duration
}

type ExecResult struct {
	Code   int
	Killed bool
	Stderr string
	Stdout string
}

type ReadonlyFooterDataProvider interface {
	GetAvailableProviderCount() int
	GetExtensionStatuses() map[string]string
	GetGitBranch() *string
	OnBranchChange(ExtensionHandler) (ExtensionHandler, error)
}

type AppKeybinding string

// AppKeybinding values are the complete keyof AppKeybindings set exported by
// the pinned Coding Agent baseline.
const (
	AppKeybindingInterrupt                AppKeybinding = "app.interrupt"
	AppKeybindingClear                    AppKeybinding = "app.clear"
	AppKeybindingExit                     AppKeybinding = "app.exit"
	AppKeybindingSuspend                  AppKeybinding = "app.suspend"
	AppKeybindingThinkingCycle            AppKeybinding = "app.thinking.cycle"
	AppKeybindingModelCycleForward        AppKeybinding = "app.model.cycleForward"
	AppKeybindingModelCycleBackward       AppKeybinding = "app.model.cycleBackward"
	AppKeybindingModelSelect              AppKeybinding = "app.model.select"
	AppKeybindingToolsExpand              AppKeybinding = "app.tools.expand"
	AppKeybindingThinkingToggle           AppKeybinding = "app.thinking.toggle"
	AppKeybindingSessionToggleNamedFilter AppKeybinding = "app.session.toggleNamedFilter"
	AppKeybindingEditorExternal           AppKeybinding = "app.editor.external"
	AppKeybindingMessageCopy              AppKeybinding = "app.message.copy"
	AppKeybindingMessageFollowUp          AppKeybinding = "app.message.followUp"
	AppKeybindingMessageDequeue           AppKeybinding = "app.message.dequeue"
	AppKeybindingClipboardPasteImage      AppKeybinding = "app.clipboard.pasteImage"
	AppKeybindingSessionNew               AppKeybinding = "app.session.new"
	AppKeybindingSessionTree              AppKeybinding = "app.session.tree"
	AppKeybindingSessionFork              AppKeybinding = "app.session.fork"
	AppKeybindingSessionResume            AppKeybinding = "app.session.resume"
	AppKeybindingTreeFoldOrUp             AppKeybinding = "app.tree.foldOrUp"
	AppKeybindingTreeUnfoldOrDown         AppKeybinding = "app.tree.unfoldOrDown"
	AppKeybindingTreeEditLabel            AppKeybinding = "app.tree.editLabel"
	AppKeybindingTreeToggleLabelTimestamp AppKeybinding = "app.tree.toggleLabelTimestamp"
	AppKeybindingSessionTogglePath        AppKeybinding = "app.session.togglePath"
	AppKeybindingSessionToggleSort        AppKeybinding = "app.session.toggleSort"
	AppKeybindingSessionRename            AppKeybinding = "app.session.rename"
	AppKeybindingSessionDelete            AppKeybinding = "app.session.delete"
	AppKeybindingSessionDeleteNoninvasive AppKeybinding = "app.session.deleteNoninvasive"
	AppKeybindingModelsSave               AppKeybinding = "app.models.save"
	AppKeybindingModelsEnableAll          AppKeybinding = "app.models.enableAll"
	AppKeybindingModelsClearAll           AppKeybinding = "app.models.clearAll"
	AppKeybindingModelsToggleProvider     AppKeybinding = "app.models.toggleProvider"
	AppKeybindingModelsReorderUp          AppKeybinding = "app.models.reorderUp"
	AppKeybindingModelsReorderDown        AppKeybinding = "app.models.reorderDown"
	AppKeybindingTreeFilterDefault        AppKeybinding = "app.tree.filter.default"
	AppKeybindingTreeFilterNoTools        AppKeybinding = "app.tree.filter.noTools"
	AppKeybindingTreeFilterUserOnly       AppKeybinding = "app.tree.filter.userOnly"
	AppKeybindingTreeFilterLabeledOnly    AppKeybinding = "app.tree.filter.labeledOnly"
	AppKeybindingTreeFilterAll            AppKeybinding = "app.tree.filter.all"
	AppKeybindingTreeFilterCycleForward   AppKeybinding = "app.tree.filter.cycleForward"
	AppKeybindingTreeFilterCycleBackward  AppKeybinding = "app.tree.filter.cycleBackward"
)

type KeybindingsManager struct {
	tui.KeybindingsManager
}

func NewKeybindingsManager(...string) (*KeybindingsManager, error) {
	return nil, notImplemented("NewKeybindingsManager")
}
func (*KeybindingsManager) GetEffectiveConfig() (tui.KeybindingsConfig, error) {
	return nil, notImplemented("KeybindingsManager.GetEffectiveConfig")
}
func (*KeybindingsManager) Reload() error {
	return notImplemented("KeybindingsManager.Reload")
}

// ConvertToLLM projects persisted Coding Agent messages into Provider input.
func ConvertToLLM(messages []agent.AgentMessage) []ai.Message {
	out := []ai.Message{}
	for _, message := range messages {
		if raw, ok := message.(interface{ RawJSON() json.RawMessage }); ok {
			switch message.MessageRole() {
			case "custom":
				var value agent.CustomMessage
				if json.Unmarshal(raw.RawJSON(), &value) == nil {
					message = value
				}
			case "branchSummary":
				var value agent.BranchSummaryMessage
				if json.Unmarshal(raw.RawJSON(), &value) == nil {
					message = value
				}
			case "compactionSummary":
				var value agent.CompactionSummaryMessage
				if json.Unmarshal(raw.RawJSON(), &value) == nil {
					message = value
				}
			case "bashExecution":
				var value agent.BashExecutionMessage
				if json.Unmarshal(raw.RawJSON(), &value) == nil {
					fields, _ := decodeJSONObject(raw.RawJSON())
					value.ExitCodeSet = len(fields["exitCode"]) != 0 && string(fields["exitCode"]) != "null"
					message = value
				}
			}
		}
		for _, converted := range agent.ConvertToLLM([]agent.AgentMessage{message}) {
			if user, ok := converted.(ai.UserMessage); ok {
				if _, builtin := message.(ai.Message); !builtin {
					if text, ok := user.Content.Text(); ok {
						user.Content = ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: text})
					}
					converted = user
				}
			}
			out = append(out, converted)
		}
	}
	return out
}

type ExtensionUIDialogOptions struct {
	Timeout time.Duration
}

type WidgetPlacement string

const (
	WidgetPlacementAboveEditor WidgetPlacement = "aboveEditor"
	WidgetPlacementBelowEditor WidgetPlacement = "belowEditor"
)

type ExtensionWidgetOptions struct {
	Placement WidgetPlacement
}

// extensionCapability is the deliberately uninhabited implementation behind
// extension callback carriers. The exported names preserve the pinned surface,
// but none of them can be called or populated with a Go function before M7
// selects an extension execution ABI.
type extensionCapability struct{}

type TerminalInputHandler *extensionCapability
type AutocompleteProviderFactory *extensionCapability
type EntryRenderer *extensionCapability
type ExtensionFactory *extensionCapability
type ExtensionHandler *extensionCapability
type MarkdownTransformer *extensionCapability
type MessageRenderer *extensionCapability
type ProjectTrustHandler *extensionCapability

type WorkingIndicatorOptions struct {
	Frames     []string
	IntervalMS int64
}

// ExtensionUIContext inventories the full host UI capability set. Its
// operation fields are inert until M7 selects an executable ABI.
type ExtensionUIContext struct {
	AddAutocompleteProvider ExtensionHandler
	Confirm                 ExtensionHandler
	Custom                  ExtensionHandler
	Editor                  ExtensionHandler
	GetAllThemes            ExtensionHandler
	GetEditorComponent      ExtensionHandler
	GetEditorText           ExtensionHandler
	GetTheme                ExtensionHandler
	GetToolsExpanded        ExtensionHandler
	Input                   ExtensionHandler
	Notify                  ExtensionHandler
	OnTerminalInput         ExtensionHandler
	PasteToEditor           ExtensionHandler
	Select                  ExtensionHandler
	SetEditorComponent      ExtensionHandler
	SetEditorText           ExtensionHandler
	SetFooter               ExtensionHandler
	SetHeader               ExtensionHandler
	SetHiddenThinkingLabel  ExtensionHandler
	SetStatus               ExtensionHandler
	SetTheme                ExtensionHandler
	SetTitle                ExtensionHandler
	SetToolsExpanded        ExtensionHandler
	SetWidget               ExtensionHandler
	SetWorkingIndicator     ExtensionHandler
	SetWorkingMessage       ExtensionHandler
	SetWorkingVisible       ExtensionHandler
	Theme                   *Theme
}

type ContextUsage struct {
	ContextWindow int64
	Percent       *float64
	Tokens        *int64
}

type CompactOptions struct {
	CustomInstructions string
	OnComplete         ExtensionHandler
	OnError            ExtensionHandler
}

type ExtensionMode string

const (
	ExtensionModeTUI   ExtensionMode = "tui"
	ExtensionModeRPC   ExtensionMode = "rpc"
	ExtensionModeJSON  ExtensionMode = "json"
	ExtensionModePrint ExtensionMode = "print"
)

type ExtensionContext struct {
	Abort              ExtensionHandler
	Compact            ExtensionHandler
	CWD                string
	GetContextUsage    ExtensionHandler
	GetSystemPrompt    ExtensionHandler
	HasPendingMessages ExtensionHandler
	HasUI              bool
	IsIdle             ExtensionHandler
	IsProjectTrusted   ExtensionHandler
	Mode               ExtensionMode
	Model              *ai.Model
	ModelRegistry      *ModelRegistry
	ScopedModels       []ScopedModel
	SessionManager     *SessionManager
	Shutdown           ExtensionHandler
	ThinkingLevel      agent.ThinkingLevel
	UI                 ExtensionUIContext
}

type ExtensionCommandContext struct {
	ExtensionContext
	Fork                   ExtensionHandler
	GetSystemPromptOptions ExtensionHandler
	NavigateTree           ExtensionHandler
	NewSession             ExtensionHandler
	Reload                 ExtensionHandler
	SwitchSession          ExtensionHandler
	WaitForIdle            ExtensionHandler
}

type ExtensionContextActions struct {
	Abort                  ExtensionHandler
	Compact                ExtensionHandler
	GetContextUsage        ExtensionHandler
	GetModel               ExtensionHandler
	GetScopedModels        ExtensionHandler
	GetSignal              ExtensionHandler
	GetSystemPrompt        ExtensionHandler
	GetSystemPromptOptions ExtensionHandler
	HasPendingMessages     ExtensionHandler
	IsIdle                 ExtensionHandler
	IsProjectTrusted       ExtensionHandler
	Shutdown               ExtensionHandler
}

type ExtensionCommandContextActions struct {
	Fork          ExtensionHandler
	NavigateTree  ExtensionHandler
	NewSession    ExtensionHandler
	Reload        ExtensionHandler
	SwitchSession ExtensionHandler
	WaitForIdle   ExtensionHandler
}

type ExtensionActions struct {
	AppendEntry      ExtensionHandler
	GetActiveTools   ExtensionHandler
	GetAllTools      ExtensionHandler
	GetCommands      ExtensionHandler
	GetSessionName   ExtensionHandler
	GetThinkingLevel ExtensionHandler
	RefreshTools     ExtensionHandler
	SendMessage      ExtensionHandler
	SendUserMessage  ExtensionHandler
	SetActiveTools   ExtensionHandler
	SetLabel         ExtensionHandler
	SetModel         ExtensionHandler
	SetSessionName   ExtensionHandler
	SetThinkingLevel ExtensionHandler
}

type ExtensionAPI struct {
	AppendEntry                 ExtensionHandler
	Events                      EventBus
	Exec                        ExtensionHandler
	GetActiveTools              ExtensionHandler
	GetAllTools                 ExtensionHandler
	GetCommands                 ExtensionHandler
	GetFlag                     ExtensionHandler
	GetSessionName              ExtensionHandler
	GetThinkingLevel            ExtensionHandler
	On                          ExtensionHandler
	RegisterCommand             ExtensionHandler
	RegisterEntryRenderer       ExtensionHandler
	RegisterFlag                ExtensionHandler
	RegisterMarkdownTransformer ExtensionHandler
	RegisterMessageRenderer     ExtensionHandler
	RegisterProvider            ExtensionHandler
	RegisterShortcut            ExtensionHandler
	RegisterTool                ExtensionHandler
	SendMessage                 ExtensionHandler
	SendUserMessage             ExtensionHandler
	SetActiveTools              ExtensionHandler
	SetLabel                    ExtensionHandler
	SetModel                    ExtensionHandler
	SetSessionName              ExtensionHandler
	SetThinkingLevel            ExtensionHandler
	UnregisterProvider          ExtensionHandler
}

type EntryRenderOptions struct {
	Expanded bool
}

type MessageRenderOptions struct {
	Expanded  bool
	OutputPad bool
}

type MarkdownTransformContext struct {
	AvailableWidth int
	IsStreaming    bool
	MessageType    string
}

type ExtensionFlag struct {
	Default       any
	Description   string
	ExtensionPath string
	Name          string
	Type          string
}

type ExtensionShortcut struct {
	Description   string
	ExtensionPath string
	Handler       ExtensionHandler
	Shortcut      string
}

type RegisteredCommand struct {
	Description            string
	GetArgumentCompletions ExtensionHandler
	Handler                ExtensionHandler
	Name                   string
	SourceInfo             SourceInfo
}

type ResolvedCommand struct {
	Description            string
	GetArgumentCompletions ExtensionHandler
	Handler                ExtensionHandler
	InvocationName         string
	Name                   string
	SourceInfo             SourceInfo
}

type RegisteredTool struct {
	Definition ToolDefinition
	SourceInfo SourceInfo
}

type ToolInfo struct {
	Description      string
	Name             string
	Parameters       json.RawMessage
	PromptGuidelines []string
	SourceInfo       SourceInfo
}

type Extension struct {
	Commands            []RegisteredCommand
	EntryRenderers      map[string]EntryRenderer
	Flags               []ExtensionFlag
	Handlers            map[string][]ExtensionHandler
	Hidden              bool
	MarkdownTransformer MarkdownTransformer
	MessageRenderers    map[string]MessageRenderer
	Path                string
	ResolvedPath        string
	Shortcuts           []ExtensionShortcut
	SourceInfo          SourceInfo
	Tools               []RegisteredTool
}

type ExtensionError struct {
	Error         string
	Event         string
	ExtensionPath string
	Stack         *string
}

// ExtensionRuntime is the opaque carrier for the host capabilities consumed
// by an extension runtime. Its upstream members are inventoried in the Parity
// Catalog, but their representation remains deferred until M7.
type ExtensionRuntime struct{}

type LoadExtensionsResult struct {
	Errors []struct {
		Path  string
		Error string
	}
	Extensions []Extension
	Runtime    *ExtensionRuntime
}

// These named carriers inventory upstream runtime operations without choosing
// construction or tool-dispatch signatures before the M7 extension-runtime
// decision. They deliberately cannot be called.
type CreateExtensionRuntime *extensionCapability
type WrapRegisteredTool *extensionCapability
type WrapRegisteredTools *extensionCapability

// DiscoverAndLoadExtensions preserves the public discovery entry without
// freezing path, loading, cancellation, or callback ABI before M7.
func DiscoverAndLoadExtensions() (LoadExtensionsResult, error) {
	return LoadExtensionsResult{}, notImplemented("DiscoverAndLoadExtensions")
}

type ProjectTrustEventDecision string

const (
	ProjectTrustEventDecisionYes       ProjectTrustEventDecision = "yes"
	ProjectTrustEventDecisionNo        ProjectTrustEventDecision = "no"
	ProjectTrustEventDecisionUndecided ProjectTrustEventDecision = "undecided"
)

type ProjectTrustEvent struct{ Type, CWD string }
type ProjectTrustEventResult struct {
	Remember bool
	Trusted  ProjectTrustEventDecision
}
type ProjectTrustContext struct {
	CWD   string
	HasUI bool
	Mode  ExtensionMode
	UI    ExtensionUIContext
}

type SessionStartEvent struct{ Type, Reason, PreviousSessionFile string }
type SessionInfoChangedEvent struct{ Type, Name string }
type SessionBeforeSwitchEvent struct{ Type, Reason, TargetSessionFile string }
type SessionBeforeForkEvent struct{ Type, EntryID, Position string }
type SessionBeforeCompactEvent struct {
	BranchEntries      []SessionEntry
	CustomInstructions string
	Preparation        any
	Reason             string
	Type               string
	WillRetry          bool
}
type SessionCompactEvent struct {
	CompactionEntry CompactionEntry
	FromExtension   bool
	Reason          string
	Type            string
	WillRetry       bool
}
type SessionShutdownEvent struct{ Type, Reason, TargetSessionFile string }
type SessionBeforeTreeEvent struct {
	Preparation any
	Type        string
}
type SessionTreeEvent struct {
	FromExtension bool
	NewLeafID     *string
	OldLeafID     *string
	SummaryEntry  *BranchSummaryEntry
	Type          string
}

type ContextEvent struct {
	Messages []agent.AgentMessage
	Type     string
}
type BeforeProviderRequestEvent struct {
	Payload any
	Type    string
}
type BeforeProviderRequestEventResult any
type BeforeProviderHeadersEvent struct {
	Headers ai.ProviderHeaders
	Type    string
}
type BeforeAgentStartEvent struct {
	Images              []ai.ImageContent
	Prompt              string
	SystemPrompt        string
	SystemPromptOptions BuildSystemPromptOptions
	Type                string
}
type BeforeAgentStartEventResult struct {
	Message      agent.AgentMessage
	SystemPrompt *string
}
type AgentStartEvent struct{ Type string }
type AgentEndEvent struct {
	Messages []agent.AgentMessage
	Type     string
}
type AgentSettledEvent struct{ Type string }
type TurnStartEvent struct {
	Timestamp int64
	TurnIndex int
	Type      string
}
type TurnEndEvent struct {
	Message     agent.AgentMessage
	ToolResults []ai.ToolResultMessage
	TurnIndex   int
	Type        string
}
type MessageStartEvent struct {
	Message agent.AgentMessage
	Type    string
}
type MessageUpdateEvent struct {
	AssistantMessageEvent ai.AssistantMessageEvent
	Message               agent.AgentMessage
	Type                  string
}
type MessageEndEvent struct {
	Message agent.AgentMessage
	Type    string
}
type ToolExecutionStartEvent struct {
	Args                       any
	ToolCallID, ToolName, Type string
}
type ToolExecutionUpdateEvent struct {
	Args, PartialResult        any
	ToolCallID, ToolName, Type string
}
type ToolExecutionEndEvent struct {
	IsError                    bool
	Result                     any
	ToolCallID, ToolName, Type string
}
type UserBashEvent struct {
	Command, CWD       string
	ExcludeFromContext bool
	Type               string
}
type UserBashEventResult struct {
	Operations BashOperations
	Result     any
}

type InputSource string

const (
	InputSourceInteractive InputSource = "interactive"
	InputSourceRPC         InputSource = "rpc"
	InputSourceExtension   InputSource = "extension"
)

type InputEvent struct {
	Images            []ai.ImageContent
	Source            InputSource
	StreamingBehavior string
	Text              string
	Type              string
}
type InputEventResult struct{ Action string }

type BashToolCallEvent struct {
	Input                      BashToolInput
	ToolCallID, ToolName, Type string
}
type ReadToolCallEvent struct {
	Input                      ReadToolInput
	ToolCallID, ToolName, Type string
}
type EditToolCallEvent struct {
	Input                      EditToolInput
	ToolCallID, ToolName, Type string
}
type WriteToolCallEvent struct {
	Input                      WriteToolInput
	ToolCallID, ToolName, Type string
}
type GrepToolCallEvent struct {
	Input                      GrepToolInput
	ToolCallID, ToolName, Type string
}
type FindToolCallEvent struct {
	Input                      FindToolInput
	ToolCallID, ToolName, Type string
}
type LsToolCallEvent struct {
	Input                      LsToolInput
	ToolCallID, ToolName, Type string
}
type CustomToolCallEvent struct {
	Input                      map[string]any
	ToolCallID, ToolName, Type string
}
type ToolCallEvent struct {
	Input                      any
	ToolCallID, ToolName, Type string
}

type ToolResultEvent struct {
	Content    []ai.ToolResultContent
	Details    any
	Input      map[string]any
	IsError    bool
	ToolCallID string
	ToolName   string
	Type       string
	Usage      *ai.Usage
}

type ToolCallEventResult struct {
	Block     bool
	Reason    string
	Terminate bool
}
type ToolRenderResultOptions struct{ Expanded, IsPartial bool }
type ExtensionEvent struct{ Type string }

func DefineTool(tool ToolDefinition) ToolDefinition { return tool }
func IsBashToolResult(event ToolResultEvent) bool   { return event.ToolName == "bash" }
func IsReadToolResult(event ToolResultEvent) bool   { return event.ToolName == "read" }
func IsEditToolResult(event ToolResultEvent) bool   { return event.ToolName == "edit" }
func IsWriteToolResult(event ToolResultEvent) bool  { return event.ToolName == "write" }
func IsGrepToolResult(event ToolResultEvent) bool   { return event.ToolName == "grep" }
func IsFindToolResult(event ToolResultEvent) bool   { return event.ToolName == "find" }
func IsLsToolResult(event ToolResultEvent) bool     { return event.ToolName == "ls" }
func IsToolCallEventType(toolName string, event ToolCallEvent) bool {
	return event.ToolName == toolName
}

type ProviderConfig struct {
	API           ai.API
	APIKey        string
	AuthHeader    *bool
	BaseURL       string
	Headers       ai.ProviderHeaders
	Models        []ProviderModelConfig
	Name          string
	OAuth         ExtensionHandler
	RefreshModels ExtensionHandler
	StreamSimple  ExtensionHandler
}
type ProviderModelConfig struct {
	API              ai.API
	BaseURL          string
	Compat           json.RawMessage
	ContextWindow    int64
	Cost             ai.ModelCost
	Headers          ai.ProviderHeaders
	ID               string
	Input            []ai.ModelInput
	MaxTokens        int64
	Name             string
	Reasoning        bool
	ThinkingLevelMap map[agent.ThinkingLevel]agent.ThinkingLevel
}

// ExtensionRunner identifies the deferred extension-runtime subsystem. Its
// upstream members remain in the capability inventory, but exposing methods
// here would prematurely select construction, dispatch, lifecycle, callback,
// concurrency, cancellation, and error-isolation semantics.
type ExtensionRunner struct{}

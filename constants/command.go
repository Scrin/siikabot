package constants

// Command represents a bot command identifier
type Command string

const (
	CommandPing       Command = "!ping"
	CommandTraceroute Command = "!traceroute"
	CommandRemind     Command = "!remind"
	CommandChat       Command = "!chat"
	CommandServers    Command = "!servers"
	CommandAuth       Command = "!auth"
	CommandStats      Command = "!stats"
	CommandMention    Command = "mention"
	CommandReply      Command = "reply"
)

// AllCommands contains all valid command values
var AllCommands = []Command{
	CommandPing, CommandTraceroute, CommandRemind, CommandChat,
	CommandServers, CommandAuth, CommandStats, CommandMention,
	CommandReply,
}

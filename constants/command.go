package constants

// Command represents a bot command identifier
type Command string

const (
	CommandPing       Command = "!ping"
	CommandTraceroute Command = "!traceroute"
	CommandRuuvi      Command = "!ruuvi"
	CommandGrafana    Command = "!grafana"
	CommandRemind     Command = "!remind"
	CommandChat       Command = "!chat"
	CommandServers    Command = "!servers"
	CommandConfig     Command = "!config"
	CommandAuth       Command = "!auth"
	CommandStats      Command = "!stats"
	CommandMention    Command = "mention"
	CommandReply      Command = "reply"
)

// AllCommands contains all valid command values
var AllCommands = []Command{
	CommandPing, CommandTraceroute, CommandRuuvi, CommandGrafana,
	CommandRemind, CommandChat, CommandServers, CommandConfig,
	CommandAuth, CommandStats, CommandMention, CommandReply,
}

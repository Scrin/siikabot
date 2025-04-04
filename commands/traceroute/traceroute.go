package traceroute

import (
	"bufio"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

// Handle handles the traceroute command
func Handle(ctx context.Context, roomID, msg string) {
	split := strings.Split(msg, " ")
	if len(split) < 2 {
		return
	}

	target := split[1]
	command := "traceroute"
	if runtime.GOOS == "windows" {
		command = "tracert"
	}

	log.Debug().
		Str("room_id", roomID).
		Str("target", target).
		Str("command", command).
		Msg("Executing traceroute command")

	cmd := exec.Command(command, target)

	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		log.Error().Err(err).Str("room_id", roomID).Str("target", target).Msg("Failed to create stdout pipe")
		matrix.SendMessage(roomID, err.Error())
		return
	}

	scanner := bufio.NewScanner(cmdReader)

	err = cmd.Start()
	if err != nil {
		log.Error().Err(err).
			Str("room_id", roomID).
			Str("target", target).
			Msg("Failed to start traceroute command")
		matrix.SendMessage(roomID, err.Error())
		return
	}

	matrix.SendTyping(ctx, roomID, true, 30*time.Second)
	var output []string
	for scanner.Scan() {
		output = append(output, scanner.Text())
	}
	matrix.SendTyping(ctx, roomID, false, 0)

	if err = cmd.Wait(); err != nil {
		log.Error().Err(err).
			Str("room_id", roomID).
			Str("target", target).
			Msg("Traceroute command failed")
		matrix.SendMessage(roomID, err.Error())
	} else {
		log.Debug().
			Str("room_id", roomID).
			Str("target", target).
			Msg("Traceroute command completed")
		matrix.SendMessage(roomID, strings.Join(output, "\n"))
	}
}

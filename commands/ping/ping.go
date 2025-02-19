package ping

import (
	"bufio"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

// Handle handles the ping command
func Handle(roomID, msg string) {
	split := strings.Split(msg, " ")
	if len(split) < 2 {
		return
	}

	target := split[1]
	count := 5
	isV6 := strings.Contains(split[1], ":")
	if split[1] == "-6" {
		target = split[2]
		isV6 = true
		if len(split) > 3 {
			if i, err := strconv.Atoi(split[3]); err == nil && i > 0 && i <= 20 {
				count = i
			}
		}
	} else {
		if len(split) > 2 {
			if i, err := strconv.Atoi(split[2]); err == nil && i > 0 && i <= 20 {
				count = i
			}
		}
	}

	log.Debug().
		Str("room_id", roomID).
		Str("target", target).
		Int("count", count).
		Bool("ipv6", isV6).
		Msg("Executing ping command")

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		if isV6 {
			cmd = exec.Command("ping", "-6", "-n", strconv.Itoa(count), target)
		} else {
			cmd = exec.Command("ping", "-n", strconv.Itoa(count), target)
		}
	} else {
		if isV6 {
			cmd = exec.Command("ping6", "-c", strconv.Itoa(count), target)
		} else {
			cmd = exec.Command("ping", "-c", strconv.Itoa(count), target)
		}
	}

	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		log.Error().Err(err).
			Str("room_id", roomID).
			Str("target", target).
			Msg("Failed to create stdout pipe")
		matrix.SendMessage(roomID, err.Error())
		return
	}

	scanner := bufio.NewScanner(cmdReader)
	go func() {
		outChan, done := matrix.SendStreamingMessage(roomID)
		var output []string
		for scanner.Scan() {
			output = append(output, scanner.Text())
			outChan <- strings.Join(output, "\n")
		}
		close(done)
		if err = cmd.Wait(); err != nil {
			log.Error().Err(err).
				Str("room_id", roomID).
				Str("target", target).
				Msg("Ping command failed")
			matrix.SendMessage(roomID, err.Error())
		} else {
			log.Debug().
				Str("room_id", roomID).
				Str("target", target).
				Msg("Ping command completed")
		}
	}()

	err = cmd.Start()
	if err != nil {
		log.Error().Err(err).
			Str("room_id", roomID).
			Str("target", target).
			Msg("Failed to start ping command")
		matrix.SendMessage(roomID, err.Error())
		return
	}
}

package traceroute

import (
	"bufio"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Scrin/siikabot/matrix"
)

// Handle handles the traceroute command
func Handle(roomID, msg string) {
	split := strings.Split(msg, " ")
	if len(split) < 2 {
		return
	}
	command := "traceroute"
	if runtime.GOOS == "windows" {
		command = "tracert"
	}
	cmd := exec.Command(command, split[1])

	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
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
			matrix.SendMessage(roomID, err.Error())
		}
	}()

	err = cmd.Start()
	if err != nil {
		matrix.SendMessage(roomID, err.Error())
		return
	}
}

package bot

import (
	"bufio"
	"os/exec"
	"runtime"
	"strings"
)

func traceroute(roomID, msg string) {
	split := strings.Split(msg, " ")
	if len(split) < 2 {
		client.SendMessage(roomID, "Usage: !traceroute <host>")
		return
	}
	command := "traceroute"
	if runtime.GOOS == "windows" {
		command = "tracert"
	}
	cmd := exec.Command(command, split[1])

	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		client.SendMessage(roomID, err.Error())
		return
	}

	scanner := bufio.NewScanner(cmdReader)
	go func() {
		outChan, done := client.SendStreamingMessage(roomID)
		var output []string
		for scanner.Scan() {
			output = append(output, scanner.Text())
			outChan <- strings.Join(output, "\n")
		}
		close(done)
		if err = cmd.Wait(); err != nil {
			client.SendMessage(roomID, err.Error())
		}
	}()

	err = cmd.Start()
	if err != nil {
		client.SendMessage(roomID, err.Error())
		return
	}
}
